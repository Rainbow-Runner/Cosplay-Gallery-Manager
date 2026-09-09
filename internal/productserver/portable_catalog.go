package productserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/coserasset"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/product"
)

type PortableExportOptions struct {
	TargetPath             string
	AllowIncompleteGallery bool
}

type PortableExportResult struct {
	ExportID        string
	FileName        string
	ByteSize        int64
	ArchiveSHA256   string
	IdentityCount   int
	CoreEntityCount int
	GalleryCount    int
	AssetCount      int
	WarningCount    int
}

type PortablePreflightResult struct {
	IdentityCount          int
	IdentityByKind         map[string]int
	IdentityByState        map[string]int
	CoreEntityCount        int
	GalleryCount           int
	IncompleteGalleryCount int
	AssetCount             int
	AssetBytes             int64
	Issues                 []productdb.PortablePreflightIssue
}

func (r PortablePreflightResult) WarningCount() int {
	total := 0
	for _, issue := range r.Issues {
		if issue.Severity == "WARNING" {
			total += issue.Count
		}
	}
	return total
}

func (r PortablePreflightResult) BlockingCount() int {
	total := 0
	for _, issue := range r.Issues {
		if issue.Severity == "BLOCKING" {
			total += issue.Count
		}
	}
	return total
}

// PreflightPortableMetadata validates the current database and managed Coser
// originals without writing a package or changing product state.
func (s *Server) PreflightPortableMetadata(ctx context.Context) (result PortablePreflightResult, returnErr error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	defer func() {
		outcome, code := "SUCCESS", ""
		if returnErr != nil {
			outcome, code = "FAILURE", "PORTABLE_METADATA_PREFLIGHT_FAILED"
		}
		_ = s.Database.Operations().Audit(ctx, "PORTABLE_METADATA_PREFLIGHT", "PORTABLE_EXPORT", "preflight", outcome, code, map[string]any{"identities": result.IdentityCount, "core_entities": result.CoreEntityCount, "galleries": result.GalleryCount, "assets": result.AssetCount, "warnings": result.WarningCount(), "blocking": result.BlockingCount()}, time.Now())
	}()
	return s.preflightPortableMetadata(ctx)
}

func (s *Server) preflightPortableMetadata(ctx context.Context) (PortablePreflightResult, error) {
	state, err := s.Database.Operations().Maintenance(ctx)
	if err != nil {
		return PortablePreflightResult{}, err
	}
	if state.Mode != "NORMAL" {
		return PortablePreflightResult{}, errors.New("portable metadata preflight is unavailable during maintenance")
	}
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return PortablePreflightResult{}, err
	}
	preflight, err := s.Database.PortableCatalogPreflight(ctx)
	if err != nil {
		return PortablePreflightResult{}, err
	}
	result := PortablePreflightResult{
		IdentityCount: preflight.IdentityCount, IdentityByKind: preflight.IdentityByKind, IdentityByState: preflight.IdentityByState,
		CoreEntityCount: preflight.CoserCount + preflight.WorkCount + preflight.CharacterCount + preflight.TagCount + preflight.AccountCount,
		GalleryCount:    preflight.GalleryCount, IncompleteGalleryCount: preflight.IncompleteGalleryCount,
		AssetCount: preflight.AssetCount, Issues: append([]productdb.PortablePreflightIssue{}, preflight.Issues...),
	}
	for _, source := range preflight.Assets {
		file, info, openErr := coserasset.OpenManaged(roots.CoserMetadataRoot, source.CoserUUID, source.RelativePath)
		if openErr != nil {
			result.Issues = addPortableServerIssue(result.Issues, "PORTABLE_COSER_ASSET_UNREADABLE", "BLOCKING")
			continue
		}
		validateErr := portablecatalog.ValidateCoserAsset(file, info.Size())
		closeErr := file.Close()
		if validateErr != nil || closeErr != nil {
			result.Issues = addPortableServerIssue(result.Issues, "PORTABLE_COSER_ASSET_INVALID", "BLOCKING")
			continue
		}
		result.AssetBytes += info.Size()
	}
	sort.Slice(result.Issues, func(i, j int) bool { return result.Issues[i].Code < result.Issues[j].Code })
	return result, nil
}

func addPortableServerIssue(issues []productdb.PortablePreflightIssue, code, severity string) []productdb.PortablePreflightIssue {
	for index := range issues {
		if issues[index].Code == code && issues[index].Severity == severity {
			issues[index].Count++
			return issues
		}
	}
	return append(issues, productdb.PortablePreflightIssue{Code: code, Severity: severity, Count: 1})
}

