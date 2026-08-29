package productdb

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/discovery"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/library"
)

func TestFilesystemDiscoveryHonoursMarkerRootAndSuppressesNestedDiagnostics(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 2, 0, 0, 0, time.UTC)
	root := t.TempDir()
	setRoot := filepath.Join(root, "Coser", "Set")
	if err := os.MkdirAll(filepath.Join(setRoot, "selfie"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(setRoot, ".cosplay-root"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(setRoot, "selfie", "01.jpg"), []byte("candidate"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(setRoot, "cover.jpg"), []byte("root media"), 0o644); err != nil {
		t.Fatal(err)
	}
	mediaLibrary, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Root", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{LibraryID: mediaLibrary.ID, Name: "Marker", Kind: discovery.RuleKindMarker, Enabled: true, Order: 1}, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.CandidateDiscovery().DiscoverFilesystem(ctx, mediaLibrary.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 1 || snapshot.Candidates[0].RootPath != setRoot || snapshot.Candidates[0].MediaCount != 2 {
		t.Fatalf("filesystem candidates = %#v", snapshot.Candidates)
	}
	if len(snapshot.Candidates[0].Suggestions) != 1 ||
		snapshot.Candidates[0].Suggestions[0] != (discovery.Suggestion{Field: "title", Value: "selfie"}) {
		t.Fatalf("marker title suggestions = %#v", snapshot.Candidates[0].Suggestions)
	}
	if len(snapshot.Unassigned) != 0 {
		t.Fatalf("nested media was also reported unassigned: %#v", snapshot.Unassigned)
	}
	created, err := db.CandidateDiscovery().ImportCandidate(ctx, snapshot.Candidates[0].ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "selfie" {
		t.Fatalf("marker Gallery title = %q, want sole immediate child directory name", created.Title)
	}
	var pendingTitleSuggestions int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM gallery_metadata_suggestions
		WHERE gallery_id = ? AND suggestion_kind = 'TITLE' AND status = 'PENDING'
	`, created.ID).Scan(&pendingTitleSuggestions); err != nil {
		t.Fatal(err)
	}
	if pendingTitleSuggestions != 0 {
		t.Fatalf("marker fallback created %d redundant title suggestions", pendingTitleSuggestions)
	}
}

func TestFilesystemMarkerWithMultipleImmediateChildDirectoriesUsesRootName(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 19, 0, 10, 0, 0, time.UTC)
	libraryRoot := t.TempDir()
	setRoot := filepath.Join(libraryRoot, "Coser", "Set")
	for _, relative := range []string{"part-a/01.jpg", "part-b/01.jpg"} {
		filename := filepath.Join(setRoot, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte("candidate"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(setRoot, ".cosplay-root"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mediaLibrary, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Root", RootPath: libraryRoot, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{LibraryID: mediaLibrary.ID, Name: "Marker", Kind: discovery.RuleKindMarker, Enabled: true, Order: 1}, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.CandidateDiscovery().DiscoverFilesystem(ctx, mediaLibrary.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 1 || snapshot.Candidates[0].RootPath != setRoot || snapshot.Candidates[0].MediaCount != 2 {
		t.Fatalf("filesystem candidates = %#v", snapshot.Candidates)
	}
	if len(snapshot.Candidates[0].Suggestions) != 1 || snapshot.Candidates[0].Suggestions[0] != (discovery.Suggestion{Field: "title", Value: "Set"}) {
		t.Fatalf("marker title suggestions = %#v", snapshot.Candidates[0].Suggestions)
	}
	created, err := db.CandidateDiscovery().ImportCandidate(ctx, snapshot.Candidates[0].ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "Set" {
		t.Fatalf("marker Gallery title = %q, want marker root directory name", created.Title)
	}
}

func TestParentFilesystemDiscoverySkipsConfiguredChildLibrary(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 2, 15, 0, 0, time.UTC)
	root := t.TempDir()
	childRoot := filepath.Join(root, "child")
	if err := os.MkdirAll(filepath.Join(childRoot, "set"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childRoot, "set", "01.jpg"), []byte("candidate"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Parent", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Child", RootPath: childRoot, Enabled: true}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{LibraryID: parent.ID, Name: "Depth", Kind: discovery.RuleKindFixedDepth, Enabled: true, FixedDepth: 1, Order: 1}, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.CandidateDiscovery().DiscoverFilesystem(ctx, parent.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 0 || len(snapshot.Unassigned) != 0 {
		t.Fatalf("parent crossed child boundary: %#v", snapshot)
	}
}

func TestFilesystemArchiveVideoCanCreateDraftForExplicitExclusion(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 2, 30, 0, 0, time.UTC)
	root := t.TempDir()
	archivePath := filepath.Join(root, "video-set.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("not decoded during discovery")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	mediaLibrary, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Archives", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{LibraryID: mediaLibrary.ID, Name: "Direct child", Kind: discovery.RuleKindFixedDepth, Enabled: true, FixedDepth: 1, Order: 1}, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.CandidateDiscovery().DiscoverFilesystem(ctx, mediaLibrary.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 1 || snapshot.Candidates[0].HasConflict {
		t.Fatalf("archive candidate = %#v", snapshot.Candidates)
	}
	if _, err := db.CandidateDiscovery().ImportCandidate(ctx, snapshot.Candidates[0].ID, now); err != nil {
		t.Fatalf("video archive could not enter DRAFT: %v", err)
	}
}

func TestDiscoveryDefaultsToUnassignedDiagnostics(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)

	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{
		LibraryID: mediaLibrary.ID, Name: "Disabled direct child",
		Kind: discovery.RuleKindFixedDepth, FixedDepth: 1,
	}, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: "unassigned/set", MediaCount: 3,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 0 || len(snapshot.Unassigned) != 1 || snapshot.Unassigned[0].MediaCount != 3 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestDiscoveryPriorityAndPathSuggestions(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	rules := db.RecognitionRules()
	if _, err := rules.Create(ctx, CreateRecognitionRuleInput{
		LibraryID: mediaLibrary.ID, Name: "Template", Kind: discovery.RuleKindPathTemplate,
		Enabled: true, Order: 20, Pattern: `(?P<work>[^/]+)/(?P<character>[^/]+)/(?P<coser>[^/]+)`,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := rules.Create(ctx, CreateRecognitionRuleInput{
		LibraryID: mediaLibrary.ID, Name: "Fallback", Kind: discovery.RuleKindFixedDepth,
		Enabled: true, Order: 1, FixedDepth: 1,
	}, now); err != nil {
		t.Fatal(err)
	}

	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{
		{RelativePath: "Work/Character/Coser", MediaCount: 10},
		{RelativePath: "manifest/set", HasValidManifest: true, MediaCount: 5},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 2 {
		t.Fatalf("candidates = %#v", snapshot.Candidates)
	}
	var template, manifest *Candidate
	for index := range snapshot.Candidates {
		candidate := &snapshot.Candidates[index]
		switch candidate.Method {
		case string(discovery.RuleKindPathTemplate):
			template = candidate
		case "MANIFEST":
			manifest = candidate
		}
	}
	if template == nil || len(template.Suggestions) != 3 {
		t.Fatalf("template candidate = %#v", template)
	}
	if manifest == nil || manifest.RuleID != nil {
		t.Fatalf("manifest candidate = %#v", manifest)
	}
}

func TestRecognitionRuleUpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	rules := db.RecognitionRules()
	created, err := rules.Create(ctx, CreateRecognitionRuleInput{
		LibraryID: mediaLibrary.ID, Name: "Depth", Kind: discovery.RuleKindFixedDepth,
		Enabled: true, Order: 20, FixedDepth: 2,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := rules.Update(ctx, UpdateRecognitionRuleInput{
		ID: created.ID, Name: "Marker", Kind: discovery.RuleKindMarker,
		Enabled: true, AutoCreateDraft: true, Order: 5,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Kind != discovery.RuleKindMarker || updated.FixedDepth != 0 ||
		!updated.Enabled || !updated.AutoCreateDraft || updated.Order != 5 {
		t.Fatalf("updated rule = %#v", updated)
	}
	listed, err := rules.List(ctx, mediaLibrary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0] != updated {
		t.Fatalf("listed rules = %#v, want %#v", listed, updated)
	}

	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: "Set", HasRootMarker: true, MediaCount: 2,
	}}, now.Add(2*time.Minute))
	if err != nil || len(snapshot.Candidates) != 1 || snapshot.Candidates[0].RuleID == nil {
		t.Fatalf("candidate before rule deletion = %#v, %v", snapshot.Candidates, err)
	}
	if err := rules.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	listed, err = rules.List(ctx, mediaLibrary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("rules after delete = %#v", listed)
	}
	snapshot, err = db.CandidateDiscovery().LatestSnapshot(ctx, mediaLibrary.ID)
	if err != nil || len(snapshot.Candidates) != 1 || snapshot.Candidates[0].RuleID != nil {
		t.Fatalf("candidate after rule deletion = %#v, %v", snapshot.Candidates, err)
	}
	if _, err := rules.Update(ctx, UpdateRecognitionRuleInput{
		ID: created.ID, Name: "Missing", Kind: discovery.RuleKindMarker,
	}, now.Add(3*time.Minute)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("updating missing rule = %v, want sql.ErrNoRows", err)
	}
	if err := rules.Delete(ctx, created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleting missing rule = %v, want sql.ErrNoRows", err)
	}
}

func TestAutoCreateDraftKeepsSuggestionsPending(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{
		LibraryID: mediaLibrary.ID, Name: "Auto template", Kind: discovery.RuleKindPathTemplate,
		Enabled: true, AutoCreateDraft: true,
		Pattern: `(?P<title>[^/]+)-(?P<coser>[^/]+)`,
	}, now); err != nil {
		t.Fatal(err)
	}

	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: "My Set-Coser", MediaCount: 8,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 1 || snapshot.Candidates[0].Status != "IMPORTED" || snapshot.Candidates[0].GalleryID == nil {
		t.Fatalf("auto candidate = %#v", snapshot.Candidates)
	}
	created, err := db.Galleries().Find(ctx, *snapshot.Candidates[0].GalleryID)
	if err != nil {
		t.Fatal(err)
	}
	if created.State != gallery.StateDraft || created.Title != "" {
		t.Fatalf("auto-created Gallery wrote suggestions into metadata: %#v", created)
	}
	var identityPending, metadataPending int
	if err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM gallery_identity_suggestions WHERE gallery_id = ? AND status = 'PENDING'),
			(SELECT COUNT(*) FROM gallery_metadata_suggestions WHERE gallery_id = ? AND status = 'PENDING')
	`, created.ID, created.ID).Scan(&identityPending, &metadataPending); err != nil {
		t.Fatal(err)
	}
	if identityPending != 1 || metadataPending != 1 {
		t.Fatalf("pending suggestions = identity %d metadata %d", identityPending, metadataPending)
	}
}

func TestMarkerImportUsesFolderTitleAndOffersExistingEntityMatches(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 27, 18, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alice", Aliases: []string{"爱丽丝"}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	work, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{
		Name: "Fate/stay night", Aliases: []string{"命运之夜"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{
		Name: "Saber", Aliases: []string{"阿尔托莉雅"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{
		LibraryID: mediaLibrary.ID, Name: "Marker", Kind: discovery.RuleKindMarker,
		Enabled: true, AutoCreateDraft: true,
	}, now); err != nil {
		t.Fatal(err)
	}
	folderName := "爱丽丝 - 命运之夜 - 阿尔托莉雅"
	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: folderName, HasRootMarker: true, MediaCount: 3,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 1 || snapshot.Candidates[0].GalleryID == nil ||
		snapshot.Candidates[0].Status != "IMPORTED" {
		t.Fatalf("marker candidates = %#v", snapshot.Candidates)
	}
	created, err := db.Galleries().Find(ctx, *snapshot.Candidates[0].GalleryID)
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != folderName {
		t.Fatalf("marker title = %q, want %q", created.Title, folderName)
	}
	detail, err := db.Manage().GalleryDetail(ctx, created.SetID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, match := range detail.FolderMatches {
		got[match.Kind] = match.UUID
	}
	if got["COSER"] != coser.UUID || got["WORK"] != work.UUID || got["CHARACTER"] != character.UUID {
		t.Fatalf("folder matches = %#v", detail.FolderMatches)
	}
}

func TestBoundAndIgnoredSourcesOutrankAutomaticRules(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{
		LibraryID: mediaLibrary.ID, Name: "Direct child", Kind: discovery.RuleKindFixedDepth,
		Enabled: true, FixedDepth: 1,
	}, now); err != nil {
		t.Fatal(err)
	}

	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Bound"}, now)
	if err != nil {
		t.Fatal(err)
	}
	boundPath := filepath.Join(mediaLibrary.RootPath, "bound")
	if _, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &mediaLibrary.ID, Type: gallery.SourceTypeDirectory,
		Path: boundPath, Availability: gallery.AvailabilityAvailable,
	}, now); err != nil {
		t.Fatal(err)
	}
	ignoredPath := filepath.Join(mediaLibrary.RootPath, "ignored")
	if err := db.CandidateDiscovery().IgnoreSource(
		ctx, &mediaLibrary.ID, nil, ignoredPath, "deleted Gallery", now,
	); err != nil {
		t.Fatal(err)
	}

	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{
		{RelativePath: "bound/nested", MediaCount: 2},
		{RelativePath: "ignored/nested", MediaCount: 2},
		{RelativePath: "new/nested", MediaCount: 2},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 1 || snapshot.Candidates[0].RootPath != filepath.Join(mediaLibrary.RootPath, "new") {
		t.Fatalf("priority snapshot = %#v", snapshot)
	}
}

func TestDiscoverySnapshotFailureIsAtomicAndOverLimitDoesNotAutoCreate(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{
		LibraryID: mediaLibrary.ID, Name: "Auto direct child", Kind: discovery.RuleKindFixedDepth,
		Enabled: true, AutoCreateDraft: true, FixedDepth: 1,
	}, now); err != nil {
		t.Fatal(err)
	}

	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: "large/nested", MediaCount: 1001,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 1 || !snapshot.Candidates[0].OverLimit || snapshot.Candidates[0].GalleryID != nil {
		t.Fatalf("over-limit candidate = %#v", snapshot.Candidates)
	}

	if _, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{
		{RelativePath: "valid/set", MediaCount: 1},
		{RelativePath: "../invalid", MediaCount: 1},
	}, now); err == nil {
		t.Fatal("invalid observation unexpectedly committed")
	}
	var snapshotCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM discovery_snapshots`).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if snapshotCount != 1 {
		t.Fatalf("failed discovery leaked snapshot; count = %d", snapshotCount)
	}
}

func TestManifestSetIDCreatesExplicitSourceRebindCandidate(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Moved set"}, now)
	if err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(mediaLibrary.RootPath, "old")
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &mediaLibrary.ID, Type: gallery.SourceTypeDirectory,
		Path: oldPath, Availability: gallery.AvailabilityMissing,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: "new", HasValidManifest: true, ManifestSetID: created.SetID, MediaCount: 2,
	}}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 1 {
		t.Fatalf("rebind candidates = %#v", snapshot.Candidates)
	}
	candidate := snapshot.Candidates[0]
	if candidate.Status != "SOURCE_REBIND_CANDIDATE" || candidate.RebindGalleryID == nil ||
		*candidate.RebindGalleryID != created.ID || candidate.GalleryID != nil || candidate.HasConflict {
		t.Fatalf("rebind candidate = %#v", candidate)
	}
	if _, err := db.CandidateDiscovery().ImportCandidate(ctx, candidate.ID, now); err == nil {
		t.Fatal("source rebind candidate unexpectedly created a second Gallery")
	}
	rebound, err := db.CandidateDiscovery().ConfirmSourceRebind(ctx, candidate.ID, false, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if rebound.ID != source.ID || rebound.Path != filepath.Join(mediaLibrary.RootPath, "new") ||
		rebound.ReconcileState != gallery.ReconcileNeedsRescan {
		t.Fatalf("rebound Source = %#v", rebound)
	}
	var ignoredOld int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ignored_gallery_sources WHERE source_path = ?`, oldPath).Scan(&ignoredOld); err != nil {
		t.Fatal(err)
	}
	if ignoredOld != 1 {
		t.Fatal("old source path was not protected from silent rediscovery")
	}
}

