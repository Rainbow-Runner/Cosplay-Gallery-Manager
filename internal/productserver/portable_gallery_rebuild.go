package productserver

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/archivefile"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/sourcescan"
)

type PortableGalleryRebuildReport struct {
	ImportID string
	Ready    int
	Blocked  int
	Skipped  int
	Rebuilt  int
	Entries  []PortableGalleryRebuildEntry
}

type PortableGalleryRebuildEntry struct {
	SetID          string
	State          string
	IssueCode      string
	LibraryKey     string
	RelativeSource string
	SourceType     gallery.SourceType
	GalleryID      *int64
}

type portablePreparedGallery struct {
	rebuild productdb.PortableGalleryRebuild
	mapping productdb.PortableLibraryMapping
	path    string
	doc     manifest.GalleryDocument
}

func (s *Server) MapPortableLibraries(ctx context.Context, importID string, decisions []productdb.PortableLibraryDecision) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if _, err := portableid.Parse(importID); err != nil {
		return err
	}
	if err := s.Database.SetPortableLibraryMappings(ctx, importID, decisions, time.Now()); err != nil {
		_ = s.Database.Operations().Audit(ctx, "PORTABLE_LIBRARY_MAP", "PORTABLE_IMPORT", importID, "FAILURE", "PORTABLE_LIBRARY_MAP_FAILED", nil, time.Now())
		return err
	}
	_ = s.Database.Operations().Audit(ctx, "PORTABLE_LIBRARY_MAP", "PORTABLE_IMPORT", importID, "SUCCESS", "", map[string]any{"decision_count": len(decisions)}, time.Now())
	return nil
}

// PreflightPortableGalleryRebuild reads mapped sources and sidecars but does
// not create a Gallery, scan run, Item, processing job, or Manifest file.
func (s *Server) PreflightPortableGalleryRebuild(ctx context.Context, importID string) (PortableGalleryRebuildReport, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.preflightPortableGalleryRebuild(ctx, importID)
}

func (s *Server) preflightPortableGalleryRebuild(ctx context.Context, importID string) (PortableGalleryRebuildReport, error) {
	result := PortableGalleryRebuildReport{ImportID: importID}
	if _, err := portableid.Parse(importID); err != nil {
		return result, err
	}
	session, err := s.Database.FindPortableImportSession(ctx, importID)
	if err != nil {
		return result, err
	}
	if session.State != "LIBRARIES_MAPPED" && session.State != "GALLERIES_REBUILT" {
		return result, errors.New("portable import media libraries are not fully mapped")
	}
	mappings, err := s.Database.ListPortableLibraryMappings(ctx, importID)
	if err != nil {
		return result, err
	}
	byKey := make(map[string]productdb.PortableLibraryMapping, len(mappings))
	for _, value := range mappings {
		byKey[value.LibraryKey] = value
	}
	rebuilds, err := s.Database.ListPortableGalleryRebuilds(ctx, importID)
	if err != nil {
		return result, err
	}
	limits, err := s.portableArchiveLimits(ctx)
	if err != nil {
		return result, err
	}
	for _, rebuild := range rebuilds {
		entry := PortableGalleryRebuildEntry{SetID: rebuild.SetID, State: rebuild.State, IssueCode: rebuild.IssueCode, LibraryKey: rebuild.LibraryKey, RelativeSource: rebuild.RelativeSource, SourceType: rebuild.SourceType, GalleryID: rebuild.GalleryID}
		if rebuild.State == "REBUILT" {
			result.Rebuilt++
			result.Entries = append(result.Entries, entry)
			continue
		}
		prepared, code := s.preparePortableGallery(ctx, rebuild, byKey, limits)
		_ = prepared
		switch {
		case code == "PORTABLE_LIBRARY_SKIPPED":
			entry.State, entry.IssueCode = "SKIPPED", ""
			result.Skipped++
			if rebuild.State != "REBUILDING" {
				if err := s.Database.SetPortableGalleryInspection(ctx, importID, rebuild.SetID, "SKIPPED", "", time.Now()); err != nil {
					return result, err
				}
			}
		case code != "":
			entry.State, entry.IssueCode = "BLOCKED", code
			result.Blocked++
			if rebuild.State == "REBUILDING" {
				if err := s.Database.SetPortableGalleryRebuildIssue(ctx, importID, rebuild.SetID, code, time.Now()); err != nil {
					return result, err
				}
			} else {
				if err := s.Database.SetPortableGalleryInspection(ctx, importID, rebuild.SetID, "BLOCKED", code, time.Now()); err != nil {
					return result, err
				}
			}
		default:
			entry.IssueCode = ""
			result.Ready++
			if rebuild.State != "REBUILDING" {
				entry.State = "READY"
				if err := s.Database.SetPortableGalleryInspection(ctx, importID, rebuild.SetID, "READY", "", time.Now()); err != nil {
					return result, err
				}
			}
		}
		result.Entries = append(result.Entries, entry)
	}
	if err := s.Database.FinalizePortableGalleryRebuilds(ctx, importID, time.Now()); err != nil {
		return result, err
	}
	_ = s.Database.Operations().Audit(ctx, "PORTABLE_GALLERY_PREFLIGHT", "PORTABLE_IMPORT", importID, "SUCCESS", "", map[string]any{"ready": result.Ready, "blocked": result.Blocked, "skipped": result.Skipped, "rebuilt": result.Rebuilt}, time.Now())
	return result, nil
}