// ExportPortableMetadata creates a portable catalog without changing the
// database, manifests, media sources, or Coser metadata tree.
func (s *Server) ExportPortableMetadata(ctx context.Context, options PortableExportOptions) (result PortableExportResult, returnErr error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if !filepath.IsAbs(options.TargetPath) || filepath.Ext(options.TargetPath) != ".zip" {
		return result, errors.New("portable metadata target must be an absolute .zip path")
	}
	preflight, err := s.preflightPortableMetadata(ctx)
	if err != nil {
		return result, err
	}
	if preflight.BlockingCount() > 0 {
		return result, fmt.Errorf("portable metadata export blocked by %d blocking preflight findings", preflight.BlockingCount())
	}
	if preflight.IncompleteGalleryCount > 0 && !options.AllowIncompleteGallery {
		return result, fmt.Errorf("portable metadata export blocked by %d incomplete Gallery records", preflight.IncompleteGalleryCount)
	}
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return result, err
	}
	now := time.Now().UTC().Truncate(time.Second)
	exportID := portableid.New()
	result.ExportID = exportID
	defer func() {
		if returnErr != nil {
			_ = s.Database.Operations().Audit(ctx, "PORTABLE_METADATA_EXPORT", "PORTABLE_EXPORT", exportID, "FAILURE", "PORTABLE_METADATA_EXPORT_FAILED", map[string]any{}, time.Now())
		}
	}()
	version, _, _ := build.Version()
	manifest := portablecatalog.PackageManifest{Format: portablecatalog.Format, FormatVersion: portablecatalog.FormatVersion, ProductID: product.ID, ExportID: exportID, CreatedAt: now.Format(time.RFC3339), Versions: portablecatalog.Versions(product.CurrentVersions(version))}
	reader, err := s.Database.BeginPortableCatalogRead(ctx, manifest)
	if err != nil {
		return result, err
	}
	defer reader.Close()
	snapshot := reader.Snapshot()
	warnings := 0
	for _, gallery := range snapshot.Bundle.Gallery.Galleries {
		if gallery.ManifestStatus != "CLEAN" || gallery.LocatorStatus == "UNBOUND" || gallery.LocatorStatus == "OUTSIDE_LIBRARY" {
			warnings++
		}
	}
	if warnings > 0 && !options.AllowIncompleteGallery {
		return result, fmt.Errorf("portable metadata export blocked by %d incomplete Gallery records", warnings)
	}
	opened := []*os.File{}
	defer func() {
		for _, file := range opened {
			_ = file.Close()
		}
	}()
	assets := make([]portablecatalog.AssetSource, 0, len(snapshot.Assets))
	for _, source := range snapshot.Assets {
		file, info, err := coserasset.OpenManaged(roots.CoserMetadataRoot, source.CoserUUID, source.RelativePath)
		if err != nil {
			return result, fmt.Errorf("opening current Coser %s asset: %w", source.Kind, err)
		}
		opened = append(opened, file)
		assetFile := file
		used := false
		assets = append(assets, portablecatalog.AssetSource{Path: source.PackagePath, Kind: source.Kind, Size: info.Size(), Open: func() (io.ReadCloser, error) {
			if used {
				return nil, errors.New("portable asset was opened more than once")
			}
			used = true
			return assetFile, nil
		}})
	}
	identitySource := portablecatalog.IdentitySource{Count: reader.IdentityCount(), Stream: func(yield func(portablecatalog.IdentityRecord) error) error {
		return reader.StreamIdentities(ctx, yield)
	}}
	if err := portablecatalog.WriteFileAtomicStreaming(options.TargetPath, snapshot.Bundle, assets, identitySource); err != nil {
		return result, err
	}
	if err := reader.Commit(); err != nil {
		_ = os.Remove(options.TargetPath)
		return result, err
	}
	inspection, err := portablecatalog.InspectFile(ctx, options.TargetPath)
	if err != nil {
		_ = os.Remove(options.TargetPath)
		return result, fmt.Errorf("verifying completed portable metadata package: %w", err)
	}
	info, err := os.Stat(options.TargetPath)
	if err != nil {
		_ = os.Remove(options.TargetPath)
		return result, err
	}
	digest, err := portableFileSHA256(options.TargetPath)
	if err != nil {
		_ = os.Remove(options.TargetPath)
		return result, err
	}
	result = PortableExportResult{ExportID: exportID, FileName: filepath.Base(options.TargetPath), ByteSize: info.Size(), ArchiveSHA256: digest, IdentityCount: inspection.Manifest.IdentityCount, CoreEntityCount: inspection.Manifest.CoserCount + inspection.Manifest.WorkCount + inspection.Manifest.CharacterCount + inspection.Manifest.TagCount + inspection.Manifest.AccountCount, GalleryCount: inspection.Manifest.GalleryCount, AssetCount: inspection.Manifest.AssetCount, WarningCount: warnings}
	_ = s.Database.Operations().Audit(ctx, "PORTABLE_METADATA_EXPORT", "PORTABLE_EXPORT", exportID, "SUCCESS", "", map[string]any{"bytes": result.ByteSize, "identities": result.IdentityCount, "core_entities": result.CoreEntityCount, "galleries": result.GalleryCount, "assets": result.AssetCount, "warnings": warnings}, time.Now())
	return result, nil
}

func portableFileSHA256(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