func TestManifestSetIDDuplicateNeedsExplicitOverrideAndIgnoreWins(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Copied set"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &mediaLibrary.ID, Type: gallery.SourceTypeDirectory,
		Path: filepath.Join(mediaLibrary.RootPath, "original"), Availability: gallery.AvailabilityAvailable,
	}, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: "copy", HasValidManifest: true, ManifestSetID: created.SetID, MediaCount: 1,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	candidate := snapshot.Candidates[0]
	if !candidate.HasConflict {
		t.Fatalf("accessible duplicate did not report conflict: %#v", candidate)
	}
	if _, err := db.CandidateDiscovery().ConfirmSourceRebind(ctx, candidate.ID, false, now); err == nil {
		t.Fatal("accessible duplicate rebound without explicit override")
	}

	ignoredPath := filepath.Join(mediaLibrary.RootPath, "ignored-copy")
	if err := db.CandidateDiscovery().IgnoreSource(ctx, &mediaLibrary.ID, &created.SetID, ignoredPath, "deleted", now); err != nil {
		t.Fatal(err)
	}
	ignoredSnapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: "ignored-copy", HasValidManifest: true, ManifestSetID: created.SetID, MediaCount: 1,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ignoredSnapshot.Candidates) != 0 || len(ignoredSnapshot.Unassigned) != 0 {
		t.Fatalf("ignored set_id leaked into discovery: %#v", ignoredSnapshot)
	}
}

