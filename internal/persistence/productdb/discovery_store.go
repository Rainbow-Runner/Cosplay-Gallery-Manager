package productdb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/archivefile"
	"github.com/stashapp/stash/internal/discovery"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/slug"
	"github.com/stashapp/stash/internal/sourcescan"
	"golang.org/x/text/unicode/norm"
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

type UpdateRecognitionRuleInput struct {
	ID              int64
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
	if err := validateRecognitionRule(rule); err != nil {
		return discovery.Rule{}, err
	}

	pattern, depth := recognitionRuleParameters(rule)
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

func (s *RecognitionRuleStore) Update(
	ctx context.Context,
	input UpdateRecognitionRuleInput,
	now time.Time,
) (discovery.Rule, error) {
	rule := discovery.Rule{
		ID: input.ID, Name: input.Name,
		Kind: input.Kind, Enabled: input.Enabled, AutoCreateDraft: input.AutoCreateDraft,
		Order: input.Order, Pattern: input.Pattern, FixedDepth: input.FixedDepth,
	}
	if err := validateRecognitionRule(rule); err != nil {
		return discovery.Rule{}, err
	}

	pattern, depth := recognitionRuleParameters(rule)
	result, err := s.db.ExecContext(ctx, `
		UPDATE gallery_recognition_rules
		SET name=?, rule_kind=?, enabled=?, auto_create_draft=?,
		    sort_order=?, pattern=?, fixed_depth=?, updated_at_utc=?
		WHERE id=?
	`, rule.Name, rule.Kind, rule.Enabled, rule.AutoCreateDraft,
		rule.Order, pattern, depth, formatTime(normalisedTime(now)), rule.ID)
	if err != nil {
		return discovery.Rule{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return discovery.Rule{}, err
	}
	if affected != 1 {
		return discovery.Rule{}, sql.ErrNoRows
	}
	return rule, nil
}

func (s *RecognitionRuleStore) Delete(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM gallery_recognition_rules WHERE id=?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func validateRecognitionRule(rule discovery.Rule) error {
	if err := discovery.ValidateRule(rule); err != nil {
		return err
	}
	if rule.Name == "" || len([]rune(rule.Name)) > 300 {
		return errors.New("recognition rule name must contain 1 to 300 characters")
	}
	return nil
}

func recognitionRuleParameters(rule discovery.Rule) (pattern any, depth any) {
	if rule.Kind == discovery.RuleKindPathTemplate {
		pattern = rule.Pattern
	}
	if rule.Kind == discovery.RuleKindFixedDepth {
		depth = rule.FixedDepth
	}
	return pattern, depth
}

func (s *RecognitionRuleStore) List(ctx context.Context, libraryID int64) ([]discovery.Rule, error) {
	return listRecognitionRules(ctx, s.db, libraryID)
}

func listRecognitionRules(ctx context.Context, queryer discoveryQueryer, libraryID int64) ([]discovery.Rule, error) {
	rows, err := queryer.QueryContext(ctx, `
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
	ManifestSchema   int
	ManifestRevision int64
	ManifestHash     string
	ManifestDocument *manifest.GalleryDocument
	HasRootMarker    bool
	MarkerTitle      string
	MediaCount       int
	HasConflict      bool
	OverLimit        bool
}

type Candidate struct {
	ID                     int64
	RootPath               string
	SourceType             gallery.SourceType
	Method                 string
	GalleryID              *int64
	ManifestSetID          string
	ManifestSchema         int
	ManifestRevision       int64
	ManifestHash           string
	IdentityClassification string
	IdentityIssueCode      string
	InspectionTokenHash    string
	RebindGalleryID        *int64
	Status                 string
	RuleID                 *int64
	AutoCreateDraft        bool
	HasConflict            bool
	OverLimit              bool
	MediaCount             int
	Suggestions            []discovery.Suggestion
}

type UnassignedDiagnostic struct {
	ParentPath string
	MediaCount int
}

type LibraryCoverageSummary struct {
	RegularFileCount, SupportedMediaCount, SupportedArchiveCount                       int
	UnsupportedArchiveCount, ControlFileCount, IgnoredOtherCount, ActionableIssueCount int
	RegisteredSourceCount, IndexedItemCount, SourceNeedsScanCount                      int
}

type LibraryCoverageDiagnostic struct {
	Path, EntryKind, ReasonCode string
	FileCount                   int
	ByteSize                    int64
}

type DiscoverySnapshot struct {
	ID                  int64
	LibraryID           int64
	CompletedAt         time.Time
	Candidates          []Candidate
	Unassigned          []UnassignedDiagnostic
	CoverageSummary     LibraryCoverageSummary
	CoverageDiagnostics []LibraryCoverageDiagnostic
}

type filesystemCoverage struct {
	summary     LibraryCoverageSummary
	diagnostics []LibraryCoverageDiagnostic
}

type DiscoveryOptions struct {
	// AutoCreateArchives is only supplied by an explicitly saved ASSISTED or
	// TRUSTED automation run. Manual discovery always leaves archive candidates
	// for the owner to import explicitly.
	AutoCreateArchives bool
	// AutoCreateTrustedManifests is supplied only by a TRUSTED automation
	// run. Identity inspection can still downgrade any conflicting Manifest to
	// review; manual and ASSISTED discovery never set this flag.
	AutoCreateTrustedManifests bool
}

type CandidateDiscoveryStore struct {
	db *sql.DB
	// beforeSnapshotCommit is a deterministic test seam between filesystem
	// traversal and the transaction; production stores leave it nil.
	beforeSnapshotCommit func()
}

var ErrDiscoveryStateChanged = errors.New("discovery state changed during filesystem traversal; retry discovery")

type discoveryQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (db *Database) CandidateDiscovery() *CandidateDiscoveryStore {
	return &CandidateDiscoveryStore{db: db.DB}
}

// DiscoverFilesystem performs the media-read-only first phase of import. It
// never creates GalleryItems or accepts metadata suggestions, but may mark a
// changed registered archive NEEDS_RESCAN. Configured child library roots are
// hard traversal boundaries.
func (s *CandidateDiscoveryStore) DiscoverFilesystem(ctx context.Context, libraryID int64, now time.Time) (DiscoverySnapshot, error) {
	return s.DiscoverFilesystemWithOptions(ctx, libraryID, DiscoveryOptions{}, now)
}

func (s *CandidateDiscoveryStore) DiscoverFilesystemWithOptions(ctx context.Context, libraryID int64, options DiscoveryOptions, now time.Time) (DiscoverySnapshot, error) {
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
	coverage := &filesystemCoverage{}
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
		coverage.summary.RegularFileCount++
		base := strings.ToLower(entry.Name())
		directory := filepath.Dir(filename)
		if base == ".cosplay-root" {
			coverage.summary.ControlFileCount++
			markerDirectories[directory] = true
			return nil
		}
		if base == ".cosplay.json" {
			coverage.summary.ControlFileCount++
			manifestDirectories[directory] = true
			return nil
		}
		if archivefile.IsSupportedPath(filename) {
			coverage.summary.SupportedArchiveCount++
			observation, reason := observeArchiveCandidate(mediaLibrary.RootPath, filename, archiveLimits)
			if reason != "" {
				coverage.diagnostics = append(coverage.diagnostics, LibraryCoverageDiagnostic{Path: filename, EntryKind: "ARCHIVE", ReasonCode: reason, FileCount: 1, ByteSize: info.Size()})
			}
			if observation.MediaCount > 0 || observation.HasConflict {
				archives = append(archives, observation)
			}
			return nil
		}
		if sourcescan.IsSupportedMediaPath(filename) {
			coverage.summary.SupportedMediaCount++
			directoryCounts[directory]++
		} else if archivefile.IsArchiveLikePath(filename) {
			coverage.summary.UnsupportedArchiveCount++
			coverage.diagnostics = append(coverage.diagnostics, LibraryCoverageDiagnostic{Path: filename, EntryKind: "ARCHIVE", ReasonCode: "UNSUPPORTED_ARCHIVE_FORMAT", FileCount: 1, ByteSize: info.Size()})
		} else {
			coverage.summary.IgnoredOtherCount++
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
		if observation.HasRootMarker {
			observation.MarkerTitle, err = markerGalleryTitle(directory)
			if err != nil {
				return DiscoverySnapshot{}, err
			}
		}
		if manifestDirectories[directory] {
			evidence := readManifestIdentity(filepath.Join(directory, ".cosplay.json"))
			observation.HasValidManifest, observation.ManifestSetID = evidence.Valid, evidence.SetID
			observation.ManifestSchema, observation.ManifestRevision, observation.ManifestHash, observation.ManifestDocument = evidence.Schema, evidence.Revision, evidence.Hash, evidence.Document
			observation.HasConflict = !observation.HasValidManifest
		}
		observations = append(observations, observation)
	}
	observations = append(observations, archives...)
	sort.Slice(observations, func(i, j int) bool { return observations[i].RelativePath < observations[j].RelativePath })
	snapshot, err := s.commitSnapshot(ctx, libraryID, observations, coverage, options, mediaLibrary.RootPath, childRoots, now)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	// Discovery still does not read registered archive members. A cheap stat
	// comparison makes same-path replacements eligible for background source
	// reconciliation instead of silently treating them as unchanged roots.
	if err := markChangedArchiveSources(ctx, s.db, libraryID, archiveLimits, now); err != nil {
		return DiscoverySnapshot{}, err
	}
	return snapshot, nil
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

// markerGalleryTitle returns the deterministic title for a newly discovered
// DIRECTORY marker root. The marker's parent remains the source root; only the
// title changes when that root has exactly one immediate real subdirectory.
func markerGalleryTitle(rootPath string) (string, error) {
	cleaned := filepath.Clean(rootPath)
	rootInfo, err := os.Lstat(cleaned)
	if err != nil {
		return "", err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("MARKER candidate root must be a real directory")
	}
	entries, err := os.ReadDir(cleaned)
	if err != nil {
		return "", err
	}
	title := filepath.Base(cleaned)
	var onlyChild string
	childCount := 0
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		childCount++
		onlyChild = entry.Name()
		if childCount > 1 {
			break
		}
	}
	if childCount == 1 {
		title = onlyChild
	}
	return validateMarkerTitle(title)
}

func validateMarkerTitle(value string) (string, error) {
	title := norm.NFC.String(strings.TrimSpace(value))
	if title == "" || title == "." || title == string(filepath.Separator) {
		return "", errors.New("MARKER candidate has no usable directory name")
	}
	if len([]rune(title)) > 300 {
		return "", errors.New("MARKER candidate directory name exceeds the Gallery title limit")
	}
	return title, nil
}

// archiveEntitySuggestions uses only the archive's external library path and
// filename. Exact tokens produce pending suggestions, never relations. Identity
// ambiguity is resolved separately against UUIDs before automatic acceptance.
func archiveEntitySuggestions(ctx context.Context, db discoveryQueryer, libraryRoot, archivePath string) ([]discovery.Suggestion, error) {
	relative, err := filepath.Rel(libraryRoot, archivePath)
	if err != nil {
		return nil, err
	}
	parts := strings.FieldsFunc(filepath.ToSlash(relative), func(r rune) bool { return r == '/' })
	if len(parts) == 0 {
		return nil, nil
	}
	parts[len(parts)-1] = archivefile.BaseName(parts[len(parts)-1])
	tokens := make(map[string]struct{}, len(parts)*3)
	coserTokens := make(map[string]struct{}, len(parts)*4)
	for _, part := range parts {
		partTokens := archiveEntityNameTokens(part)
		for _, token := range partTokens {
			if key := normalizedKey(token); key != "" {
				tokens[key] = struct{}{}
				coserTokens[key] = struct{}{}
			}
		}
		for _, token := range archiveMultiCoserTokens(partTokens) {
			if key := normalizedKey(token); key != "" {
				coserTokens[key] = struct{}{}
			}
		}
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	type entity struct{ kind, name string }
	rows, err := db.QueryContext(ctx, `
		SELECT 'COSER', name FROM cosers
		UNION ALL SELECT 'COSER', alias FROM coser_aliases
		UNION ALL SELECT 'WORK', name FROM works
		UNION ALL SELECT 'WORK', alias FROM work_aliases
		UNION ALL SELECT 'CHARACTER', name FROM characters
		UNION ALL SELECT 'CHARACTER', alias FROM character_aliases`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	matched := map[string]map[string]struct{}{}
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			return nil, err
		}
		key := normalizedKey(name)
		available := tokens
		if kind == "COSER" {
			available = coserTokens
		}
		if _, ok := available[key]; !ok || key == "" {
			continue
		}
		if matched[kind] == nil {
			matched[kind] = map[string]struct{}{}
		}
		matched[kind][normalizedDisplay(name)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var result []discovery.Suggestion
	for _, kind := range []string{"COSER", "WORK", "CHARACTER"} {
		values := matched[kind]
		ordered := make([]string, 0, len(values))
		for value := range values {
			ordered = append(ordered, value)
		}
		pathKey := normalizedKey(entityInferenceText(filepath.ToSlash(relative)))
		sort.Slice(ordered, func(i, j int) bool {
			left, right := normalizedKey(ordered[i]), normalizedKey(ordered[j])
			a, b := strings.Index(pathKey, left), strings.Index(pathKey, right)
			if a != b {
				return a < b
			}
			return ordered[i] < ordered[j]
		})
		for _, value := range ordered {
			result = append(result, discovery.Suggestion{Field: strings.ToLower(kind), Value: value})
		}
	}
	return result, nil
}

// archiveEntityNameTokens expands only explicit presentation separators and
// bracket boundaries. It does not split ordinary whitespace or use substring
// matching, so automatic acceptance remains exact and conservative while
// common names such as "Coser - Character [100P]" become independently
// matchable.
func archiveEntityNameTokens(value string) []string {
	value = entityInferenceText(value)
	segments := []string{value}
	var outside strings.Builder
	var bracket strings.Builder
	depth := 0
	flushBracket := func() {
		if text := strings.TrimSpace(bracket.String()); text != "" {
			segments = append(segments, text)
		}
		bracket.Reset()
	}
	for _, r := range value {
		switch r {
		case '[', '【':
			if depth == 0 {
				flushBracket()
			}
			depth++
		case ']', '】':
			if depth > 0 {
				depth--
				if depth == 0 {
					flushBracket()
				}
			} else {
				outside.WriteRune(r)
			}
		default:
			if depth > 0 {
				bracket.WriteRune(r)
			} else {
				outside.WriteRune(r)
			}
		}
	}
	flushBracket()
	if text := strings.TrimSpace(outside.String()); text != "" {
		segments = append(segments, text)
	}

	result := make([]string, 0, len(segments)*2)
	seen := map[string]struct{}{}
	for _, segment := range segments {
		expanded := strings.NewReplacer(" - ", "\x00", " – ", "\x00", " — ", "\x00", " | ", "\x00", " ｜ ", "\x00").Replace(segment)
		for _, token := range strings.Split(expanded, "\x00") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			key := normalizedKey(token)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, token)
		}
	}
	return result
}

// archiveMultiCoserTokens augments, rather than replaces, ordinary identity
// tokens. The expansion is consulted only for Coser suggestions, preventing
// '&', 'x', '×' and '+' in titles from changing Work/Character inference.
func archiveMultiCoserTokens(tokens []string) []string {
	var result []string
	for _, token := range tokens {
		result = append(result, splitMultiCoserToken(token)...)
	}
	return result
}

func observeArchiveCandidate(libraryRoot, filename string, limits archivecheck.Limits) (ObservedDirectory, string) {
	validation, err := archivecheck.ValidateFile(filename, limits)
	if err != nil {
		return ObservedDirectory{}, "UNREADABLE_ARCHIVE"
	}
	count := 0
	err = archivefile.Walk(filename, func(entry archivefile.Entry) error {
		if !entry.IsDir() && sourcescan.IsSupportedMediaPath(entry.Name) {
			count++
		}
		return nil
	})
	if err != nil && !archivefile.IsEncryptedError(err) {
		return ObservedDirectory{}, "UNREADABLE_ARCHIVE"
	}
	relative, err := filepath.Rel(libraryRoot, filename)
	if err != nil {
		return ObservedDirectory{}, "UNREADABLE_ARCHIVE"
	}
	conflict := false
	for _, issue := range validation.Issues {
		if issue.Code != "UNSUPPORTED_ARCHIVE_MEDIA" {
			conflict = true
			break
		}
	}
	observation := ObservedDirectory{RelativePath: filepath.ToSlash(relative), SourceType: gallery.SourceTypeArchive, MediaCount: count, HasConflict: conflict, OverLimit: count > 1000}
	if evidence := readManifestIdentity(filename + ".cosplay.json"); evidence.Exists {
		observation.HasValidManifest, observation.ManifestSetID = evidence.Valid, evidence.SetID
		observation.ManifestSchema, observation.ManifestRevision, observation.ManifestHash, observation.ManifestDocument = evidence.Schema, evidence.Revision, evidence.Hash, evidence.Document
		observation.HasConflict = observation.HasConflict || !evidence.Valid
	}
	reason := ""
	for _, issue := range validation.Issues {
		if issue.Code == "ENCRYPTED_ARCHIVE" || issue.Code == "ENCRYPTED_ENTRY" {
			reason = "ENCRYPTED_ARCHIVE"
			break
		}
		if issue.Code != "UNSUPPORTED_ARCHIVE_MEDIA" {
			reason = "UNSAFE_ARCHIVE"
		}
	}
	if reason == "" && count == 0 {
		reason = "ARCHIVE_WITHOUT_SUPPORTED_MEDIA"
	}
	return observation, reason
}

type manifestIdentityEvidence struct {
	Exists, Valid bool
	SetID         string
	Schema        int
	Revision      int64
	Hash          string
	Document      *manifest.GalleryDocument
}

func readManifestIdentity(filename string) manifestIdentityEvidence {
	data, hash, err := manifest.ReadFile(filename, manifest.MaxGalleryBytes)
	if errors.Is(err, os.ErrNotExist) {
		return manifestIdentityEvidence{}
	}
	if err != nil {
		return manifestIdentityEvidence{Exists: true}
	}
	document, err := manifest.ParseGallery(bytes.NewReader(data))
	if err != nil {
		return manifestIdentityEvidence{Exists: true}
	}
	return manifestIdentityEvidence{Exists: true, Valid: true, SetID: document.SetID, Schema: document.SchemaVersion, Revision: document.Revision, Hash: hash, Document: &document}
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
	root                   string
	sourceType             gallery.SourceType
	method                 string
	ruleID                 *int64
	autoCreate             bool
	conflict               bool
	overLimit              bool
	mediaCount             int
	manifestSetID          string
	manifestSchema         int
	manifestRevision       int64
	manifestHash           string
	identityClassification string
	identityIssueCode      string
	rebindGalleryID        *int64
	status                 string
	suggestions            []discovery.Suggestion
}

func (s *CandidateDiscoveryStore) CommitSnapshot(
	ctx context.Context,
	libraryID int64,
	observations []ObservedDirectory,
	now time.Time,
) (DiscoverySnapshot, error) {
	return s.commitSnapshot(ctx, libraryID, observations, nil, DiscoveryOptions{}, "", nil, now)
}

func (s *CandidateDiscoveryStore) commitSnapshot(
	ctx context.Context, libraryID int64, observations []ObservedDirectory, coverage *filesystemCoverage, options DiscoveryOptions,
	expectedRoot string, expectedChildRoots map[string]struct{}, now time.Time,
) (DiscoverySnapshot, error) {
	if s.beforeSnapshotCommit != nil {
		s.beforeSnapshotCommit()
	}
	// The writable connection uses BEGIN IMMEDIATE. All discovery decisions and
	// the resulting snapshot therefore observe one serialized database state.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()
	mediaLibrary, err := findLibrary(ctx, tx, libraryID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	if !mediaLibrary.Enabled || expectedRoot != "" && filepath.Clean(mediaLibrary.RootPath) != filepath.Clean(expectedRoot) {
		return DiscoverySnapshot{}, ErrDiscoveryStateChanged
	}
	if expectedChildRoots != nil {
		actual, err := childBoundaryRoots(ctx, tx, libraryID, mediaLibrary.RootPath)
		if err != nil {
			return DiscoverySnapshot{}, err
		}
		if !samePathSet(actual, expectedChildRoots) {
			return DiscoverySnapshot{}, ErrDiscoveryStateChanged
		}
	}
	rules, err := listRecognitionRules(ctx, tx, libraryID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	boundaries, err := boundSourcePaths(ctx, tx, libraryID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	ignored, err := ignoredPaths(ctx, tx, libraryID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	ignoredSetIDs, err := ignoredSetIDs(ctx, tx)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	setIdentities, err := gallerySetIdentities(ctx, tx)
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
		if observed.HasValidManifest && observed.ManifestDocument == nil {
			manifestPath, pathErr := manifest.GalleryPath(observed.SourceType, absolute)
			if pathErr != nil {
				return DiscoverySnapshot{}, pathErr
			}
			evidence := readManifestIdentity(manifestPath)
			if evidence.Valid && evidence.SetID == observed.ManifestSetID {
				observed.ManifestSchema, observed.ManifestRevision, observed.ManifestHash, observed.ManifestDocument = evidence.Schema, evidence.Revision, evidence.Hash, evidence.Document
			}
		}

		var match *discovery.Match
		if observed.HasValidManifest {
			match = &discovery.Match{Root: relative, Kind: "MANIFEST", AutoCreate: (observed.SourceType == gallery.SourceTypeArchive && options.AutoCreateArchives) || (observed.SourceType == gallery.SourceTypeDirectory && options.AutoCreateTrustedManifests)}
		} else if observed.SourceType == gallery.SourceTypeArchive {
			match = &discovery.Match{Root: relative, Kind: discovery.RuleKindArchiveFile, AutoCreate: options.AutoCreateArchives}
			// PATH_TEMPLATE remains an optional metadata suggestion provider for
			// archives; it no longer decides whether the archive is a Gallery root.
			for _, rule := range rules {
				if rule.Kind != discovery.RuleKindPathTemplate {
					continue
				}
				templateMatch, matchErr := discovery.MatchDirectory(relative, false, []discovery.Rule{rule})
				if matchErr != nil {
					return DiscoverySnapshot{}, matchErr
				}
				if templateMatch != nil {
					match.RuleID = templateMatch.RuleID
					match.Suggestions = append(match.Suggestions, templateMatch.Suggestions...)
					break
				}
			}
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
		if match.Kind == discovery.RuleKindMarker && observed.MarkerTitle != "" {
			markerTitle, err := validateMarkerTitle(observed.MarkerTitle)
			if err != nil {
				return DiscoverySnapshot{}, err
			}
			match.Suggestions = []discovery.Suggestion{{Field: "title", Value: markerTitle}}
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
				manifestSetID: observed.ManifestSetID, manifestSchema: observed.ManifestSchema,
				manifestRevision: observed.ManifestRevision, manifestHash: observed.ManifestHash, status: "PENDING",
			}
			if observed.ManifestDocument != nil {
				inspection, inspectionErr := inspectManifestIdentity(ctx, tx, *observed.ManifestDocument)
				if inspectionErr != nil {
					return DiscoverySnapshot{}, inspectionErr
				}
				candidate.identityClassification = string(inspection.Class)
				if inspection.Class != "UNCLAIMED" {
					candidate.identityIssueCode = "MANIFEST_IDENTITY_" + string(inspection.Class)
					candidate.conflict = true
					candidate.autoCreate = false
				}
			}
			if identity, found := setIdentities[observed.ManifestSetID]; observed.ManifestSetID != "" && found && identity.SourcePath != rootPath {
				value := identity.GalleryID
				candidate.rebindGalleryID = &value
				candidate.status = "SOURCE_REBIND_CANDIDATE"
				candidate.conflict = candidate.conflict || identity.SourceAvailable
				candidate.autoCreate = false
				if identity.SourceAvailable {
					candidate.identityClassification = "DUPLICATE_ACCESSIBLE_SOURCE"
					candidate.identityIssueCode = "MANIFEST_IDENTITY_DUPLICATE_ACCESSIBLE_SOURCE"
				} else {
					candidate.identityClassification = "SAME_GALLERY_SOURCE_MOVE"
					candidate.identityIssueCode = "MANIFEST_IDENTITY_SAME_GALLERY_SOURCE_MOVE"
				}
			}
			if match.RuleID != 0 {
				value := match.RuleID
				candidate.ruleID = &value
			}
			if candidate.sourceType == gallery.SourceTypeArchive && !observed.HasValidManifest {
				entitySuggestions, suggestionErr := archiveEntitySuggestions(ctx, tx, mediaLibrary.RootPath, candidate.root)
				if suggestionErr != nil {
					return DiscoverySnapshot{}, suggestionErr
				}
				candidate.suggestions = append(candidate.suggestions, entitySuggestions...)
			}
			candidates[rootPath] = candidate
		} else if candidate.sourceType != observed.SourceType {
			candidate.conflict = true
		}
		candidate.mediaCount += observed.MediaCount
		candidate.conflict = candidate.conflict || observed.HasConflict
		candidate.overLimit = candidate.overLimit || observed.OverLimit || candidate.mediaCount > 1000
	}
	// A confirmed DIRECTORY candidate owns its complete subtree. Archives under
	// that root are files within that Gallery, not nested Gallery sources.
	for archiveRoot, archiveCandidate := range candidates {
		if archiveCandidate.sourceType != gallery.SourceTypeArchive {
			continue
		}
		for directoryRoot, directoryCandidate := range candidates {
			if directoryCandidate.sourceType == gallery.SourceTypeDirectory && directoryRoot != archiveRoot && pathWithin(directoryRoot, archiveRoot) {
				delete(candidates, archiveRoot)
				break
			}
		}
	}
	for unassignedPath := range unassigned {
		for candidateRoot := range candidates {
			if pathWithin(candidateRoot, unassignedPath) {
				delete(unassigned, unassignedPath)
				break
			}
		}
	}

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
		inspectionToken := ""
		if candidate.manifestHash != "" {
			inspectionToken = discoveryInspectionToken(snapshotID, libraryID, root, candidate.manifestHash, candidate.identityClassification)
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_candidates (
				snapshot_id, library_id, root_path, source_type, recognition_method, rule_id,
				manifest_set_id, rebind_gallery_id, auto_create_draft, has_conflict,
				over_limit, media_count, status, created_at_utc,manifest_schema,manifest_revision,
				manifest_hash,identity_classification,identity_issue_code,inspection_token_hash
			) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?,?,?,?,?,?,?)
		`, snapshotID, libraryID, candidate.root, candidate.sourceType, candidate.method, candidate.ruleID,
			candidate.manifestSetID, candidate.rebindGalleryID, candidate.autoCreate, candidate.conflict,
			candidate.overLimit, candidate.mediaCount, candidate.status, timestamp, candidate.manifestSchema,
			candidate.manifestRevision, candidate.manifestHash, candidate.identityClassification, candidate.identityIssueCode, inspectionToken)
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
	if coverage != nil {
		for _, parent := range diagnosticPaths {
			coverage.diagnostics = append(coverage.diagnostics, LibraryCoverageDiagnostic{Path: parent, EntryKind: "DIRECTORY", ReasonCode: "UNASSIGNED_MEDIA_DIRECTORY", FileCount: unassigned[parent]})
		}
		coverage.summary.ActionableIssueCount = len(coverage.diagnostics)
		if _, err := tx.ExecContext(ctx, `INSERT INTO library_coverage_summaries (
			snapshot_id,library_id,regular_file_count,supported_media_count,supported_archive_count,
			unsupported_archive_count,control_file_count,ignored_other_count,actionable_issue_count
		) VALUES (?,?,?,?,?,?,?,?,?)`, snapshotID, libraryID, coverage.summary.RegularFileCount,
			coverage.summary.SupportedMediaCount, coverage.summary.SupportedArchiveCount,
			coverage.summary.UnsupportedArchiveCount, coverage.summary.ControlFileCount, coverage.summary.IgnoredOtherCount,
			coverage.summary.ActionableIssueCount); err != nil {
			return DiscoverySnapshot{}, err
		}
		sort.Slice(coverage.diagnostics, func(i, j int) bool { return coverage.diagnostics[i].Path < coverage.diagnostics[j].Path })
		for _, diagnostic := range coverage.diagnostics {
			if _, err := tx.ExecContext(ctx, `INSERT INTO library_coverage_diagnostics (
				snapshot_id,library_id,path,entry_kind,reason_code,file_count,byte_size
			) VALUES (?,?,?,?,?,?,?)`, snapshotID, libraryID, diagnostic.Path, diagnostic.EntryKind,
				diagnostic.ReasonCode, diagnostic.FileCount, diagnostic.ByteSize); err != nil {
				return DiscoverySnapshot{}, err
			}
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

func childBoundaryRoots(ctx context.Context, queryer discoveryQueryer, parentID int64, parentRoot string) (map[string]struct{}, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT id, root_path FROM media_libraries WHERE id != ?`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roots := make(map[string]struct{})
	for rows.Next() {
		var id int64
		var root string
		if err := rows.Scan(&id, &root); err != nil {
			return nil, err
		}
		if pathWithin(parentRoot, root) {
			roots[filepath.Clean(root)] = struct{}{}
		}
	}
	return roots, rows.Err()
}

func samePathSet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for path := range left {
		if _, exists := right[path]; !exists {
			return false
		}
	}
	return true
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
			has_conflict, over_limit, media_count,manifest_schema,manifest_revision,manifest_hash,
			identity_classification,identity_issue_code,inspection_token_hash
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
			&overLimit, &candidate.MediaCount, &candidate.ManifestSchema, &candidate.ManifestRevision,
			&candidate.ManifestHash, &candidate.IdentityClassification, &candidate.IdentityIssueCode, &candidate.InspectionTokenHash,
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
	if err := diagnosticRows.Err(); err != nil {
		return DiscoverySnapshot{}, err
	}
	_ = diagnosticRows.Close()
	if err := s.db.QueryRowContext(ctx, `SELECT regular_file_count,supported_media_count,supported_archive_count,
		unsupported_archive_count,control_file_count,ignored_other_count,actionable_issue_count FROM library_coverage_summaries WHERE snapshot_id=?`, snapshotID).
		Scan(&result.CoverageSummary.RegularFileCount, &result.CoverageSummary.SupportedMediaCount,
			&result.CoverageSummary.SupportedArchiveCount, &result.CoverageSummary.UnsupportedArchiveCount, &result.CoverageSummary.ControlFileCount,
			&result.CoverageSummary.IgnoredOtherCount, &result.CoverageSummary.ActionableIssueCount); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return DiscoverySnapshot{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN availability_state!='AVAILABLE' OR reconcile_state!='IN_SYNC' THEN 1 ELSE 0 END),0)
		FROM gallery_sources WHERE library_id=?`, result.LibraryID).Scan(&result.CoverageSummary.RegisteredSourceCount, &result.CoverageSummary.SourceNeedsScanCount); err != nil {
		return DiscoverySnapshot{}, err
	}
	result.CoverageSummary.ActionableIssueCount += result.CoverageSummary.SourceNeedsScanCount
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_items i JOIN gallery_sources s ON s.id=i.source_id WHERE s.library_id=?`, result.LibraryID).
		Scan(&result.CoverageSummary.IndexedItemCount); err != nil {
		return DiscoverySnapshot{}, err
	}
	coverageRows, err := s.db.QueryContext(ctx, `SELECT path,entry_kind,reason_code,file_count,byte_size
		FROM library_coverage_diagnostics WHERE snapshot_id=? ORDER BY reason_code,path`, snapshotID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	defer coverageRows.Close()
	for coverageRows.Next() {
		var value LibraryCoverageDiagnostic
		if err := coverageRows.Scan(&value.Path, &value.EntryKind, &value.ReasonCode, &value.FileCount, &value.ByteSize); err != nil {
			return DiscoverySnapshot{}, err
		}
		result.CoverageDiagnostics = append(result.CoverageDiagnostics, value)
	}
	return result, coverageRows.Err()
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

	var libraryID, candidateSnapshotID int64
	var rootPath string
	var sourceType gallery.SourceType
	var recognitionMethod string
	var status string
	var existingGalleryID sql.NullInt64
	var manifestSetID sql.NullString
	var storedManifestSchema int
	var storedManifestRevision int64
	var storedManifestHash, storedIdentityClassification, storedInspectionToken string
	var conflict, overLimit int
	if err := tx.QueryRowContext(ctx, `
		SELECT library_id, snapshot_id, root_path, source_type, recognition_method, status, has_conflict, over_limit, manifest_set_id,
			gallery_id,
			manifest_schema,manifest_revision,manifest_hash,identity_classification,inspection_token_hash
		FROM gallery_candidates WHERE id = ?
	`, candidateID).Scan(&libraryID, &candidateSnapshotID, &rootPath, &sourceType, &recognitionMethod, &status, &conflict, &overLimit, &manifestSetID,
		&existingGalleryID,
		&storedManifestSchema, &storedManifestRevision, &storedManifestHash, &storedIdentityClassification, &storedInspectionToken); err != nil {
		return gallery.Gallery{}, err
	}
	if status != "PENDING" && !(status == "IMPORTED" && existingGalleryID.Valid) {
		return gallery.Gallery{}, fmt.Errorf("candidate %d is already %s", candidateID, status)
	}
	if status == "PENDING" {
		var latestSnapshotID int64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(id) FROM discovery_snapshots WHERE library_id=?`, libraryID).Scan(&latestSnapshotID); err != nil || latestSnapshotID != candidateSnapshotID {
			return gallery.Gallery{}, errors.New("candidate is not from the latest discovery snapshot")
		}
	}
	if conflict == 1 || overLimit == 1 {
		return gallery.Gallery{}, errors.New("conflicting or over-limit candidate cannot create a DRAFT")
	}
	var manifestDocument *manifest.GalleryDocument
	manifestItemUUIDs := map[string]string{}
	manifestPath := ""
	if manifestSetID.Valid {
		manifestPath, err = manifest.GalleryPath(sourceType, rootPath)
		if err != nil {
			return gallery.Gallery{}, err
		}
		data, hash, readErr := manifest.ReadFile(manifestPath, manifest.MaxGalleryBytes)
		if readErr != nil {
			return gallery.Gallery{}, errors.New("Manifest changed after discovery; run discovery again")
		}
		document, parseErr := manifest.ParseGallery(bytes.NewReader(data))
		if parseErr != nil || document.SetID != manifestSetID.String || document.SchemaVersion != storedManifestSchema || document.Revision != storedManifestRevision || hash != storedManifestHash {
			return gallery.Gallery{}, errors.New("Manifest changed after discovery; run discovery again")
		}
		inspection, inspectionErr := inspectManifestIdentity(ctx, tx, document)
		if inspectionErr != nil {
			return gallery.Gallery{}, inspectionErr
		}
		expectedToken := discoveryInspectionToken(candidateSnapshotID, libraryID, rootPath, storedManifestHash, storedIdentityClassification)
		if string(inspection.Class) != storedIdentityClassification || storedInspectionToken == "" || storedInspectionToken != expectedToken {
			return gallery.Gallery{}, errors.New("Manifest identity state changed after discovery; run discovery again")
		}
		if inspection.Class != "UNCLAIMED" {
			return gallery.Gallery{}, errors.New("Manifest identity requires explicit review")
		}
		for _, item := range document.Items.Value {
			if item.ItemUUID != "" {
				manifestItemUUIDs[item.Path] = item.ItemUUID
			}
		}
		manifestDocument = &document
	}
	if status == "IMPORTED" {
		if manifestDocument == nil {
			return gallery.Gallery{}, errors.New("candidate import already created a DRAFT; continue from Gallery management")
		}
		var sourceID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM gallery_sources WHERE gallery_id=? AND library_id=? AND source_type=? AND source_path=?`,
			existingGalleryID.Int64, libraryID, sourceType, rootPath).Scan(&sourceID); err != nil {
			return gallery.Gallery{}, err
		}
		if err := tx.Rollback(); err != nil {
			return gallery.Gallery{}, err
		}
		return s.finishManifestCandidateImport(ctx, existingGalleryID.Int64, sourceID, manifestPath, storedManifestHash, manifestItemUUIDs, now)
	}

	setID := manifestSetID.String
	if setID == "" {
		setID = portableid.New()
	}
	title := ""
	if recognitionMethod == string(discovery.RuleKindMarker) {
		if err := tx.QueryRowContext(ctx, `
			SELECT value FROM gallery_candidate_suggestions
			WHERE candidate_id = ? AND field_name = 'title' ORDER BY id LIMIT 1
		`, candidateID).Scan(&title); errors.Is(err, sql.ErrNoRows) {
			title = filepath.Base(filepath.Clean(rootPath))
		} else if err != nil {
			return gallery.Gallery{}, err
		}
		title, err = validateMarkerTitle(title)
		if err != nil {
			return gallery.Gallery{}, err
		}
	}
	if title == "" && sourceType == gallery.SourceTypeArchive {
		// Archives without a valid sidecar still need a stable, human-readable
		// Gallery title. The filename is more specific than its parent folder.
		title = archivefile.BaseName(rootPath)
		title, err = validateMarkerTitle(title)
		if err != nil {
			return gallery.Gallery{}, err
		}
	}
	timestamp := normalisedTime(now)
	if _, err := registerPortableUUID(ctx, tx, setID, portableid.KindGallery, timestamp); err != nil {
		return gallery.Gallery{}, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO galleries (
			set_id, slug, state, title, created_at_utc, updated_at_utc
		) VALUES (?, ?, 'DRAFT', ?, ?, ?)
	`, setID, slug.FromName(title, setID), title, formatTime(timestamp), formatTime(timestamp))
	if err != nil {
		return gallery.Gallery{}, err
	}
	galleryID, err := result.LastInsertId()
	if err != nil {
		return gallery.Gallery{}, err
	}
	sourceResult, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_sources (
			gallery_id, library_id, source_type, source_path, availability_state,
			reconcile_state, created_at_utc, updated_at_utc
		) VALUES (?, ?, ?, ?, 'AVAILABLE', 'NEVER_SCANNED', ?, ?)
	`, galleryID, libraryID, sourceType, rootPath, formatTime(timestamp), formatTime(timestamp))
	if err != nil {
		return gallery.Gallery{}, err
	}
	sourceID, err := sourceResult.LastInsertId()
	if err != nil {
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
		case "title":
			// A MARKER import already used the exact source-directory name as
			// its deterministic title fallback. Do not turn that same value
			// into a redundant pending metadata suggestion.
			if recognitionMethod == string(discovery.RuleKindMarker) {
				continue
			}
			fallthrough
		case "year", "month":
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
	if manifestDocument != nil {
		return s.finishManifestCandidateImport(ctx, galleryID, sourceID, manifestPath, storedManifestHash, manifestItemUUIDs, now)
	}
	return created, nil
}

func discoveryInspectionToken(snapshotID, libraryID int64, rootPath, manifestHash, classification string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d\x00%s\x00%s\x00%s", snapshotID, libraryID, rootPath, manifestHash, classification)))
	return hex.EncodeToString(digest[:])
}

func (s *CandidateDiscoveryStore) finishManifestCandidateImport(
	ctx context.Context,
	galleryID, sourceID int64,
	manifestPath, manifestHash string,
	manifestItemUUIDs map[string]string,
	now time.Time,
) (gallery.Gallery, error) {
	if _, hash, readErr := manifest.ReadFile(manifestPath, manifest.MaxGalleryBytes); readErr != nil || hash != manifestHash {
		return gallery.Gallery{}, errors.New("Manifest changed before source scan; DRAFT retained for review")
	}
	runtime, err := (&SettingsStore{db: s.db}).Find(ctx)
	if err != nil {
		return gallery.Gallery{}, err
	}
	limits := archivecheck.Limits{MaxEntries: runtime.ArchiveMaxEntries, MaxEntryUncompressed: uint64(runtime.ArchiveMaxEntryBytes), MaxTotalUncompressed: uint64(runtime.ArchiveMaxTotalBytes), MaxCompressionRatio: runtime.ArchiveMaxCompressionRatio, MaxImagePixels: uint64(runtime.ArchiveMaxImagePixels)}
	if err := (&ScanStore{db: s.db}).RunWithOptions(ctx, sourceID, limits, ScanOptions{ExcludeNewRootMedia: false, PortableItemUUIDByPath: manifestItemUUIDs, RegisterManifestItemUUIDs: true}, now); err != nil {
		return gallery.Gallery{}, err
	}
	current, err := findGallery(ctx, s.db, galleryID)
	if err != nil {
		return gallery.Gallery{}, err
	}
	if _, err := (&ManifestStore{db: s.db}).PullGallery(ctx, galleryID, current.MetadataRevision, now); err != nil {
		return gallery.Gallery{}, err
	}
	return findGallery(ctx, s.db, galleryID)
}

// ForkCandidateManifest turns an explicitly confirmed duplicate source into a
// new Gallery identity lineage. Only Gallery-local identities are replaced;
// shared Coser/Work/Character/Tag UUIDs and all media bytes remain untouched.
func (s *CandidateDiscoveryStore) ForkCandidateManifest(ctx context.Context, candidateID int64, now time.Time) (DiscoverySnapshot, error) {
	var libraryID, snapshotID int64
	var rootPath string
	var sourceType gallery.SourceType
	var status, classification, expectedHash, token string
	var writebackEnabled int
	if err := s.db.QueryRowContext(ctx, `SELECT candidate.library_id,candidate.snapshot_id,candidate.root_path,candidate.source_type,
		candidate.status,candidate.identity_classification,candidate.manifest_hash,candidate.inspection_token_hash,library.metadata_writeback_enabled
		FROM gallery_candidates candidate JOIN media_libraries library ON library.id=candidate.library_id WHERE candidate.id=?`, candidateID).
		Scan(&libraryID, &snapshotID, &rootPath, &sourceType, &status, &classification, &expectedHash, &token, &writebackEnabled); err != nil {
		return DiscoverySnapshot{}, err
	}
	if status != "PENDING" && status != "SOURCE_REBIND_CANDIDATE" {
		return DiscoverySnapshot{}, errors.New("Gallery candidate is no longer forkable")
	}
	if classification != "DUPLICATE_ACCESSIBLE_SOURCE" && classification != "LOCAL_ITEM_OR_LINK_CONFLICT" {
		return DiscoverySnapshot{}, errors.New("Gallery candidate identity class cannot be forked")
	}
	if writebackEnabled != 1 {
		return DiscoverySnapshot{}, errors.New("media library metadata writeback is disabled")
	}
	var latestSnapshotID int64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(id) FROM discovery_snapshots WHERE library_id=?`, libraryID).Scan(&latestSnapshotID); err != nil || latestSnapshotID != snapshotID {
		return DiscoverySnapshot{}, errors.New("candidate is not from the latest discovery snapshot")
	}
	manifestPath, err := manifest.GalleryPath(sourceType, rootPath)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	data, hash, err := manifest.ReadFile(manifestPath, manifest.MaxGalleryBytes)
	if err != nil || hash != expectedHash || token == "" {
		return DiscoverySnapshot{}, errors.New("Manifest changed after discovery; run discovery again")
	}
	document, err := manifest.ParseGallery(bytes.NewReader(data))
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	replacements := map[string]string{document.SetID: portableid.New()}
	if document.Items.Present && !document.Items.Null {
		for _, item := range document.Items.Value {
			old := item.ItemUUID
			if old == "" {
				continue
			}
			fresh := portableid.New()
			replacements[old] = fresh
		}
	}
	if document.ExternalLinks.Present && !document.ExternalLinks.Null {
		for _, link := range document.ExternalLinks.Value {
			if old := link.LinkUUID; old != "" {
				replacements[old] = portableid.New()
			}
		}
	}
	var rawDocument map[string]any
	if err := json.Unmarshal(data, &rawDocument); err != nil {
		return DiscoverySnapshot{}, err
	}
	replaceGalleryLocalJSONIdentities(rawDocument, replacements)
	rawDocument["revision"] = 0
	rawDocument["updated_at"] = normalisedTime(now).Format(time.RFC3339)
	encoded, err := json.MarshalIndent(rawDocument, "", "  ")
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	encoded = append(encoded, '\n')
	if _, currentHash, err := manifest.ReadFile(manifestPath, manifest.MaxGalleryBytes); err != nil || currentHash != expectedHash {
		return DiscoverySnapshot{}, errors.New("Manifest changed before fork; run discovery again")
	}
	if _, err := manifest.WriteAtomic(manifestPath, encoded, manifest.MaxGalleryBytes); err != nil {
		return DiscoverySnapshot{}, err
	}
	return s.DiscoverFilesystem(ctx, libraryID, now)
}

func replaceGalleryLocalJSONIdentities(value any, replacements map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "set_id" || key == "item_uuid" || key == "link_uuid" {
				if current, ok := child.(string); ok {
					if replacement, found := replacements[current]; found {
						typed[key] = replacement
						continue
					}
				}
			}
			replaceGalleryLocalJSONIdentities(child, replacements)
		}
	case []any:
		for _, child := range typed {
			replaceGalleryLocalJSONIdentities(child, replacements)
		}
	}
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

func boundSourcePaths(ctx context.Context, queryer discoveryQueryer, libraryID int64) ([]string, error) {
	return queryPaths(ctx, queryer, `SELECT source_path FROM gallery_sources WHERE library_id = ?`, libraryID)
}

func ignoredPaths(ctx context.Context, queryer discoveryQueryer, libraryID int64) ([]string, error) {
	return queryPaths(ctx, queryer, `
		SELECT source_path FROM ignored_gallery_sources
		WHERE library_id = ? OR library_id IS NULL
	`, libraryID)
}

func ignoredSetIDs(ctx context.Context, queryer discoveryQueryer) (map[string]struct{}, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT set_id FROM ignored_gallery_sources WHERE set_id IS NOT NULL`)
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

func gallerySetIdentities(ctx context.Context, queryer discoveryQueryer) (map[string]gallerySetIdentity, error) {
	rows, err := queryer.QueryContext(ctx, `
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

func queryPaths(ctx context.Context, db discoveryQueryer, query string, argument int64) ([]string, error) {
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
