package productserver

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/coreentity"
	"github.com/stashapp/stash/internal/coserasset"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/portableid"
)

const maxPortableArchiveBytes = int64(10 * 1024 * 1024 * 1024)
const portableImportOwnerMarker = ".portable-import-owner"

type PortableImportOptions struct {
	SourcePath string
}

type PortableImportResult struct {
	ImportID          string
	ExportID          string
	SafetyBackupID    string
	ArchiveSHA256     string
	CoreIdentityCount int
	CoreEntityCount   int
	ClaimCount        int
	AssetCount        int
}

// ImportPortableMetadata imports only into an empty business database. It
// preserves machine-local setup and creates pending claims for Gallery,
// GalleryItem and ExternalLink identities instead of orphan registry rows.
func (s *Server) ImportPortableMetadata(ctx context.Context, options PortableImportOptions) (result PortableImportResult, returnErr error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if !filepath.IsAbs(options.SourcePath) || filepath.Ext(options.SourcePath) != ".zip" {
		return result, errors.New("portable metadata source must be an absolute .zip path")
	}
	state, err := s.Database.Operations().Maintenance(ctx)
	if err != nil {
		return result, err
	}
	if state.Mode != productdb.MaintenanceNormal {
		return result, errors.New("portable metadata import is unavailable during maintenance")
	}
	empty, err := s.Database.PortableImportTargetEmpty(ctx)
	if err != nil {
		return result, err
	}
	if !empty {
		return result, productdb.ErrPortableImportTargetNotEmpty
	}
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return result, err
	}
	importID := portableid.New()
	result.ImportID = importID
	sessionCreated, maintenanceSet, coreCommitted := false, false, false
	defer func() {
		if returnErr != nil && sessionCreated && !coreCommitted {
			_ = s.Database.MarkPortableImportFailed(context.Background(), importID, "PORTABLE_CORE_IMPORT_FAILED", time.Now())
		}
		if returnErr != nil && maintenanceSet && !coreCommitted {
			_ = s.Database.Operations().SetMaintenance(context.Background(), productdb.MaintenanceNormal, "", "PORTABLE_CORE_IMPORT_FAILED", time.Now())
		}
		outcome, code := "SUCCESS", ""
		if returnErr != nil {
			outcome, code = "FAILURE", "PORTABLE_METADATA_IMPORT_FAILED"
		}
		_ = s.Database.Operations().Audit(context.Background(), "PORTABLE_METADATA_IMPORT", "PORTABLE_IMPORT", importID, outcome, code, map[string]any{"core_identities": result.CoreIdentityCount, "core_entities": result.CoreEntityCount, "claims": result.ClaimCount, "assets": result.AssetCount}, time.Now())
	}()
	importRoot := filepath.Join(roots.BackupRoot, "portable-imports", importID)
	if err := os.MkdirAll(importRoot, 0o700); err != nil {
		return result, err
	}
	packagePath := filepath.Join(importRoot, "package.zip")
	cleanupPackage := true
	defer func() {
		if cleanupPackage {
			_ = os.RemoveAll(importRoot)
		}
	}()
	digest, err := copyPortableImportArchive(options.SourcePath, packagePath)
	if err != nil {
		return result, err
	}
	result.ArchiveSHA256 = digest
	inspection, err := portablecatalog.InspectFile(ctx, packagePath)
	if err != nil {
		return result, err
	}
	result.ExportID = inspection.Manifest.ExportID
	claimCounts := map[string]int{}
	streamed, err := portablecatalog.StreamFileIdentities(ctx, packagePath, func(identity portablecatalog.IdentityRecord) error {
		if identity.Kind == "GALLERY" || identity.Kind == "GALLERY_ITEM" || identity.Kind == "EXTERNAL_LINK" {
			claimCounts[identity.Kind]++
		}
		return nil
	})
	if err != nil || streamed != inspection.Manifest.IdentityCount {
		return result, errors.New("portable identity ledger changed after inspection")
	}
	if err := os.MkdirAll(roots.CoserMetadataRoot, 0o700); err != nil {
		return result, err
	}
	version, _, _ := build.Version()
	safety, err := s.Database.Backups().CreateSafetyFull(ctx, productdb.FullBackupOptions{
		BackupRoot: roots.BackupRoot, CoserMetadataRoot: roots.CoserMetadataRoot,
		ProductVersion: version, StartupConfig: s.Config,
	}, time.Now())
	if err != nil {
		return result, err
	}
	result.SafetyBackupID = safety.ID
	stageRoot, assetCosers, err := stagePortableCoserAssets(ctx, packagePath, roots.CoserMetadataRoot, importID, inspection)
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stageRoot)
	coreCount := inspection.Manifest.CoserCount + inspection.Manifest.WorkCount + inspection.Manifest.CharacterCount + inspection.Manifest.TagCount + inspection.Manifest.AccountCount
	relativePackage := filepath.ToSlash(filepath.Join("portable-imports", importID, "package.zip"))
	if err := s.Database.CreatePortableImportSession(ctx, productdb.PortableImportSessionInput{
		ImportID: importID, ExportID: inspection.Manifest.ExportID, PackageSHA256: digest,
		PackageRelativePath: relativePackage, FormatVersion: inspection.Manifest.FormatVersion,
		IdentityCount: inspection.Manifest.IdentityCount, CoreEntityCount: coreCount,
		GalleryClaimCount: claimCounts["GALLERY"], ItemClaimCount: claimCounts["GALLERY_ITEM"], LinkClaimCount: claimCounts["EXTERNAL_LINK"],
		AssetCount: inspection.Manifest.AssetCount, GalleryIndex: inspection.Bundle.Gallery,
	}, time.Now()); err != nil {
		return result, err
	}
	sessionCreated = true
	cleanupPackage = false
	if err := s.Database.Operations().SetMaintenance(ctx, productdb.MaintenancePortableImporting, importID, "", time.Now()); err != nil {
		return result, err
	}
	maintenanceSet = true
	if err := s.Database.BeginPortableImport(ctx, importID, time.Now()); err != nil {
		return result, err
	}
	if currentDigest, err := portableFileSHA256(packagePath); err != nil || currentDigest != digest {
		return result, errors.New("portable metadata package changed before import")
	}
	published := []string{}
	defer func() {
		if !coreCommitted {
			for _, directory := range published {
				_ = os.RemoveAll(directory)
			}
		}
	}()
	publish := func() error {
		for _, coserUUID := range assetCosers {
			source := filepath.Join(stageRoot, coserUUID)
			destination := filepath.Join(roots.CoserMetadataRoot, coserUUID)
			if _, err := os.Lstat(destination); err == nil {
				return errors.New("portable Coser asset destination already exists")
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.Rename(source, destination); err != nil {
				return err
			}
			published = append(published, destination)
		}
		return syncPortableDirectory(roots.CoserMetadataRoot)
	}
	imported, err := s.Database.ImportPortableCore(ctx, importID, inspection.Bundle, func(yield func(portablecatalog.IdentityRecord) error) (int, error) {
		return portablecatalog.StreamFileIdentities(ctx, packagePath, yield)
	}, publish, time.Now())
	if err != nil {
		return result, err
	}
	coreCommitted = true
	result.CoreIdentityCount = imported.CoreIdentityCount
	result.CoreEntityCount = imported.CoreEntityCount
	result.ClaimCount = imported.ClaimCount
	result.AssetCount = inspection.Manifest.AssetCount
	if err := removePortableImportMarkers(published, importID); err != nil {
		return result, errors.New("portable import completed but asset publication could not be finalized")
	}
	if err := s.Database.Operations().SetMaintenance(ctx, productdb.MaintenanceNormal, "", "", time.Now()); err != nil {
		return result, errors.New("portable import completed but maintenance mode could not be resumed")
	}
	maintenanceSet = false
	return result, nil
}