func TestArchiveCandidateUsesSameTwoStageDraftFlow(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: "archives/set.cbz", SourceType: gallery.SourceTypeArchive,
		HasValidManifest: true, MediaCount: 20,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 1 || snapshot.Candidates[0].SourceType != gallery.SourceTypeArchive {
		t.Fatalf("archive candidate = %#v", snapshot.Candidates)
	}
	created, err := db.CandidateDiscovery().ImportCandidate(ctx, snapshot.Candidates[0].ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if created.State != gallery.StateDraft {
		t.Fatalf("archive import state = %s", created.State)
	}
	if _, err := db.CandidateDiscovery().ImportCandidate(ctx, snapshot.Candidates[0].ID, now); err == nil || errors.Is(err, ErrGalleryNotFound) {
		t.Fatalf("second archive import error = %v", err)
	}
}

func TestArchiveCandidateUsesFilenameTitleFallbackWithoutManifest(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 21, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{LibraryID: mediaLibrary.ID, Name: "Archive depth", Kind: discovery.RuleKindFixedDepth, Enabled: true, FixedDepth: 2, Order: 1}, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: "archives/Blue Archive Vol.1.cbz", SourceType: gallery.SourceTypeArchive, MediaCount: 1,
	}}, now)
	if err != nil || len(snapshot.Candidates) != 1 {
		t.Fatalf("archive snapshot = %#v, err=%v", snapshot, err)
	}
	created, err := db.CandidateDiscovery().ImportCandidate(ctx, snapshot.Candidates[0].ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "Blue Archive Vol.1" {
		t.Fatalf("archive title = %q, want filename stem", created.Title)
	}
}

