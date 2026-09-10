package productserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/productapi"
)

func (s *Server) PortableMigrationSnapshot(ctx context.Context, importID, mergeID string) (productapi.PortableMigrationSnapshot, error) {
	imports, err := s.Database.ListPortableImportSessions(ctx)
	if err != nil {
		return productapi.PortableMigrationSnapshot{}, err
	}
	merges, err := s.Database.ListPortableMergeSessions(ctx)
	if err != nil {
		return productapi.PortableMigrationSnapshot{}, err
	}
	result := productapi.PortableMigrationSnapshot{Imports: imports, Merges: merges, Conflicts: []productdb.PortableMergeConflict{}, Mappings: []productdb.PortableLibraryMapping{}, Rebuilds: []productdb.PortableGalleryRebuild{}}
	if mergeID != "" {
		result.Conflicts, err = s.Database.ListPortableMergeConflicts(ctx, mergeID)
		if err != nil {
			return productapi.PortableMigrationSnapshot{}, err
		}
	}
	if importID != "" {
		result.Mappings, err = s.Database.ListPortableLibraryMappings(ctx, importID)
		if err != nil {
			return productapi.PortableMigrationSnapshot{}, err
		}
		result.Rebuilds, err = s.Database.ListPortableGalleryRebuilds(ctx, importID)
		if err != nil {
			return productapi.PortableMigrationSnapshot{}, err
		}
		selected, findErr := s.Database.FindPortableImportSession(ctx, importID)
		if findErr != nil {
			return productapi.PortableMigrationSnapshot{}, findErr
		}
		roots, rootErr := s.Database.Operations().StorageRoots(ctx)
		if rootErr != nil {
			return productapi.PortableMigrationSnapshot{}, rootErr
		}
		packagePath := filepath.Join(roots.BackupRoot, filepath.FromSlash(selected.PackageRelativePath))
		relative, relativeErr := filepath.Rel(roots.BackupRoot, packagePath)
		if relativeErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return productapi.PortableMigrationSnapshot{}, errors.New("portable import package escaped backup root")
		}
		manifest, manifestErr := portablecatalog.ReadPackageManifest(ctx, packagePath)
		if manifestErr != nil || manifest.ExportID != selected.ExportID || manifest.FormatVersion != selected.FormatVersion {
			return productapi.PortableMigrationSnapshot{}, errors.New("portable import package summary is invalid")
		}
		result.Owner = &productapi.PortableOwnerContinuitySummary{Available: manifest.OwnerContinuity, GalleryLifecycle: manifest.OwnerGalleryLifecycle, PersonalFlags: manifest.OwnerPersonalFlags, GalleryCount: manifest.OwnerGalleryCount, ItemCount: manifest.OwnerItemCount}
	}
	return result, nil
}

func (s *Server) RunPortableMigration(ctx context.Context, request productapi.PortableMigrationRequest) (productapi.PortableMigrationRunResult, error) {
	want := map[string]string{"PREFLIGHT_EXPORT": "PREFLIGHT", "EXPORT": "EXPORT", "IMPORT": "IMPORT", "PREPARE_MERGE": "PREPARE", "DECIDE_MERGE": "DECIDE", "APPLY_MERGE": "MERGE", "ABORT_MERGE": "ABORT", "RECOVER_IMPORT": "RECOVER", "RECOVER_MERGE": "RECOVER", "MAP_LIBRARIES": "MAP", "PREFLIGHT_REBUILD": "PREFLIGHT", "REBUILD": "REBUILD", "APPLY_CONTINUITY": "CONTINUITY"}
	if expected := want[request.Action]; expected == "" || request.Confirmation != expected {
		return productapi.PortableMigrationRunResult{}, errors.New("portable migration confirmation is invalid")
	}
	result := productapi.PortableMigrationRunResult{Code: "PORTABLE_" + request.Action + "_COMPLETED", ImportID: request.ImportID, MergeID: request.MergeID}
	switch request.Action {
	case "PREFLIGHT_EXPORT":
		value, err := s.PreflightPortableMetadata(ctx)
		if err != nil {
			return result, err
		}
		result.Count = value.GalleryCount
		result.Preflight = &productapi.PortablePreflightSummary{IdentityCount: value.IdentityCount, CoreEntityCount: value.CoreEntityCount, GalleryCount: value.GalleryCount, IncompleteGalleryCount: value.IncompleteGalleryCount, AssetCount: value.AssetCount, WarningCount: value.WarningCount(), BlockingCount: value.BlockingCount(), Issues: value.Issues}
	case "EXPORT":
		path, err := filepath.Abs(request.Path)
		if err != nil {
			return result, err
		}
		value, err := s.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: path, AllowIncompleteGallery: request.AllowIncompleteGallery, IncludeGalleryLifecycle: request.IncludeGalleryLifecycle, IncludePersonalFlags: request.IncludePersonalFlags})
		if err != nil {
			return result, err
		}
		result.ExportID, result.FileName, result.Count = value.ExportID, value.FileName, value.GalleryCount
	case "IMPORT":
		path, err := filepath.Abs(request.Path)
		if err != nil {
			return result, err
		}
		value, err := s.ImportPortableMetadata(ctx, PortableImportOptions{SourcePath: path})
		if err != nil {
			return result, err
		}
		result.ImportID, result.ExportID, result.Count = value.ImportID, value.ExportID, value.ClaimCount
	case "PREPARE_MERGE":
		path, err := filepath.Abs(request.Path)
		if err != nil {
			return result, err
		}
		value, err := s.PreparePortableMerge(ctx, path)
		if err != nil {
			return result, err
		}
		result.MergeID, result.ExportID, result.Count = value.MergeID, value.Report.ExportID, value.Report.ReviewCount()
	case "DECIDE_MERGE":
		if err := s.SetPortableMergeDecisions(ctx, request.MergeID, request.MergeDecisions); err != nil {
			return result, err
		}
		result.Count = len(request.MergeDecisions)
	case "APPLY_MERGE":
		value, err := s.ApplyPortableMerge(ctx, request.MergeID)
		if err != nil {
			return result, err
		}
		result.ImportID, result.Count = request.MergeID, value.PendingGalleryClaims
	case "ABORT_MERGE":
		if err := s.AbortPortableMerge(ctx, request.MergeID); err != nil {
			return result, err
		}
	case "RECOVER_IMPORT":
		value, err := s.RecoverPortableMetadataImport(ctx)
		if err != nil {
			return result, err
		}
		result.ImportID = value.ImportID
	case "RECOVER_MERGE":
		if err := s.RecoverPortableMerge(ctx, request.MergeID); err != nil {
			return result, err
		}
	case "MAP_LIBRARIES":
		if err := s.MapPortableLibraries(ctx, request.ImportID, request.LibraryDecisions); err != nil {
			return result, err
		}
		result.Count = len(request.LibraryDecisions)
	case "PREFLIGHT_REBUILD":
		value, err := s.PreflightPortableGalleryRebuild(ctx, request.ImportID)
		if err != nil {
			return result, err
		}
		result.Count = value.Ready
	case "REBUILD":
		value, err := s.RebuildPortableGalleries(ctx, request.ImportID)
		if err != nil {
			return result, err
		}
		result.Count = value.Rebuilt
	case "APPLY_CONTINUITY":
		value, err := s.ApplyPortableOwnerContinuity(ctx, request.ImportID)
		if err != nil {
			return result, err
		}
		result.Count = value.GalleryCount
	}
	snapshot, err := s.PortableMigrationSnapshot(ctx, result.ImportID, result.MergeID)
	if err != nil {
		return result, err
	}
	result.Snapshot = snapshot
	return result, nil
}
