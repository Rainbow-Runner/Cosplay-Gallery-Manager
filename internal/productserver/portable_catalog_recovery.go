package productserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/coserasset"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portablecatalog"
)

// RecoverPortableMetadataImport reconciles the only process-crash windows:
// published Coser directories before the database commit, or a committed core
// import before its ownership markers and maintenance state were finalized.
func (s *Server) RecoverPortableMetadataImport(ctx context.Context) (result PortableImportResult, returnErr error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	state, err := s.Database.Operations().Maintenance(ctx)
	if err != nil {
		return result, err
	}
	if state.Mode != productdb.MaintenancePortableImporting || state.RestoreBackupID == "" {
		return result, errors.New("portable import recovery is not pending")
	}
	session, err := s.Database.FindPortableImportSession(ctx, state.RestoreBackupID)
	if err != nil {
		return result, err
	}
	result.ImportID, result.ExportID = session.ImportID, session.ExportID
	defer func() {
		outcome := "SUCCESS"
		code := ""
		if returnErr != nil {
			outcome, code = "FAILURE", "PORTABLE_IMPORT_RECOVERY_FAILED"
		}
		_ = s.Database.Operations().Audit(context.Background(), "PORTABLE_IMPORT_RECOVERY", "PORTABLE_IMPORT", result.ImportID, outcome, code, nil, time.Now())
	}()
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return result, err
	}
	packagePath := filepath.Join(roots.BackupRoot, filepath.FromSlash(session.PackageRelativePath))
	relative, err := filepath.Rel(roots.BackupRoot, packagePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return result, errors.New("portable import recovery package escaped backup root")
	}
	digest, err := portableFileSHA256(packagePath)
	if err != nil || digest != session.PackageSHA256 {
		return result, errors.New("portable import recovery package digest is invalid")
	}
	inspection, err := portablecatalog.InspectFile(ctx, packagePath)
	if err != nil || inspection.Manifest.ExportID != session.ExportID {
		return result, errors.New("portable import recovery package is invalid")
	}
	assetCosers := portableAssetCosers(inspection.Bundle.Catalog)
	stageRoot := filepath.Join(roots.CoserMetadataRoot, ".portable-import-"+session.ImportID)
	switch session.State {
	case "CORE_IMPORTED":
		if err := validatePublishedPortableAssets(roots.CoserMetadataRoot, inspection); err != nil {
			return result, err
		}
		if err := clearPublishedPortableMarkers(roots.CoserMetadataRoot, assetCosers, session.ImportID); err != nil {
			return result, err
		}
	case "INSPECTED", "IMPORTING", "FAILED":
		empty, err := s.Database.PortableImportTargetEmpty(ctx)
		if err != nil || !empty {
			return result, errors.New("interrupted portable import left unexpected business records")
		}
		if err := removeInterruptedPortableAssets(roots.CoserMetadataRoot, assetCosers, session.ImportID); err != nil {
			return result, err
		}
		if session.State != "FAILED" {
			if err := s.Database.MarkPortableImportFailed(ctx, session.ImportID, "PORTABLE_IMPORT_INTERRUPTED", time.Now()); err != nil {
				return result, err
			}
		}
	default:
		return result, errors.New("portable import session state cannot be recovered")
	}
	if err := os.RemoveAll(stageRoot); err != nil {
		return result, err
	}
	if err := s.Database.Operations().SetMaintenance(ctx, productdb.MaintenanceNormal, "", "", time.Now()); err != nil {
		return result, err
	}
	return result, nil
}

func portableAssetCosers(catalog portablecatalog.CoreCatalog) []string {
	result := make([]string, 0)
	for _, coser := range catalog.Cosers {
		if coser.Avatar != nil || coser.Banner != nil {
			result = append(result, coser.UUID)
		}
	}
	return result
}

func validatePublishedPortableAssets(coserRoot string, inspection portablecatalog.Inspection) error {
	checksums := make(map[string]portablecatalog.ChecksumEntry, len(inspection.Checksums.Files))
	for _, value := range inspection.Checksums.Files {
		checksums[value.Path] = value
	}
	for _, coser := range inspection.Bundle.Catalog.Cosers {
		for _, ref := range []*portablecatalog.AssetRef{coser.Avatar, coser.Banner} {
			if ref == nil {
				continue
			}
			name := strings.TrimPrefix(ref.PackagePath, "coser-assets/"+coser.UUID+"/")
			expected, ok := checksums[ref.PackagePath]
			if !ok || name == ref.PackagePath || filepath.Base(name) != name {
				return errors.New("portable imported asset reference is invalid")
			}
			file, info, err := coserasset.OpenManaged(coserRoot, coser.UUID, filepath.ToSlash(filepath.Join("assets", name)))
			if err != nil {
				return err
			}
			hash := sha256.New()
			written, copyErr := io.Copy(hash, file)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil || written != info.Size() || written != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
				return errors.New("portable imported asset validation failed")
			}
		}
	}
	return nil
}

func clearPublishedPortableMarkers(coserRoot string, coserUUIDs []string, importID string) error {
	for _, coserUUID := range coserUUIDs {
		marker := filepath.Join(coserRoot, coserUUID, portableImportOwnerMarker)
		contents, err := os.ReadFile(marker)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || strings.TrimSpace(string(contents)) != importID {
			return errors.New("portable import asset ownership marker is invalid")
		}
		if err := os.Remove(marker); err != nil {
			return err
		}
		if err := syncPortableDirectory(filepath.Dir(marker)); err != nil {
			return err
		}
	}
	return nil
}

func removeInterruptedPortableAssets(coserRoot string, coserUUIDs []string, importID string) error {
	for _, coserUUID := range coserUUIDs {
		directory := filepath.Join(coserRoot, coserUUID)
		contents, err := os.ReadFile(filepath.Join(directory, portableImportOwnerMarker))
		if errors.Is(err, os.ErrNotExist) {
			if _, statErr := os.Lstat(directory); statErr == nil {
				return errors.New("portable import asset directory lacks an ownership marker")
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
			continue
		}
		if err != nil || strings.TrimSpace(string(contents)) != importID {
			return errors.New("portable import asset ownership marker is invalid")
		}
		if err := os.RemoveAll(directory); err != nil {
			return err
		}
	}
	return nil
}
