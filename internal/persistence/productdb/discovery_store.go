package productdb

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/discovery"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/slug"
	"github.com/stashapp/stash/internal/sourcescan"
)

type RecognitionRuleStore struct {
	db *sql.DB
}

func (db *Database) RecognitionRules() *RecognitionRuleStore {
	return &RecognitionRuleStore{db: db.DB}
}

type CreateRecognitionRuleInput struct {
	LibraryID       int64
	Name            string
	Kind            discovery.RuleKind
	Enabled         bool
	AutoCreateDraft bool
	Order           int
	Pattern         string
	FixedDepth      int
}

func (s *RecognitionRuleStore) Create(
	ctx context.Context,
	input CreateRecognitionRuleInput,
	now time.Time,
) (discovery.Rule, error) {
	rule := discovery.Rule{
		Name: input.Name,
		Kind: input.Kind, Enabled: input.Enabled, AutoCreateDraft: input.AutoCreateDraft,
		Order: input.Order, Pattern: input.Pattern, FixedDepth: input.FixedDepth,
	}
	if err := discovery.ValidateRule(rule); err != nil {
		return discovery.Rule{}, err
	}
	if input.Name == "" || len([]rune(input.Name)) > 300 {
		return discovery.Rule{}, errors.New("recognition rule name must contain 1 to 300 characters")
	}

	var pattern interface{}
	var depth interface{}
	if input.Kind == discovery.RuleKindPathTemplate {
		pattern = input.Pattern
	}
	if input.Kind == discovery.RuleKindFixedDepth {
		depth = input.FixedDepth
	}
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_recognition_rules (
			library_id, name, rule_kind, enabled, auto_create_draft,
			sort_order, pattern, fixed_depth, created_at_utc, updated_at_utc
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.LibraryID, input.Name, input.Kind, input.Enabled, input.AutoCreateDraft,
		input.Order, pattern, depth, timestamp, timestamp)
	if err != nil {
		return discovery.Rule{}, err
	}
	rule.ID, err = result.LastInsertId()
	return rule, err
}