func (s *Server) preparePortableGallery(ctx context.Context, rebuild productdb.PortableGalleryRebuild, mappings map[string]productdb.PortableLibraryMapping, limits archivecheck.Limits) (portablePreparedGallery, string) {
	prepared := portablePreparedGallery{rebuild: rebuild}
	if rebuild.LocatorStatus != "MAPPED" && rebuild.LocatorStatus != "LIBRARY_ROOT" {
		return prepared, "PORTABLE_LOCATOR_UNRESOLVED"
	}
	mapping, ok := mappings[rebuild.LibraryKey]
	if !ok || mapping.Decision == "UNMAPPED" {
		return prepared, "PORTABLE_LIBRARY_UNMAPPED"
	}
	if mapping.Decision == "SKIPPED" {
		return prepared, "PORTABLE_LIBRARY_SKIPPED"
	}
	if mapping.TargetLibraryID == nil || mapping.TargetRoot == "" {
		return prepared, "PORTABLE_LIBRARY_TARGET_MISSING"
	}
	prepared.mapping = mapping
	if rebuild.ManifestStatus != "CLEAN" || rebuild.ManifestHash == "" {
		return prepared, "PORTABLE_MANIFEST_NOT_CLEAN"
	}
	sourcePath := mapping.TargetRoot
	if rebuild.LocatorStatus == "MAPPED" {
		sourcePath = filepath.Join(mapping.TargetRoot, filepath.FromSlash(rebuild.RelativeSource))
	}
	if err := validatePortableSourcePath(mapping.TargetRoot, sourcePath, rebuild.SourceType); err != nil {
		return prepared, "PORTABLE_SOURCE_UNAVAILABLE"
	}
	manifestPath, err := manifest.GalleryPath(rebuild.SourceType, sourcePath)
	if err != nil {
		return prepared, "PORTABLE_MANIFEST_PATH_INVALID"
	}
	data, hash, err := manifest.ReadFile(manifestPath, manifest.MaxGalleryBytes)
	if err != nil {
		return prepared, "PORTABLE_MANIFEST_UNAVAILABLE"
	}
	if hash != rebuild.ManifestHash {
		return prepared, "PORTABLE_MANIFEST_CHANGED"
	}
	document, err := manifest.ParseGallery(bytes.NewReader(data))
	if err != nil || document.SetID != rebuild.SetID || document.Revision != rebuild.ManifestRevision {
		return prepared, "PORTABLE_MANIFEST_IDENTITY_MISMATCH"
	}
	var scan sourcescan.Result
	if rebuild.SourceType == gallery.SourceTypeArchive {
		scan, err = sourcescan.ScanArchive(ctx, sourcePath, limits)
	} else {
		scan, err = sourcescan.ScanDirectory(ctx, sourcePath)
	}
	if err != nil {
		return prepared, "PORTABLE_SOURCE_SCAN_FAILED"
	}
	if !scan.Complete {
		return prepared, "PORTABLE_SOURCE_SCAN_UNSAFE"
	}
	if len(scan.Observations) > 1000 {
		return prepared, "PORTABLE_SOURCE_OVER_LIMIT"
	}
	observed := make(map[string]int, len(scan.Observations))
	for _, value := range scan.Observations {
		observed[value.RelativePath]++
	}
	itemUUIDs, linkUUIDs, uniquePaths := portableManifestClaimedUUIDs(document)
	if !uniquePaths {
		return prepared, "PORTABLE_ITEM_PATH_MISMATCH"
	}
	for path := range itemUUIDs {
		if observed[path] != 1 {
			return prepared, "PORTABLE_ITEM_PATH_MISMATCH"
		}
	}
	if !s.portableClaimsValid(ctx, rebuild, itemUUIDs, linkUUIDs) {
		return prepared, "PORTABLE_IDENTITY_CLAIM_INVALID"
	}
	prepared.path, prepared.doc = sourcePath, document
	return prepared, ""
}