func TestCandidateFirstImportUsesManifestSetIDAndPullsMetadata(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	mediaLibrary := createTestLibrary(t, db, now)
	setID := "a2345678-1234-4123-8123-123456789abc"
	root := filepath.Join(mediaLibrary.RootPath, "manifest-set")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := `{
		"schema_version":1,"revision":0,"set_id":"` + setID + `",
		"updated_at":"2026-07-23T03:00:00Z","title":"Manifest title",
		"content_rating":"NON_ADULT"
	}`
	if err := os.WriteFile(filepath.Join(root, ".cosplay.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.CandidateDiscovery().CommitSnapshot(ctx, mediaLibrary.ID, []ObservedDirectory{{
		RelativePath: "manifest-set", HasValidManifest: true, ManifestSetID: setID, MediaCount: 1,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Candidates) != 1 {
		t.Fatalf("Manifest candidate = %#v", snapshot.Candidates)
	}
	created, err := db.CandidateDiscovery().ImportCandidate(ctx, snapshot.Candidates[0].ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if created.SetID != setID || created.Title != "Manifest title" || created.MetadataRevision != 2 {
		t.Fatalf("first-import Gallery = %#v", created)
	}
	state, err := db.Manifests().CheckGallery(ctx, created.ID, now)
	if err != nil || state.Status != ManifestClean {
		t.Fatalf("first-import Manifest state = %#v, %v", state, err)
	}
}

func createTestLibrary(t *testing.T, db *Database, now time.Time) library.Library {
	t.Helper()
	created, err := db.Libraries().Create(context.Background(), CreateLibraryInput{
		Name: "Library", RootPath: t.TempDir(), Enabled: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return created
}