func (s *RecognitionRuleStore) List(ctx context.Context, libraryID int64) ([]discovery.Rule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, rule_kind, enabled, auto_create_draft, sort_order, pattern, fixed_depth
		FROM gallery_recognition_rules WHERE library_id = ?
		ORDER BY sort_order, id
	`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []discovery.Rule
	for rows.Next() {
		var rule discovery.Rule
		var enabled, autoCreate int
		var pattern sql.NullString
		var depth sql.NullInt64
		if err := rows.Scan(
			&rule.ID, &rule.Name, &rule.Kind, &enabled, &autoCreate, &rule.Order, &pattern, &depth,
		); err != nil {
			return nil, err
		}
		rule.Enabled = enabled == 1
		rule.AutoCreateDraft = autoCreate == 1
		rule.Pattern = pattern.String
		rule.FixedDepth = int(depth.Int64)
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

type ObservedDirectory struct {
	RelativePath     string
	SourceType       gallery.SourceType
	HasValidManifest bool
	ManifestSetID    string
	HasRootMarker    bool
	MediaCount       int
	HasConflict      bool
	OverLimit        bool
}

type Candidate struct {
	ID              int64
	RootPath        string
	SourceType      gallery.SourceType
	Method          string
	GalleryID       *int64
	ManifestSetID   string
	RebindGalleryID *int64
	Status          string
	RuleID          *int64
	AutoCreateDraft bool
	HasConflict     bool
	OverLimit       bool
	MediaCount      int
	Suggestions     []discovery.Suggestion
}

type UnassignedDiagnostic struct {
	ParentPath string
	MediaCount int
}

type DiscoverySnapshot struct {
	ID          int64
	LibraryID   int64
	CompletedAt time.Time
	Candidates  []Candidate
	Unassigned  []UnassignedDiagnostic
}

type CandidateDiscoveryStore struct {
	db *sql.DB
}

func (db *Database) CandidateDiscovery() *CandidateDiscoveryStore {
	return &CandidateDiscoveryStore{db: db.DB}
}

// DiscoverFilesystem performs the explicit, read-only first phase of import.
// It never creates GalleryItems or accepts metadata suggestions. Configured
// child library roots are hard traversal boundaries.
func (s *CandidateDiscoveryStore) DiscoverFilesystem(ctx context.Context, libraryID int64, now time.Time) (DiscoverySnapshot, error) {
	mediaLibrary, err := findLibrary(ctx, s.db, libraryID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	if !mediaLibrary.Enabled {
		return DiscoverySnapshot{}, errors.New("media library is disabled")
	}
	rootInfo, err := os.Lstat(mediaLibrary.RootPath)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return DiscoverySnapshot{}, errors.New("media library root must be an accessible real directory")
	}
	children, err := (&LibraryStore{db: s.db}).ChildBoundaries(ctx, libraryID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	childRoots := make(map[string]struct{}, len(children))
	for _, child := range children {
		childRoots[filepath.Clean(child.RootPath)] = struct{}{}
	}
	runtimeSettings, err := (&SettingsStore{db: s.db}).Find(ctx)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	archiveLimits := archivecheck.Limits{MaxEntries: runtimeSettings.ArchiveMaxEntries, MaxEntryUncompressed: uint64(runtimeSettings.ArchiveMaxEntryBytes), MaxTotalUncompressed: uint64(runtimeSettings.ArchiveMaxTotalBytes), MaxCompressionRatio: runtimeSettings.ArchiveMaxCompressionRatio, MaxImagePixels: uint64(runtimeSettings.ArchiveMaxImagePixels)}

	directoryCounts := make(map[string]int)
	markerDirectories := make(map[string]bool)
	manifestDirectories := make(map[string]bool)
	var archives []ObservedDirectory
	err = filepath.WalkDir(mediaLibrary.RootPath, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		cleaned := filepath.Clean(filename)
		if cleaned != filepath.Clean(mediaLibrary.RootPath) {
			if _, boundary := childRoots[cleaned]; boundary && entry.IsDir() {
				return filepath.SkipDir
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		base := strings.ToLower(entry.Name())
		directory := filepath.Dir(filename)
		if base == ".cosplay-root" {
			markerDirectories[directory] = true
			return nil
		}
		if base == ".cosplay.json" {
			manifestDirectories[directory] = true
			return nil
		}
		extension := strings.ToLower(filepath.Ext(filename))
		if extension == ".zip" || extension == ".cbz" {
			observation, err := observeArchiveCandidate(mediaLibrary.RootPath, filename, archiveLimits)
			if err != nil {
				return err
			}
			if observation.MediaCount > 0 {
				archives = append(archives, observation)
			}
			return nil
		}
		if sourcescan.IsSupportedMediaPath(filename) {
			directoryCounts[directory]++
		}
		return nil
	})
	if err != nil {
		return DiscoverySnapshot{}, err
	}

	root := filepath.Clean(mediaLibrary.RootPath)
	directories := make(map[string]struct{}, len(directoryCounts)+len(markerDirectories)+len(manifestDirectories))
	for directory := range directoryCounts {
		directories[directory] = struct{}{}
	}
	for directory := range markerDirectories {
		directories[directory] = struct{}{}
	}
	for directory := range manifestDirectories {
		directories[directory] = struct{}{}
	}
	observations := make([]ObservedDirectory, 0, len(directories)+len(archives))
	for directory := range directories {
		count := directoryCounts[directory]
		if markerDirectories[directory] || manifestDirectories[directory] {
			count = descendantMediaCount(directory, directoryCounts)
		}
		if count == 0 {
			continue
		}
		relative, err := filepath.Rel(root, directory)
		if err != nil || relative == "." {
			continue
		}
		observation := ObservedDirectory{RelativePath: filepath.ToSlash(relative), SourceType: gallery.SourceTypeDirectory, HasRootMarker: markerDirectories[directory], MediaCount: count, OverLimit: count > 1000}
		if manifestDirectories[directory] {
			_, observation.HasValidManifest, observation.ManifestSetID = readManifestIdentity(filepath.Join(directory, ".cosplay.json"))
			observation.HasConflict = !observation.HasValidManifest
		}
		observations = append(observations, observation)
	}
	observations = append(observations, archives...)
	sort.Slice(observations, func(i, j int) bool { return observations[i].RelativePath < observations[j].RelativePath })
	return s.CommitSnapshot(ctx, libraryID, observations, now)
}

func descendantMediaCount(root string, counts map[string]int) int {
	total := 0
	for directory, count := range counts {
		if pathWithin(root, directory) {
			total += count
		}
	}
	return total
}

func observeArchiveCandidate(libraryRoot, filename string, limits archivecheck.Limits) (ObservedDirectory, error) {
	validation, err := archivecheck.ValidateFile(filename, limits)
	if err != nil {
		return ObservedDirectory{}, err
	}
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return ObservedDirectory{}, err
	}
	count := 0
	for _, entry := range reader.File {
		if !entry.FileInfo().IsDir() && sourcescan.IsSupportedMediaPath(entry.Name) {
			count++
		}
	}
	if err := reader.Close(); err != nil {
		return ObservedDirectory{}, err
	}
	relative, err := filepath.Rel(libraryRoot, filename)
	if err != nil {
		return ObservedDirectory{}, err
	}
	conflict := false
	for _, issue := range validation.Issues {
		if issue.Code != "UNSUPPORTED_ARCHIVE_MEDIA" {
			conflict = true
			break
		}
	}
	observation := ObservedDirectory{RelativePath: filepath.ToSlash(relative), SourceType: gallery.SourceTypeArchive, MediaCount: count, HasConflict: conflict, OverLimit: count > 1000}
	if exists, valid, setID := readManifestIdentity(filename + ".cosplay.json"); exists {
		observation.HasValidManifest, observation.ManifestSetID = valid, setID
		observation.HasConflict = observation.HasConflict || !valid
	}
	return observation, nil
}

func readManifestIdentity(filename string) (exists, valid bool, setID string) {
	file, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, ""
	}
	if err != nil {
		return true, false, ""
	}
	defer file.Close()
	document, err := manifest.ParseGallery(file)
	if err != nil {
		return true, false, ""
	}
	return true, true, document.SetID
}

func (s *CandidateDiscoveryStore) IgnoreSource(
	ctx context.Context,
	libraryID *int64,
	setID *string,
	sourcePath string,
	reason string,
	now time.Time,
) error {
	absolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return err
	}
	absolute = filepath.Clean(absolute)
	if len([]rune(absolute)) > 4096 || len([]rune(reason)) > 1000 {
		return errors.New("ignored GallerySource path or reason exceeds its limit")
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO ignored_gallery_sources (
			library_id, set_id, source_path, reason, created_at_utc
		) VALUES (?, ?, ?, ?, ?)
	`, libraryID, setID, absolute, reason, formatTime(normalisedTime(now)))
	return err
}