func copyPortableImportArchive(sourcePath, destinationPath string) (string, error) {
	linkInfo, err := os.Lstat(sourcePath)
	if err != nil {
		return "", err
	}
	if !linkInfo.Mode().IsRegular() {
		return "", errors.New("portable metadata source must be a regular non-symlink file")
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer source.Close()
	openedInfo, err := source.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(linkInfo, openedInfo) {
		return "", errors.New("portable metadata source changed while opening")
	}
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	complete := false
	defer func() {
		_ = destination.Close()
		if !complete {
			_ = os.Remove(destinationPath)
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(source, maxPortableArchiveBytes+1))
	if err != nil {
		return "", err
	}
	if written > maxPortableArchiveBytes || written != openedInfo.Size() {
		return "", errors.New("portable metadata source exceeds the archive limit or changed while copying")
	}
	if err := destination.Sync(); err != nil {
		return "", err
	}
	if err := destination.Close(); err != nil {
		return "", err
	}
	complete = true
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func stagePortableCoserAssets(ctx context.Context, packagePath, coserRoot, importID string, inspection portablecatalog.Inspection) (string, []string, error) {
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		return "", nil, err
	}
	stageRoot := filepath.Join(coserRoot, ".portable-import-"+importID)
	if err := os.Mkdir(stageRoot, 0o700); err != nil {
		return "", nil, err
	}
	fail := func(err error) (string, []string, error) {
		_ = os.RemoveAll(stageRoot)
		return "", nil, err
	}
	archive, err := zip.OpenReader(packagePath)
	if err != nil {
		return fail(err)
	}
	defer archive.Close()
	entries := map[string]*zip.File{}
	for _, entry := range archive.File {
		entries[entry.Name] = entry
	}
	checksums := map[string]portablecatalog.ChecksumEntry{}
	for _, checksum := range inspection.Checksums.Files {
		checksums[checksum.Path] = checksum
	}
	cosers := map[string]bool{}
	for _, coser := range inspection.Bundle.Catalog.Cosers {
		for _, ref := range []*portablecatalog.AssetRef{coser.Avatar, coser.Banner} {
			if ref == nil {
				continue
			}
			entry, expected := entries[ref.PackagePath], checksums[ref.PackagePath]
			if entry == nil || expected.Path == "" || expected.Size < 1 {
				return fail(errors.New("portable Coser asset disappeared after inspection"))
			}
			name := strings.TrimPrefix(ref.PackagePath, "coser-assets/"+coser.UUID+"/")
			if name == ref.PackagePath || name == "" || filepath.Base(name) != name {
				return fail(errors.New("portable Coser asset path is invalid"))
			}
			directory := filepath.Join(stageRoot, coser.UUID, "assets")
			if err := os.MkdirAll(directory, 0o700); err != nil {
				return fail(err)
			}
			destination, err := os.OpenFile(filepath.Join(directory, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return fail(err)
			}
			source, err := entry.Open()
			if err != nil {
				_ = destination.Close()
				return fail(err)
			}
			hash := sha256.New()
			written, copyErr := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(source, expected.Size+1))
			closeErr := errors.Join(source.Close(), destination.Sync(), destination.Close())
			if copyErr != nil || closeErr != nil || written != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
				return fail(errors.New("portable Coser asset changed while staging"))
			}
			cosers[coser.UUID] = true
		}
	}
	for _, coser := range inspection.Bundle.Catalog.Cosers {
		if !cosers[coser.UUID] {
			continue
		}
		relative := func(ref *portablecatalog.AssetRef) string {
			if ref == nil {
				return ""
			}
			return filepath.ToSlash(filepath.Join("assets", filepath.Base(ref.PackagePath)))
		}
		var crop *coreentity.AvatarCrop
		if coser.AvatarCrop != nil {
			crop = &coreentity.AvatarCrop{X: coser.AvatarCrop.X, Y: coser.AvatarCrop.Y, Size: coser.AvatarCrop.Size}
		}
		var focal *coreentity.FocalPoint
		if coser.BannerFocal != nil {
			focal = &coreentity.FocalPoint{X: coser.BannerFocal.X, Y: coser.BannerFocal.Y}
		}
		if err := coserasset.RebuildDerivatives(stageRoot, coser.UUID, relative(coser.Avatar), crop, relative(coser.Banner), focal); err != nil {
			return fail(err)
		}
	}
	for coserUUID := range cosers {
		marker := filepath.Join(stageRoot, coserUUID, portableImportOwnerMarker)
		if err := os.WriteFile(marker, []byte(importID+"\n"), 0o600); err != nil {
			return fail(err)
		}
	}
	result := make([]string, 0, len(cosers))
	for uuid := range cosers {
		result = append(result, uuid)
	}
	sort.Strings(result)
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return stageRoot, result, nil
}

func removePortableImportMarkers(directories []string, importID string) error {
	for _, directory := range directories {
		marker := filepath.Join(directory, portableImportOwnerMarker)
		contents, err := os.ReadFile(marker)
		if err != nil || strings.TrimSpace(string(contents)) != importID {
			return errors.New("portable import asset ownership marker is unavailable")
		}
		if err := os.Remove(marker); err != nil {
			return err
		}
		if err := syncPortableDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func syncPortableDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