func (s *Server) RebuildPortableGalleries(ctx context.Context, importID string) (report PortableGalleryRebuildReport, returnErr error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	defer func() {
		if returnErr != nil {
			_ = s.Database.Operations().Audit(ctx, "PORTABLE_GALLERY_REBUILD", "PORTABLE_IMPORT", importID, "FAILURE", "PORTABLE_GALLERY_REBUILD_FAILED", nil, time.Now())
		}
	}()
	preflight, err := s.preflightPortableGalleryRebuild(ctx, importID)
	if err != nil {
		return preflight, err
	}
	if preflight.Blocked != 0 {
		return preflight, errors.New("portable Gallery rebuild is blocked by preflight findings")
	}
	mappings, err := s.Database.ListPortableLibraryMappings(ctx, importID)
	if err != nil {
		return preflight, err
	}
	byKey := make(map[string]productdb.PortableLibraryMapping, len(mappings))
	for _, value := range mappings {
		byKey[value.LibraryKey] = value
	}
	rebuilds, err := s.Database.ListPortableGalleryRebuilds(ctx, importID)
	if err != nil {
		return preflight, err
	}
	limits, err := s.portableArchiveLimits(ctx)
	if err != nil {
		return preflight, err
	}
	for _, rebuild := range rebuilds {
		if rebuild.State == "REBUILT" || rebuild.State == "SKIPPED" {
			continue
		}
		prepared, code := s.preparePortableGallery(ctx, rebuild, byKey, limits)
		if code != "" {
			return preflight, errors.New(code)
		}
		galleryID, sourceID := int64(0), int64(0)
		if rebuild.State == "REBUILDING" && rebuild.GalleryID != nil && rebuild.SourceID != nil {
			galleryID, sourceID = *rebuild.GalleryID, *rebuild.SourceID
		} else {
			title := portableGalleryTitle(prepared.doc, prepared.path, rebuild.SourceType)
			created, source, err := s.Database.CreatePortableGalleryDraft(ctx, importID, rebuild.SetID, *prepared.mapping.TargetLibraryID, rebuild.SourceType, prepared.path, title, time.Now())
			if err != nil {
				return preflight, err
			}
			galleryID, sourceID = created.ID, source.ID
		}
		itemUUIDs, _, _ := portableManifestClaimedUUIDs(prepared.doc)
		if err := s.Database.Scans().RunWithOptions(ctx, sourceID, limits, productdb.ScanOptions{ExcludeNewRootMedia: false, PortableImportID: importID, PortableItemUUIDByPath: itemUUIDs}, time.Now()); err != nil {
			_ = s.Database.SetPortableGalleryRebuildIssue(ctx, importID, rebuild.SetID, "PORTABLE_SOURCE_SCAN_COMMIT_FAILED", time.Now())
			return preflight, err
		}
		current, err := s.Database.Galleries().Find(ctx, galleryID)
		if err != nil {
			return preflight, err
		}
		if _, err := s.Database.Manifests().PullPortableGallery(ctx, galleryID, current.MetadataRevision, importID, time.Now()); err != nil {
			_ = s.Database.SetPortableGalleryRebuildIssue(ctx, importID, rebuild.SetID, "PORTABLE_MANIFEST_APPLY_FAILED", time.Now())
			return preflight, err
		}
		if _, err := s.Database.CaptureDates().EnqueueGallery(ctx, galleryID, s.VideoTools.FFprobe.Available, time.Now()); err != nil {
			_ = s.Database.SetPortableGalleryRebuildIssue(ctx, importID, rebuild.SetID, "PORTABLE_CAPTURE_DATE_QUEUE_FAILED", time.Now())
			return preflight, err
		}
		if err := s.Database.FinishPortableGalleryRebuild(ctx, importID, rebuild.SetID, galleryID, sourceID, time.Now()); err != nil {
			return preflight, err
		}
	}
	if err := s.Database.FinalizePortableGalleryRebuilds(ctx, importID, time.Now()); err != nil {
		return preflight, err
	}
	result, err := s.preflightPortableGalleryRebuild(ctx, importID)
	if err == nil {
		_ = s.Database.Operations().Audit(ctx, "PORTABLE_GALLERY_REBUILD", "PORTABLE_IMPORT", importID, "SUCCESS", "", map[string]any{"rebuilt": result.Rebuilt, "skipped": result.Skipped}, time.Now())
	}
	return result, err
}