type pendingCandidate struct {
	root            string
	sourceType      gallery.SourceType
	method          string
	ruleID          *int64
	autoCreate      bool
	conflict        bool
	overLimit       bool
	mediaCount      int
	manifestSetID   string
	rebindGalleryID *int64
	status          string
	suggestions     []discovery.Suggestion
}

func (s *CandidateDiscoveryStore) CommitSnapshot(
	ctx context.Context,
	libraryID int64,
	observations []ObservedDirectory,
	now time.Time,
) (DiscoverySnapshot, error) {
	mediaLibrary, err := findLibrary(ctx, s.db, libraryID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	rules, err := (&RecognitionRuleStore{db: s.db}).List(ctx, libraryID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	boundaries, err := s.boundSourcePaths(ctx, libraryID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	ignored, err := s.ignoredPaths(ctx, libraryID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	ignoredSetIDs, err := s.ignoredSetIDs(ctx)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	setIdentities, err := s.gallerySetIdentities(ctx)
	if err != nil {
		return DiscoverySnapshot{}, err
	}

	candidates := make(map[string]*pendingCandidate)
	unassigned := make(map[string]int)
	for _, observed := range observations {
		if observed.MediaCount <= 0 {
			continue
		}
		if observed.SourceType == "" {
			observed.SourceType = gallery.SourceTypeDirectory
		}
		if observed.SourceType != gallery.SourceTypeDirectory && observed.SourceType != gallery.SourceTypeArchive {
			return DiscoverySnapshot{}, errors.New("observed source type must be DIRECTORY or ARCHIVE")
		}
		if observed.ManifestSetID != "" {
			if _, err := portableid.Parse(observed.ManifestSetID); err != nil {
				return DiscoverySnapshot{}, fmt.Errorf("invalid observed Manifest set_id: %w", err)
			}
			if !observed.HasValidManifest {
				return DiscoverySnapshot{}, errors.New("Manifest set_id requires a valid Manifest")
			}
			if _, ignored := ignoredSetIDs[observed.ManifestSetID]; ignored {
				continue
			}
		}
		relative, err := discovery.NormalizeRelativeDirectory(observed.RelativePath)
		if err != nil {
			return DiscoverySnapshot{}, err
		}
		absolute := filepath.Join(mediaLibrary.RootPath, filepath.FromSlash(relative))
		if withinAny(boundaries, absolute) || withinAny(ignored, absolute) {
			continue
		}

		var match *discovery.Match
		if observed.HasValidManifest {
			match = &discovery.Match{Root: relative, Kind: "MANIFEST"}
		} else {
			match, err = discovery.MatchDirectory(relative, observed.HasRootMarker, rules)
			if err != nil {
				return DiscoverySnapshot{}, err
			}
		}
		if match == nil {
			unassigned[absolute] += observed.MediaCount
			continue
		}

		rootPath := filepath.Join(mediaLibrary.RootPath, filepath.FromSlash(match.Root))
		if withinAny(boundaries, rootPath) || withinAny(ignored, rootPath) {
			continue
		}
		candidate := candidates[rootPath]
		if candidate == nil {
			candidate = &pendingCandidate{
				root: rootPath, sourceType: observed.SourceType,
				method: string(match.Kind), autoCreate: match.AutoCreate,
				suggestions:   append([]discovery.Suggestion(nil), match.Suggestions...),
				manifestSetID: observed.ManifestSetID, status: "PENDING",
			}
			if identity, found := setIdentities[observed.ManifestSetID]; observed.ManifestSetID != "" && found && identity.SourcePath != rootPath {
				value := identity.GalleryID
				candidate.rebindGalleryID = &value
				candidate.status = "SOURCE_REBIND_CANDIDATE"
				candidate.conflict = identity.SourceAvailable
				candidate.autoCreate = false
			}
			if match.RuleID != 0 {
				value := match.RuleID
				candidate.ruleID = &value
			}
			candidates[rootPath] = candidate
		} else if candidate.sourceType != observed.SourceType {
			candidate.conflict = true
		}
		candidate.mediaCount += observed.MediaCount
		candidate.conflict = candidate.conflict || observed.HasConflict
		candidate.overLimit = candidate.overLimit || observed.OverLimit || candidate.mediaCount > 1000
	}
	for unassignedPath := range unassigned {
		for candidateRoot := range candidates {
			if pathWithin(candidateRoot, unassignedPath) {
				delete(unassigned, unassignedPath)
				break
			}
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()
	timestamp := formatTime(normalisedTime(now))
	result, err := tx.ExecContext(ctx, `
		INSERT INTO discovery_snapshots (library_id, completed_at_utc) VALUES (?, ?)
	`, libraryID, timestamp)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	snapshotID, err := result.LastInsertId()
	if err != nil {
		return DiscoverySnapshot{}, err
	}

	roots := make([]string, 0, len(candidates))
	for root := range candidates {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	for _, root := range roots {
		candidate := candidates[root]
		result, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_candidates (
				snapshot_id, library_id, root_path, source_type, recognition_method, rule_id,
				manifest_set_id, rebind_gallery_id, auto_create_draft, has_conflict,
				over_limit, media_count, status, created_at_utc
			) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)
		`, snapshotID, libraryID, candidate.root, candidate.sourceType, candidate.method, candidate.ruleID,
			candidate.manifestSetID, candidate.rebindGalleryID, candidate.autoCreate, candidate.conflict,
			candidate.overLimit, candidate.mediaCount, candidate.status, timestamp)
		if err != nil {
			return DiscoverySnapshot{}, err
		}
		candidateID, err := result.LastInsertId()
		if err != nil {
			return DiscoverySnapshot{}, err
		}
		for _, suggestion := range candidate.suggestions {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO gallery_candidate_suggestions (candidate_id, field_name, value)
				VALUES (?, ?, ?)
			`, candidateID, suggestion.Field, suggestion.Value); err != nil {
				return DiscoverySnapshot{}, err
			}
		}
	}

	diagnosticPaths := make([]string, 0, len(unassigned))
	for parent := range unassigned {
		diagnosticPaths = append(diagnosticPaths, parent)
	}
	sort.Strings(diagnosticPaths)
	for _, parent := range diagnosticPaths {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO unassigned_media_diagnostics (
				snapshot_id, library_id, parent_path, media_count
			) VALUES (?, ?, ?, ?)
		`, snapshotID, libraryID, parent, unassigned[parent]); err != nil {
			return DiscoverySnapshot{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return DiscoverySnapshot{}, err
	}
	for _, root := range roots {
		candidate := candidates[root]
		if candidate.status == "PENDING" && candidate.autoCreate && !candidate.conflict && !candidate.overLimit {
			var candidateID int64
			if err := s.db.QueryRowContext(ctx, `
				SELECT id FROM gallery_candidates WHERE snapshot_id = ? AND root_path = ?
			`, snapshotID, root).Scan(&candidateID); err != nil {
				return DiscoverySnapshot{}, err
			}
			if _, err := s.ImportCandidate(ctx, candidateID, now); err != nil {
				return DiscoverySnapshot{}, err
			}
		}
	}
	return s.FindSnapshot(ctx, snapshotID)
}

func (s *CandidateDiscoveryStore) FindSnapshot(ctx context.Context, snapshotID int64) (DiscoverySnapshot, error) {
	var result DiscoverySnapshot
	var completedAt string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, library_id, completed_at_utc FROM discovery_snapshots WHERE id = ?
	`, snapshotID).Scan(&result.ID, &result.LibraryID, &completedAt); err != nil {
		return DiscoverySnapshot{}, err
	}
	var err error
	if result.CompletedAt, err = parseTime(completedAt); err != nil {
		return DiscoverySnapshot{}, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, root_path, source_type, recognition_method, gallery_id, status,
			rule_id, manifest_set_id, rebind_gallery_id, auto_create_draft,
			has_conflict, over_limit, media_count
		FROM gallery_candidates WHERE snapshot_id = ? ORDER BY root_path
	`, snapshotID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	for rows.Next() {
		var candidate Candidate
		var ruleID sql.NullInt64
		var galleryID sql.NullInt64
		var manifestSetID sql.NullString
		var rebindGalleryID sql.NullInt64
		var autoCreate, conflict, overLimit int
		if err := rows.Scan(
			&candidate.ID, &candidate.RootPath, &candidate.SourceType,
			&candidate.Method, &galleryID, &candidate.Status, &ruleID,
			&manifestSetID, &rebindGalleryID, &autoCreate, &conflict,
			&overLimit, &candidate.MediaCount,
		); err != nil {
			rows.Close()
			return DiscoverySnapshot{}, err
		}
		if ruleID.Valid {
			value := ruleID.Int64
			candidate.RuleID = &value
		}
		if galleryID.Valid {
			value := galleryID.Int64
			candidate.GalleryID = &value
		}
		candidate.ManifestSetID = manifestSetID.String
		if rebindGalleryID.Valid {
			value := rebindGalleryID.Int64
			candidate.RebindGalleryID = &value
		}
		candidate.AutoCreateDraft = autoCreate == 1
		candidate.HasConflict = conflict == 1
		candidate.OverLimit = overLimit == 1
		result.Candidates = append(result.Candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return DiscoverySnapshot{}, err
	}
	for index := range result.Candidates {
		suggestionRows, err := s.db.QueryContext(ctx, `
			SELECT field_name, value FROM gallery_candidate_suggestions
			WHERE candidate_id = ? ORDER BY id
		`, result.Candidates[index].ID)
		if err != nil {
			return DiscoverySnapshot{}, err
		}
		for suggestionRows.Next() {
			var suggestion discovery.Suggestion
			if err := suggestionRows.Scan(&suggestion.Field, &suggestion.Value); err != nil {
				suggestionRows.Close()
				return DiscoverySnapshot{}, err
			}
			result.Candidates[index].Suggestions = append(result.Candidates[index].Suggestions, suggestion)
		}
		if err := suggestionRows.Close(); err != nil {
			return DiscoverySnapshot{}, err
		}
	}

	diagnosticRows, err := s.db.QueryContext(ctx, `
		SELECT parent_path, media_count FROM unassigned_media_diagnostics
		WHERE snapshot_id = ? ORDER BY parent_path
	`, snapshotID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	defer diagnosticRows.Close()
	for diagnosticRows.Next() {
		var diagnostic UnassignedDiagnostic
		if err := diagnosticRows.Scan(&diagnostic.ParentPath, &diagnostic.MediaCount); err != nil {
			return DiscoverySnapshot{}, err
		}
		result.Unassigned = append(result.Unassigned, diagnostic)
	}
	return result, diagnosticRows.Err()
}

func (s *CandidateDiscoveryStore) LatestSnapshot(ctx context.Context, libraryID int64) (DiscoverySnapshot, error) {
	var snapshotID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM discovery_snapshots WHERE library_id=? ORDER BY id DESC LIMIT 1`, libraryID).Scan(&snapshotID)
	if errors.Is(err, sql.ErrNoRows) {
		return DiscoverySnapshot{LibraryID: libraryID}, nil
	}
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	return s.FindSnapshot(ctx, snapshotID)
}

func (s *CandidateDiscoveryStore) ImportCandidate(
	ctx context.Context,
	candidateID int64,
	now time.Time,
) (gallery.Gallery, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.Gallery{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var libraryID int64
	var rootPath string
	var sourceType gallery.SourceType
	var status string
	var manifestSetID sql.NullString
	var conflict, overLimit int
	if err := tx.QueryRowContext(ctx, `
		SELECT library_id, root_path, source_type, status, has_conflict, over_limit, manifest_set_id
		FROM gallery_candidates WHERE id = ?
	`, candidateID).Scan(&libraryID, &rootPath, &sourceType, &status, &conflict, &overLimit, &manifestSetID); err != nil {
		return gallery.Gallery{}, err
	}
	if status != "PENDING" {
		return gallery.Gallery{}, fmt.Errorf("candidate %d is already %s", candidateID, status)
	}
	if conflict == 1 || overLimit == 1 {
		return gallery.Gallery{}, errors.New("conflicting or over-limit candidate cannot create a DRAFT")
	}

	setID := manifestSetID.String
	if setID == "" {
		setID = portableid.New()
	}
	timestamp := normalisedTime(now)
	if _, err := registerPortableUUID(ctx, tx, setID, portableid.KindGallery, timestamp); err != nil {
		return gallery.Gallery{}, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO galleries (
			set_id, slug, state, created_at_utc, updated_at_utc
		) VALUES (?, ?, 'DRAFT', ?, ?)
	`, setID, slug.FromName("", setID), formatTime(timestamp), formatTime(timestamp))
	if err != nil {
		return gallery.Gallery{}, err
	}
	galleryID, err := result.LastInsertId()
	if err != nil {
		return gallery.Gallery{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_sources (
			gallery_id, library_id, source_type, source_path, availability_state,
			reconcile_state, created_at_utc, updated_at_utc
		) VALUES (?, ?, ?, ?, 'AVAILABLE', 'NEVER_SCANNED', ?, ?)
	`, galleryID, libraryID, sourceType, rootPath, formatTime(timestamp), formatTime(timestamp)); err != nil {
		return gallery.Gallery{}, err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT field_name, value FROM gallery_candidate_suggestions
		WHERE candidate_id = ? ORDER BY id
	`, candidateID)
	if err != nil {
		return gallery.Gallery{}, err
	}
	var suggestions []discovery.Suggestion
	for rows.Next() {
		var suggestion discovery.Suggestion
		if err := rows.Scan(&suggestion.Field, &suggestion.Value); err != nil {
			rows.Close()
			return gallery.Gallery{}, err
		}
		suggestions = append(suggestions, suggestion)
	}
	if err := rows.Close(); err != nil {
		return gallery.Gallery{}, err
	}
	for _, suggestion := range suggestions {
		switch suggestion.Field {
		case "coser", "work", "character":
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO gallery_identity_suggestions (
					gallery_id, suggestion_kind, value, status, created_at_utc
				) VALUES (?, upper(?), ?, 'PENDING', ?)
			`, galleryID, suggestion.Field, suggestion.Value, formatTime(timestamp)); err != nil {
				return gallery.Gallery{}, err
			}
		case "title", "year", "month":
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO gallery_metadata_suggestions (
					gallery_id, suggestion_kind, value, status, created_at_utc
				) VALUES (?, upper(?), ?, 'PENDING', ?)
			`, galleryID, suggestion.Field, suggestion.Value, formatTime(timestamp)); err != nil {
				return gallery.Gallery{}, err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE gallery_candidates SET status = 'IMPORTED', gallery_id = ? WHERE id = ?
	`, galleryID, candidateID); err != nil {
		return gallery.Gallery{}, err
	}
	created, err := findGallery(ctx, tx, galleryID)
	if err != nil {
		return gallery.Gallery{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.Gallery{}, err
	}
	if manifestSetID.Valid {
		manifestPath, pathErr := manifest.GalleryPath(sourceType, rootPath)
		if pathErr != nil {
			return gallery.Gallery{}, pathErr
		}
		if _, statErr := os.Lstat(manifestPath); statErr == nil {
			pulled, pullErr := (&ManifestStore{db: s.db}).PullGallery(ctx, galleryID, created.MetadataRevision, now)
			if pullErr != nil {
				return gallery.Gallery{}, pullErr
			}
			if pulled.Status == ManifestConflict {
				return findGallery(ctx, s.db, galleryID)
			}
			return findGallery(ctx, s.db, galleryID)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return gallery.Gallery{}, statErr
		}
	}
	return created, nil
}

// ConfirmSourceRebind performs the explicit identity-preserving source move
// selected by the owner. Discovery never calls this automatically. When the
// old source is still available the caller must separately confirm that the
// duplicate set_id is intentional before the move is accepted.
func (s *CandidateDiscoveryStore) ConfirmSourceRebind(
	ctx context.Context,
	candidateID int64,
	allowAccessibleDuplicate bool,
	now time.Time,
) (gallery.Source, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.Source{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		libraryID       int64
		rootPath        string
		sourceType      gallery.SourceType
		status          string
		rebindGalleryID sql.NullInt64
		conflict        int
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT library_id, root_path, source_type, status, rebind_gallery_id, has_conflict
		FROM gallery_candidates WHERE id = ?
	`, candidateID).Scan(
		&libraryID, &rootPath, &sourceType, &status, &rebindGalleryID, &conflict,
	); err != nil {
		return gallery.Source{}, err
	}
	if status != "SOURCE_REBIND_CANDIDATE" || !rebindGalleryID.Valid {
		return gallery.Source{}, errors.New("candidate is not a source rebind candidate")
	}
	if conflict == 1 && !allowAccessibleDuplicate {
		return gallery.Source{}, errors.New("duplicate set_id has two accessible sources and requires explicit confirmation")
	}

	source, err := findGallerySourceByGallery(ctx, tx, rebindGalleryID.Int64)
	if err != nil {
		return gallery.Source{}, err
	}
	oldPath := source.Path
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `
		UPDATE gallery_sources SET library_id = ?, source_type = ?, source_path = ?,
			availability_state = 'AVAILABLE', reconcile_state = 'NEEDS_RESCAN',
			over_limit = 0, updated_at_utc = ?
		WHERE id = ?
	`, libraryID, sourceType, rootPath, timestamp, source.ID); err != nil {
		return gallery.Source{}, fmt.Errorf("confirming GallerySource rebind: %w", err)
	}
	if oldPath != rootPath {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ignored_gallery_sources (library_id, source_path, reason, created_at_utc)
			VALUES (?, ?, 'SOURCE_REBOUND', ?)
			ON CONFLICT(source_path) DO NOTHING
		`, source.LibraryID, oldPath, timestamp); err != nil {
			return gallery.Source{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE gallery_candidates SET status = 'REBOUND', gallery_id = ? WHERE id = ?
	`, rebindGalleryID.Int64, candidateID); err != nil {
		return gallery.Source{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE galleries SET scan_revision = scan_revision + 1, scrubber_revision = scrubber_revision + 1 WHERE id = ?
	`, rebindGalleryID.Int64); err != nil {
		return gallery.Source{}, err
	}
	updated, err := findSource(ctx, tx, source.ID)
	if err != nil {
		return gallery.Source{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.Source{}, err
	}
	return updated, nil
}

func (s *CandidateDiscoveryStore) boundSourcePaths(ctx context.Context, libraryID int64) ([]string, error) {
	return queryPaths(ctx, s.db, `SELECT source_path FROM gallery_sources WHERE library_id = ?`, libraryID)
}

func (s *CandidateDiscoveryStore) ignoredPaths(ctx context.Context, libraryID int64) ([]string, error) {
	return queryPaths(ctx, s.db, `
		SELECT source_path FROM ignored_gallery_sources
		WHERE library_id = ? OR library_id IS NULL
	`, libraryID)
}

func (s *CandidateDiscoveryStore) ignoredSetIDs(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT set_id FROM ignored_gallery_sources WHERE set_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]struct{})
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		result[value] = struct{}{}
	}
	return result, rows.Err()
}

type gallerySetIdentity struct {
	GalleryID       int64
	SourcePath      string
	SourceAvailable bool
}

func (s *CandidateDiscoveryStore) gallerySetIdentities(ctx context.Context) (map[string]gallerySetIdentity, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT gallery.set_id, gallery.id, COALESCE(source.source_path, ''),
			COALESCE(source.availability_state = 'AVAILABLE', 0)
		FROM galleries gallery
		LEFT JOIN gallery_sources source ON source.gallery_id = gallery.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]gallerySetIdentity)
	for rows.Next() {
		var setID string
		var identity gallerySetIdentity
		var available int
		if err := rows.Scan(&setID, &identity.GalleryID, &identity.SourcePath, &available); err != nil {
			return nil, err
		}
		identity.SourceAvailable = available == 1
		result[setID] = identity
	}
	return result, rows.Err()
}

func findGallerySourceByGallery(ctx context.Context, queryer galleryQueryer, galleryID int64) (gallery.Source, error) {
	var sourceID int64
	if err := queryer.QueryRowContext(ctx, `SELECT id FROM gallery_sources WHERE gallery_id = ?`, galleryID).Scan(&sourceID); err != nil {
		return gallery.Source{}, err
	}
	return findSource(ctx, queryer, sourceID)
}

func queryPaths(ctx context.Context, db *sql.DB, query string, argument int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, argument)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		paths = append(paths, value)
	}
	return paths, rows.Err()
}

func withinAny(roots []string, candidate string) bool {
	for _, root := range roots {
		if pathWithin(root, candidate) {
			return true
		}
	}
	return false
}
