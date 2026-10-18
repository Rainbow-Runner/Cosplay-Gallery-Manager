package productserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/archivefile"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/manifestidentity"
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
		resolved, resolutionErr := s.refreshPortableSourceResolution(ctx, rebuild)
		if resolutionErr != nil {
			return result, resolutionErr
		}
		rebuild = resolved
		entry.RelativeSource = rebuild.ResolvedRelativeSource
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
	if rebuild.ResolvedTargetLibraryID != nil {
		for _, candidate := range mappings {
			if candidate.TargetLibraryID != nil && *candidate.TargetLibraryID == *rebuild.ResolvedTargetLibraryID {
				mapping, ok = candidate, true
				break
			}
		}
	}
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
	if rebuild.SourceResolution == string(manifestidentity.ResolutionDuplicateAccessibleSource) {
		return prepared, "PORTABLE_DUPLICATE_ACCESSIBLE_SOURCE"
	}
	if rebuild.SourceResolution == string(manifestidentity.ResolutionUnresolved) && rebuild.ResolutionTokenHash != "" {
		return prepared, "PORTABLE_SOURCE_UNRESOLVED"
	}
	if rebuild.SourceResolution == string(manifestidentity.ResolutionRelocatedUnique) && rebuild.AdoptionTokenHash == "" {
		return prepared, "PORTABLE_SOURCE_RELOCATION_CONFIRMATION_REQUIRED"
	}
	if rebuild.ManifestStatus != "CLEAN" || rebuild.ManifestHash == "" {
		if rebuild.AdoptedManifestHash == "" {
			return prepared, "PORTABLE_MANIFEST_ADOPTION_REQUIRED"
		}
	}
	sourcePath := mapping.TargetRoot
	relativeSource := rebuild.RelativeSource
	if rebuild.ResolvedRelativeSource != "" || rebuild.SourceResolution == string(manifestidentity.ResolutionExact) || rebuild.SourceResolution == string(manifestidentity.ResolutionRelocatedUnique) {
		relativeSource = rebuild.ResolvedRelativeSource
	}
	if rebuild.LocatorStatus == "MAPPED" || relativeSource != "" {
		sourcePath = filepath.Join(mapping.TargetRoot, filepath.FromSlash(relativeSource))
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
	expectedHash := rebuild.ManifestHash
	expectedRevision := rebuild.ManifestRevision
	if rebuild.AdoptedManifestHash != "" {
		expectedHash = rebuild.AdoptedManifestHash
		expectedRevision = rebuild.AdoptedManifestRevision
	}
	if hash != expectedHash {
		return prepared, "PORTABLE_MANIFEST_CHANGED"
	}
	document, err := manifest.ParseGallery(bytes.NewReader(data))
	if err != nil || document.SetID != rebuild.SetID || document.Revision != expectedRevision {
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
	if valid, err := s.Database.ManifestCoreReferencesValid(ctx, document); err != nil || !valid {
		return prepared, "PORTABLE_CORE_REFERENCE_REVIEW"
	}
	prepared.path, prepared.doc = sourcePath, document
	return prepared, ""
}

func (s *Server) refreshPortableSourceResolution(ctx context.Context, rebuild productdb.PortableGalleryRebuild) (productdb.PortableGalleryRebuild, error) {
	candidates, err := s.Database.PortableSourceCandidates(ctx, rebuild.ImportID, rebuild.SetID)
	if err != nil {
		return rebuild, err
	}
	complete, err := s.Database.PortableMappingsHaveCompleteDiscovery(ctx, rebuild.ImportID)
	if err != nil || !complete {
		return rebuild, err
	}
	observed := make([]manifestidentity.ObservedSource, 0, len(candidates))
	for _, candidate := range candidates {
		observed = append(observed, manifestidentity.ObservedSource{SnapshotID: strconv.FormatInt(candidate.SnapshotID, 10), LibraryID: strconv.FormatInt(candidate.LibraryID, 10),
			SetID: candidate.SetID, SourceType: string(candidate.SourceType), RelativeSource: candidate.RelativeSource,
			ManifestSchema: candidate.ManifestSchema, Revision: candidate.ManifestRevision, ManifestHash: candidate.ManifestHash})
	}
	exported := rebuild.ExportedRelativeSource
	if exported == "" {
		exported = rebuild.RelativeSource
	}
	resolution := manifestidentity.ResolveSource(manifestidentity.ExpectedSource{SetID: rebuild.SetID, SourceType: string(rebuild.SourceType),
		ExportedRelativeSource: exported, ManifestSchema: rebuild.ManifestSchema, ManifestRevision: rebuild.ManifestRevision, ManifestHash: rebuild.ManifestHash}, observed)
	var targetLibraryID, snapshotID *int64
	if resolution.ResolvedLibraryID != "" {
		value, parseErr := strconv.ParseInt(resolution.ResolvedLibraryID, 10, 64)
		if parseErr != nil {
			return rebuild, parseErr
		}
		targetLibraryID = &value
	}
	if resolution.SnapshotID != "" {
		value, parseErr := strconv.ParseInt(resolution.SnapshotID, 10, 64)
		if parseErr != nil {
			return rebuild, parseErr
		}
		snapshotID = &value
	}
	manifestHash := ""
	if len(resolution.Candidates) == 1 {
		manifestHash = resolution.Candidates[0].ManifestHash
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{rebuild.ImportID, rebuild.SetID, string(resolution.Resolution), resolution.SnapshotID,
		resolution.ResolvedLibraryID, resolution.ResolvedRelativeSource, manifestHash}, "\x00")))
	token := hex.EncodeToString(digest[:])
	if err := s.Database.SetPortableSourceResolution(ctx, rebuild.ImportID, rebuild.SetID, string(resolution.Resolution), targetLibraryID, snapshotID,
		resolution.ResolvedRelativeSource, manifestHash, token, time.Now()); err != nil {
		return rebuild, err
	}
	rebuild.ResolvedTargetLibraryID, rebuild.ResolutionSnapshotID = targetLibraryID, snapshotID
	rebuild.ResolvedRelativeSource, rebuild.SourceResolution = resolution.ResolvedRelativeSource, string(resolution.Resolution)
	rebuild.ResolutionManifestHash, rebuild.ResolutionTokenHash = manifestHash, token
	return rebuild, nil
}