func (s *Server) portableArchiveLimits(ctx context.Context) (archivecheck.Limits, error) {
	runtime, err := s.Database.Settings().Find(ctx)
	if err != nil {
		return archivecheck.Limits{}, err
	}
	return archivecheck.Limits{MaxEntries: runtime.ArchiveMaxEntries, MaxEntryUncompressed: uint64(runtime.ArchiveMaxEntryBytes), MaxTotalUncompressed: uint64(runtime.ArchiveMaxTotalBytes), MaxCompressionRatio: runtime.ArchiveMaxCompressionRatio, MaxImagePixels: uint64(runtime.ArchiveMaxImagePixels)}, nil
}

func validatePortableSourcePath(root, source string, kind gallery.SourceType) error {
	root = filepath.Clean(root)
	source = filepath.Clean(source)
	relative, err := filepath.Rel(root, source)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("portable source escaped mapped root")
	}
	parent := source
	if kind == gallery.SourceTypeArchive {
		parent = filepath.Dir(source)
	}
	for current := parent; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("portable source parent is unsafe")
		}
		if current == root {
			break
		}
		if next := filepath.Dir(current); next == current {
			return errors.New("portable source parent escaped mapped root")
		}
	}
	info, err := os.Lstat(source)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("portable source is unavailable")
	}
	if (kind == gallery.SourceTypeDirectory && !info.IsDir()) || (kind == gallery.SourceTypeArchive && !info.Mode().IsRegular()) {
		return errors.New("portable source type changed")
	}
	return nil
}

func portableManifestClaimedUUIDs(document manifest.GalleryDocument) (map[string]string, []string, bool) {
	items := map[string]string{}
	uniquePaths := true
	if document.Items.Present && !document.Items.Null {
		for _, value := range document.Items.Value {
			if value.ItemUUID != "" {
				if _, exists := items[value.Path]; exists {
					uniquePaths = false
				}
				items[value.Path] = value.ItemUUID
			}
		}
	}
	var links []string
	if document.ExternalLinks.Present && !document.ExternalLinks.Null {
		for _, value := range document.ExternalLinks.Value {
			if value.LinkUUID != "" {
				links = append(links, value.LinkUUID)
			}
		}
	}
	return items, links, uniquePaths
}

func (s *Server) portableClaimsValid(ctx context.Context, rebuild productdb.PortableGalleryRebuild, items map[string]string, links []string) bool {
	values := []struct{ uuid, kind string }{{rebuild.SetID, "GALLERY"}}
	for _, uuid := range items {
		values = append(values, struct{ uuid, kind string }{uuid, "GALLERY_ITEM"})
	}
	for _, uuid := range links {
		values = append(values, struct{ uuid, kind string }{uuid, "EXTERNAL_LINK"})
	}
	for _, value := range values {
		var kind, identityState, claimState string
		err := s.Database.QueryRowContext(ctx, `SELECT entity_kind,identity_state,claim_state FROM portable_identity_claims WHERE import_id=? AND uuid=?`, rebuild.ImportID, value.uuid).Scan(&kind, &identityState, &claimState)
		if err != nil || kind != value.kind || identityState != "ACTIVE" || claimState != "PENDING" && claimState != "CLAIMED" {
			return false
		}
		if claimState == "CLAIMED" {
			var registryKind string
			if err := s.Database.QueryRowContext(ctx, `SELECT entity_kind FROM portable_uuid_registry WHERE uuid=?`, value.uuid).Scan(&registryKind); err != nil || registryKind != value.kind {
				return false
			}
			if rebuild.GalleryID == nil {
				return false
			}
			var ownerGalleryID int64
			switch value.kind {
			case "GALLERY":
				if err := s.Database.QueryRowContext(ctx, `SELECT id FROM galleries WHERE set_id=?`, value.uuid).Scan(&ownerGalleryID); err != nil {
					return false
				}
			case "GALLERY_ITEM":
				if err := s.Database.QueryRowContext(ctx, `SELECT gallery_id FROM gallery_items WHERE item_uuid=?`, value.uuid).Scan(&ownerGalleryID); err != nil {
					return false
				}
			case "EXTERNAL_LINK":
				if err := s.Database.QueryRowContext(ctx, `SELECT gallery_id FROM gallery_external_links WHERE link_uuid=?`, value.uuid).Scan(&ownerGalleryID); err != nil {
					return false
				}
			}
			if ownerGalleryID != *rebuild.GalleryID {
				return false
			}
		}
	}
	return true
}

func portableGalleryTitle(document manifest.GalleryDocument, sourcePath string, kind gallery.SourceType) string {
	if document.Title.Present && !document.Title.Null {
		return document.Title.Value
	}
	if kind == gallery.SourceTypeArchive {
		return archivefile.BaseName(sourcePath)
	}
	return filepath.Base(sourcePath)
}
