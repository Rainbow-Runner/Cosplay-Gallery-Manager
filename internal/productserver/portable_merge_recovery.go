package productserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/portableid"
)

func (s *Server) AbortPortableMerge(ctx context.Context, mergeID string) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if _, err := portableid.Parse(mergeID); err != nil {
		return err
	}
	if err := s.Database.AbortPortableMerge(ctx, mergeID, time.Now()); err != nil {
		return err
	}
	_ = s.Database.Operations().Audit(ctx, "PORTABLE_MERGE_ABORT", "PORTABLE_MERGE", mergeID, "SUCCESS", "", nil, time.Now())
	return nil
}

// RecoverPortableMerge reconciles the bounded filesystem window around new
// Coser asset publication. READY means the database transaction did not
// commit, while APPLIED means only ownership-marker finalization may remain.
func (s *Server) RecoverPortableMerge(ctx context.Context, mergeID string) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if _, err := portableid.Parse(mergeID); err != nil {
		return err
	}
	maintenance, err := s.Database.Operations().Maintenance(ctx)
	if err != nil {
		return err
	}
	if maintenance.Mode != productdb.MaintenancePortableMerging || maintenance.RestoreBackupID != mergeID {
		return errors.New("portable merge recovery is not pending")
	}
	session, err := s.Database.FindPortableMergeSession(ctx, mergeID)
	if err != nil {
		return err
	}
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return err
	}
	packagePath := filepath.Join(roots.BackupRoot, filepath.FromSlash(session.PackageRelativePath))
	digest, err := portableFileSHA256(packagePath)
	if err != nil || digest != session.PackageSHA256 {
		return errors.New("portable merge recovery package digest is invalid")
	}
	inspection, err := portablecatalog.InspectFile(ctx, packagePath)
	if err != nil || inspection.Manifest.ExportID != session.ExportID {
		return errors.New("portable merge recovery package is invalid")
	}
	owned := make([]string, 0)
	for _, coserUUID := range portableAssetCosers(inspection.Bundle.Catalog) {
		directory := filepath.Join(roots.CoserMetadataRoot, coserUUID)
		contents, readErr := os.ReadFile(filepath.Join(directory, portableImportOwnerMarker))
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil || strings.TrimSpace(string(contents)) != mergeID {
			return errors.New("portable merge asset ownership marker is invalid")
		}
		owned = append(owned, coserUUID)
	}
	switch session.State {
	case "READY":
		for _, coserUUID := range owned {
			destinationRoot := filepath.Join(roots.CoserMetadataRoot, coserUUID)
			rollbackDirectory := filepath.Join(roots.BackupRoot, "portable-merges", mergeID, "asset-rollback", coserUUID)
			if _, statErr := os.Lstat(rollbackDirectory); errors.Is(statErr, os.ErrNotExist) {
				if err := os.RemoveAll(destinationRoot); err != nil {
					return err
				}
			} else if statErr != nil {
				return statErr
			} else {
				if err := os.RemoveAll(filepath.Join(destinationRoot, "assets")); err != nil {
					return err
				}
				oldAssets := filepath.Join(rollbackDirectory, "assets")
				if _, oldErr := os.Lstat(oldAssets); oldErr == nil {
					if err := os.Rename(oldAssets, filepath.Join(destinationRoot, "assets")); err != nil {
						return err
					}
				} else if !errors.Is(oldErr, os.ErrNotExist) {
					return oldErr
				}
				if err := os.Remove(filepath.Join(destinationRoot, portableImportOwnerMarker)); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
		}
	case "APPLIED":
		filtered := inspection
		filtered.Bundle.Catalog.Cosers = nil
		for _, coser := range inspection.Bundle.Catalog.Cosers {
			for _, uuid := range owned {
				if coser.UUID == uuid {
					filtered.Bundle.Catalog.Cosers = append(filtered.Bundle.Catalog.Cosers, coser)
				}
			}
		}
		if err := validatePublishedPortableAssets(roots.CoserMetadataRoot, filtered); err != nil {
			return err
		}
		if err := clearPublishedPortableMarkers(roots.CoserMetadataRoot, owned, mergeID); err != nil {
			return err
		}
	default:
		return errors.New("portable merge session has no recoverable asset publication")
	}
	if err := os.RemoveAll(filepath.Join(roots.CoserMetadataRoot, ".portable-import-"+mergeID)); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(roots.BackupRoot, "portable-merges", mergeID, "asset-rollback")); err != nil {
		return err
	}
	if err := s.Database.Operations().SetMaintenance(ctx, productdb.MaintenanceNormal, "", "", time.Now()); err != nil {
		return err
	}
	_ = s.Database.Operations().Audit(ctx, "PORTABLE_MERGE_RECOVERY", "PORTABLE_MERGE", mergeID, "SUCCESS", "", map[string]any{"asset_directories": len(owned)}, time.Now())
	return nil
}