// AdoptPortableGalleryManifests records owner-authorized evidence for current
// Manifests or relocated roots. It does not create a Gallery or claim a UUID;
// the ordinary rebuild phase revalidates the evidence before doing either.
func (s *Server) AdoptPortableGalleryManifests(ctx context.Context, importID string, setIDs []string) (int, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if _, err := portableid.Parse(importID); err != nil {
		return 0, err
	}
	if len(setIDs) == 0 || len(setIDs) > 100 {
		return 0, errors.New("portable Manifest adoption selection must contain 1 to 100 Galleries")
	}
	rebuilds, err := s.Database.ListPortableGalleryRebuilds(ctx, importID)
	if err != nil {
		return 0, err
	}
	selected := make(map[string]struct{}, len(setIDs))
	for _, setID := range setIDs {
		if _, err := portableid.Parse(setID); err != nil {
			return 0, err
		}
		if _, duplicate := selected[setID]; duplicate {
			return 0, errors.New("portable Manifest adoption selection contains duplicates")
		}
		selected[setID] = struct{}{}
	}
	mappings, err := s.Database.ListPortableLibraryMappings(ctx, importID)
	if err != nil {
		return 0, err
	}
	limits, err := s.portableArchiveLimits(ctx)
	if err != nil {
		return 0, err
	}
	adopted := 0
	for _, rebuild := range rebuilds {
		if _, ok := selected[rebuild.SetID]; !ok {
			continue
		}
		delete(selected, rebuild.SetID)
		rebuild, err = s.refreshPortableSourceResolution(ctx, rebuild)
		if err != nil {
			return adopted, err
		}
		if rebuild.SourceResolution != string(manifestidentity.ResolutionExact) && rebuild.SourceResolution != string(manifestidentity.ResolutionRelocatedUnique) || rebuild.ResolvedTargetLibraryID == nil || rebuild.ResolutionTokenHash == "" {
			return adopted, errors.New("portable Gallery source is not uniquely resolved")
		}
		var mapping *productdb.PortableLibraryMapping
		for index := range mappings {
			if mappings[index].TargetLibraryID != nil && *mappings[index].TargetLibraryID == *rebuild.ResolvedTargetLibraryID {
				mapping = &mappings[index]
				break
			}
		}
		if mapping == nil || mapping.TargetRoot == "" {
			return adopted, errors.New("portable Gallery target library is unavailable")
		}
		sourcePath := filepath.Join(mapping.TargetRoot, filepath.FromSlash(rebuild.ResolvedRelativeSource))
		if err := validatePortableSourcePath(mapping.TargetRoot, sourcePath, rebuild.SourceType); err != nil {
			return adopted, errors.New("portable Gallery resolved source is unavailable")
		}
		manifestPath, err := manifest.GalleryPath(rebuild.SourceType, sourcePath)
		if err != nil {
			return adopted, err
		}
		data, hash, err := manifest.ReadFile(manifestPath, manifest.MaxGalleryBytes)
		if err != nil || hash != rebuild.ResolutionManifestHash {
			return adopted, errors.New("portable Manifest changed after source resolution")
		}
		document, err := manifest.ParseGallery(bytes.NewReader(data))
		if err != nil || document.SetID != rebuild.SetID {
			return adopted, errors.New("portable Manifest identity is invalid")
		}
		var scan sourcescan.Result
		if rebuild.SourceType == gallery.SourceTypeArchive {
			scan, err = sourcescan.ScanArchive(ctx, sourcePath, limits)
		} else {
			scan, err = sourcescan.ScanDirectory(ctx, sourcePath)
		}
		if err != nil || !scan.Complete || len(scan.Observations) > 1000 {
			return adopted, errors.New("portable Gallery source scan is unsafe")
		}
		observed := make(map[string]int, len(scan.Observations))
		for _, item := range scan.Observations {
			observed[item.RelativePath]++
		}
		items, links, uniquePaths := portableManifestClaimedUUIDs(document)
		if !uniquePaths {
			return adopted, errors.New("portable Manifest item paths are not unique")
		}
		for path := range items {
			if observed[path] != 1 {
				return adopted, errors.New("portable Manifest item path is unavailable")
			}
		}
		if !s.portableClaimsValid(ctx, rebuild, items, links) {
			return adopted, errors.New("portable Manifest identities do not close over this import session")
		}
		if valid, err := s.Database.ManifestCoreReferencesValid(ctx, document); err != nil || !valid {
			return adopted, errors.New("portable Manifest core references require review")
		}
		digest := sha256.Sum256([]byte(strings.Join([]string{importID, rebuild.SetID, rebuild.ResolutionTokenHash,
			strconv.Itoa(document.SchemaVersion), strconv.FormatInt(document.Revision, 10), hash}, "\x00")))
		if err := s.Database.AdoptPortableGalleryManifest(ctx, importID, rebuild.SetID, rebuild.ResolutionTokenHash,
			document.SchemaVersion, document.Revision, hash, hex.EncodeToString(digest[:]), time.Now()); err != nil {
			return adopted, err
		}
		adopted++
	}
	if len(selected) != 0 {
		return adopted, errors.New("portable Manifest adoption selection contains an unknown Gallery")
	}
	_ = s.Database.Operations().Audit(ctx, "PORTABLE_MANIFEST_ADOPT", "PORTABLE_IMPORT", importID, "SUCCESS", "", map[string]any{"count": adopted}, time.Now())
	return adopted, nil
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
		if rebuild.State == "REBUILT" || rebuild.State == "SKIPPED" || rebuild.State == "BLOCKED" || rebuild.State == "PENDING" {
			continue
		}
		prepared, code := s.preparePortableGallery(ctx, rebuild, byKey, limits)
		if code != "" {
			if rebuild.State == "REBUILDING" {
				_ = s.Database.SetPortableGalleryRebuildIssue(ctx, importID, rebuild.SetID, code, time.Now())
			}
			continue
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
			continue
		}
		current, err := s.Database.Galleries().Find(ctx, galleryID)
		if err != nil {
			return preflight, err
		}
		if _, err := s.Database.Manifests().PullPortableGallery(ctx, galleryID, current.MetadataRevision, importID, time.Now()); err != nil {
			_ = s.Database.SetPortableGalleryRebuildIssue(ctx, importID, rebuild.SetID, "PORTABLE_MANIFEST_APPLY_FAILED", time.Now())
			continue
		}
		if _, err := s.Database.CaptureDates().EnqueueGallery(ctx, galleryID, s.VideoTools.FFprobe.Available, time.Now()); err != nil {
			_ = s.Database.SetPortableGalleryRebuildIssue(ctx, importID, rebuild.SetID, "PORTABLE_CAPTURE_DATE_QUEUE_FAILED", time.Now())
			continue
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
