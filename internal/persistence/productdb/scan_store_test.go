package productdb

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaexclusion"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestScanInitialNaturalGroupedOrderAndAtomicAbort(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	scans := db.Scans()

	runID, err := scans.Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	observations := []ScanObservation{
		scanPhoto("10.jpg", "10"),
		scanVideo("clip.mp4", "v"),
		scanPhoto("2.jpg", "2"),
		scanAnimated("animation.gif", "g"),
		scanPhoto("1.jpg", "1"),
	}
	for _, observation := range observations {
		if err := scans.Stage(ctx, runID, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := scans.Commit(ctx, runID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	want := []string{"1.jpg", "2.jpg", "10.jpg", "animation.gif", "clip.mp4"}
	if len(items) != len(want) {
		t.Fatalf("items = %#v", items)
	}
	for index := range want {
		if items[index].RelativePath != want[index] || items[index].Position != int64(index+1)*1024 {
			t.Fatalf("item order at %d = %#v, want %q", index, items[index], want[index])
		}
	}

	abortRun, err := scans.Begin(ctx, source.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := scans.Stage(ctx, abortRun, scanPhoto("new.jpg", "new")); err != nil {
		t.Fatal(err)
	}
	if err := scans.Abort(ctx, abortRun, true, "USER_CANCELLED", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	afterAbort := loadGalleryItemsForTest(t, db, created.ID)
	if len(afterAbort) != len(items) {
		t.Fatalf("aborted scan changed item count from %d to %d", len(items), len(afterAbort))
	}
	var stagedCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM gallery_scan_observations WHERE scan_run_id = ?
	`, abortRun).Scan(&stagedCount); err != nil {
		t.Fatal(err)
	}
	if stagedCount != 0 {
		t.Fatalf("aborted scan retained %d observations", stagedCount)
	}
}

func TestSuccessfulScanAtomicallyEnqueuesPrimaryDerivativeJobs(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	runID, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	photo, video := scanPhoto("photo.jpg", "photo"), scanVideo("video.mp4", "video")
	photo.ProcessingState, video.ProcessingState = gallery.ProcessingPending, gallery.ProcessingPending
	for _, observation := range []ScanObservation{photo, video} {
		if err := db.Scans().Stage(ctx, runID, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().Commit(ctx, runID, now); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(ctx, `SELECT job_kind,variant,status,profile_hash FROM processing_jobs WHERE gallery_id=? ORDER BY job_kind,variant`, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var jobs []string
	for rows.Next() {
		var kind, variant, status, profile string
		if err := rows.Scan(&kind, &variant, &status, &profile); err != nil {
			t.Fatal(err)
		}
		if status != string(mediaprocessing.JobPending) || profile != mediaprocessing.DefaultProfileHash() {
			t.Fatalf("queued job = %s %s %s", variant, status, profile)
		}
		jobs = append(jobs, kind+":"+variant)
	}
	if len(jobs) != 2 || jobs[0] != string(mediaprocessing.JobItemDerivative)+":"+mediaprocessing.VariantCard480 || jobs[1] != string(mediaprocessing.JobItemTechnicalMetadata)+":" {
		t.Fatalf("queued jobs = %#v", jobs)
	}
}

func TestScanOptionExcludesOnlyNewRootMediaAndPreservesDurableChoices(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	scans := db.Scans()

	firstRun, err := scans.Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	root := scanPhoto("cover.jpg", "root")
	nested := scanPhoto("chapter-1/001.jpg", "nested")
	root.ProcessingState, nested.ProcessingState = gallery.ProcessingPending, gallery.ProcessingPending
	for _, observation := range []ScanObservation{root, nested} {
		if err := scans.Stage(ctx, firstRun, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := scans.CommitWithOptions(ctx, firstRun, ScanOptions{ExcludeNewRootMedia: true}, now); err != nil {
		t.Fatal(err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	byPath := map[string]gallery.Item{}
	for _, item := range items {
		byPath[item.RelativePath] = item
	}
	if !byPath["cover.jpg"].Excluded || byPath["chapter-1/001.jpg"].Excluded {
		t.Fatalf("root/nested exclusions = root:%t nested:%t", byPath["cover.jpg"].Excluded, byPath["chapter-1/001.jpg"].Excluded)
	}
	var queued int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE gallery_id=?`, created.ID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("processing jobs = %d, want only nested Item", queued)
	}

	current, err := db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().SetItemExcluded(ctx, byPath["cover.jpg"].ID, false, current.MetadataRevision, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE gallery_id=?`, created.ID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 2 {
		t.Fatalf("processing jobs after Restore = %d, want root Item queued immediately", queued)
	}
	secondRun, err := scans.Begin(ctx, source.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, observation := range []ScanObservation{root, nested, scanPhoto("loose.jpg", "loose")} {
		if err := scans.Stage(ctx, secondRun, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := scans.CommitWithOptions(ctx, secondRun, ScanOptions{ExcludeNewRootMedia: true}, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	items = loadGalleryItemsForTest(t, db, created.ID)
	byPath = map[string]gallery.Item{}
	for _, item := range items {
		byPath[item.RelativePath] = item
	}
	if byPath["cover.jpg"].Excluded {
		t.Fatal("rescan overwrote the restored root Item")
	}
	if !byPath["loose.jpg"].Excluded {
		t.Fatal("new root Item was not auto-excluded")
	}

	thirdRun, err := scans.Begin(ctx, source.ID, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, observation := range []ScanObservation{root, nested, scanPhoto("loose.jpg", "loose"), scanPhoto("included.jpg", "included")} {
		if err := scans.Stage(ctx, thirdRun, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := scans.CommitWithOptions(ctx, thirdRun, ScanOptions{ExcludeNewRootMedia: false}, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	items = loadGalleryItemsForTest(t, db, created.ID)
	for _, item := range items {
		if item.RelativePath == "included.jpg" && item.Excluded {
			t.Fatal("disabled root exclusion option still excluded a new root Item")
		}
	}
}

func TestDirectoryScanAppliesEffectiveExclusionRulesOnlyToNewItems(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	library, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Rule scan", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Rule scan"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &library.ID, Type: gallery.SourceTypeDirectory, Path: filepath.Join(root, "gallery"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{
		Name: "global discard", Enabled: true, Order: 20, Subject: mediaexclusion.SubjectParentFolder,
		Operator: mediaexclusion.OperatorExact, Pattern: "discard", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionExclude,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{
		LibraryID: &library.ID, Name: "keep library exception", Enabled: true, Order: 20, Subject: mediaexclusion.SubjectParentPath,
		Operator: mediaexclusion.OperatorExact, Pattern: "discard/keep", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionInclude,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{
		LibraryID: &library.ID, Name: "root include loses to scan option", Enabled: true, Order: 1, Subject: mediaexclusion.SubjectRelativePath,
		Operator: mediaexclusion.OperatorExact, Pattern: "loose.jpg", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionInclude,
	}, now); err != nil {
		t.Fatal(err)
	}

	firstRun, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	first := []ScanObservation{
		scanPhoto("discard/a.jpg", "discard-a"),
		scanPhoto("discard/keep/b.jpg", "keep-b"),
		scanPhoto("loose.jpg", "root-loose"),
	}
	for index := range first {
		first[index].ProcessingState = gallery.ProcessingPending
		if err := db.Scans().Stage(ctx, firstRun, first[index]); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().CommitWithOptions(ctx, firstRun, ScanOptions{ExcludeNewRootMedia: true}, now); err != nil {
		t.Fatal(err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	byPath := make(map[string]gallery.Item, len(items))
	for _, item := range items {
		byPath[item.RelativePath] = item
	}
	if !byPath["discard/a.jpg"].Excluded || byPath["discard/keep/b.jpg"].Excluded || !byPath["loose.jpg"].Excluded {
		t.Fatalf("rule/root exclusions: discard=%t keep=%t root=%t", byPath["discard/a.jpg"].Excluded, byPath["discard/keep/b.jpg"].Excluded, byPath["loose.jpg"].Excluded)
	}
	keepUUID := byPath["discard/keep/b.jpg"].UUID
	var jobs, applied int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE gallery_id=?`, created.ID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_exclusion_decisions WHERE gallery_id=? AND status='APPLIED'`, created.ID).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || applied != 1 {
		t.Fatalf("jobs=%d applied rule decisions=%d, want 1/1", jobs, applied)
	}

	if _, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{
		LibraryID: &library.ID, Name: "new high-priority exclusion", Enabled: true, Order: 0, Subject: mediaexclusion.SubjectParentPath,
		Operator: mediaexclusion.OperatorExact, Pattern: "discard/keep", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionExclude,
	}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	secondRun, err := db.Scans().Begin(ctx, source.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	second := []ScanObservation{
		scanPhoto("discard/a.jpg", "discard-a"),
		scanPhoto("discard/keep/moved-b.jpg", "keep-b"),
		scanPhoto("loose.jpg", "root-loose"),
		scanPhoto("discard/keep/c.jpg", "new-c"),
	}
	for _, observation := range second {
		if err := db.Scans().Stage(ctx, secondRun, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().CommitWithOptions(ctx, secondRun, ScanOptions{ExcludeNewRootMedia: true}, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	items = loadGalleryItemsForTest(t, db, created.ID)
	byPath = make(map[string]gallery.Item, len(items))
	for _, item := range items {
		byPath[item.RelativePath] = item
	}
	if byPath["discard/keep/moved-b.jpg"].Excluded || byPath["discard/keep/moved-b.jpg"].UUID != keepUUID {
		t.Fatal("fingerprint rebind did not preserve the existing Item exclusion decision and identity")
	}
	if !byPath["discard/keep/c.jpg"].Excluded {
		t.Fatal("new Item did not use the current effective rule")
	}
}

func TestMediaExclusionRulesDoNotApplyToArchiveSources(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 28, 13, 0, 0, 0, time.UTC)
	if _, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{
		Name: "directory-only", Enabled: true, Order: 1, Subject: mediaexclusion.SubjectParentFolder,
		Operator: mediaexclusion.OperatorExact, Pattern: "discard", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionExclude,
	}, now); err != nil {
		t.Fatal(err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Archive boundary"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		Type: gallery.SourceTypeArchive, Path: filepath.Join(t.TempDir(), "gallery.cbz"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	runID, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Stage(ctx, runID, scanPhoto("discard/a.jpg", "archive-a")); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Commit(ctx, runID, now); err != nil {
		t.Fatal(err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	if len(items) != 1 || items[0].Excluded {
		t.Fatalf("archive Item was affected by directory exclusion rules: %#v", items)
	}
}

func TestDirectoryScanExclusionRulesFilterAllSupportedMediaKinds(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 28, 13, 30, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	for _, rule := range []MediaExclusionRule{
		{Name: "static only", Enabled: true, Order: 10, Subject: mediaexclusion.SubjectParentFolder, Operator: mediaexclusion.OperatorExact, Pattern: "static-only", MediaKind: mediaexclusion.MediaKindStatic, Decision: mediaexclusion.DecisionExclude},
		{Name: "animated only", Enabled: true, Order: 10, Subject: mediaexclusion.SubjectParentFolder, Operator: mediaexclusion.OperatorExact, Pattern: "animated-only", MediaKind: mediaexclusion.MediaKindAnimated, Decision: mediaexclusion.DecisionExclude},
		{Name: "video only", Enabled: true, Order: 10, Subject: mediaexclusion.SubjectParentFolder, Operator: mediaexclusion.OperatorExact, Pattern: "video-only", MediaKind: mediaexclusion.MediaKindVideo, Decision: mediaexclusion.DecisionExclude},
	} {
		if _, err := db.MediaExclusionRules().Create(ctx, rule, now); err != nil {
			t.Fatal(err)
		}
	}
	runID, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	observations := []ScanObservation{
		scanPhoto("static-only/photo.jpg", "static-match"), scanVideo("static-only/video.mp4", "static-miss"),
		scanAnimated("animated-only/animation.gif", "animated-match"), scanPhoto("animated-only/photo.jpg", "animated-miss"),
		scanVideo("video-only/video.mp4", "video-match"), scanAnimated("video-only/animation.gif", "video-miss"),
	}
	for _, observation := range observations {
		if err := db.Scans().Stage(ctx, runID, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().Commit(ctx, runID, now); err != nil {
		t.Fatal(err)
	}
	wantExcluded := map[string]bool{"static-only/photo.jpg": true, "animated-only/animation.gif": true, "video-only/video.mp4": true}
	for _, item := range loadGalleryItemsForTest(t, db, created.ID) {
		if item.Excluded != wantExcluded[item.RelativePath] {
			t.Fatalf("%s excluded=%t, want %t", item.RelativePath, item.Excluded, wantExcluded[item.RelativePath])
		}
	}
}

func TestScanRebindsUniqueFingerprintAndPreservesSamePathIdentity(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	scans := db.Scans()

	firstRun, err := scans.Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := scans.Stage(ctx, firstRun, scanPhoto("old.jpg", "move")); err != nil {
		t.Fatal(err)
	}
	if err := scans.Stage(ctx, firstRun, scanPhoto("same.jpg", "before")); err != nil {
		t.Fatal(err)
	}
	if err := scans.Commit(ctx, firstRun, now); err != nil {
		t.Fatal(err)
	}
	first := loadGalleryItemsForTest(t, db, created.ID)
	byPath := map[string]gallery.Item{first[0].RelativePath: first[0], first[1].RelativePath: first[1]}
	oldItem := byPath["same.jpg"]
	profile := mediaprocessing.DefaultProfileHash()
	derivative, err := db.Derivatives().Publish(ctx, PublishDerivativeInput{ItemUUID: oldItem.UUID, Variant: mediaprocessing.VariantCard480,
		CacheTier: mediaprocessing.CacheBase, ContentRevision: 1, ProfileHash: profile, CacheRelativePath: "old/card.jpg", MIMEType: "image/jpeg", ByteSize: 10}, now)
	if err != nil {
		t.Fatal(err)
	}
	revision := int64(1)
	oldKey := ItemDerivativeJobKey(oldItem.UUID, mediaprocessing.VariantCard480, revision, profile)
	if _, err := db.ProcessingJobs().Enqueue(ctx, EnqueueJobInput{Key: oldKey, Kind: mediaprocessing.JobItemDerivative, GalleryID: &created.ID,
		ItemUUID: oldItem.UUID, Variant: mediaprocessing.VariantCard480, ContentRevision: &revision, ProfileHash: profile, Priority: 1}, now); err != nil {
		t.Fatal(err)
	}

	secondRun, err := scans.Begin(ctx, source.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := scans.Stage(ctx, secondRun, scanPhoto("moved.jpg", "move")); err != nil {
		t.Fatal(err)
	}
	if err := scans.Stage(ctx, secondRun, scanPhoto("same.jpg", "after")); err != nil {
		t.Fatal(err)
	}
	if err := scans.Commit(ctx, secondRun, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	second := loadGalleryItemsForTest(t, db, created.ID)
	secondByPath := map[string]gallery.Item{second[0].RelativePath: second[0], second[1].RelativePath: second[1]}
	if secondByPath["moved.jpg"].UUID != byPath["old.jpg"].UUID {
		t.Fatal("unique same-source fingerprint did not retain item_uuid after move")
	}
	changed := secondByPath["same.jpg"]
	if changed.UUID != byPath["same.jpg"].UUID || changed.ContentRevision != 2 || changed.ProcessingState != gallery.ProcessingPending {
		t.Fatalf("same-path content replacement = %#v", changed)
	}
	var derivativeState string
	var current int
	if err := db.QueryRowContext(ctx, `SELECT state,is_current FROM media_derivatives WHERE id=?`, derivative.ID).Scan(&derivativeState, &current); err != nil {
		t.Fatal(err)
	}
	if derivativeState != string(mediaprocessing.DerivativeHardInvalid) || current != 0 {
		t.Fatalf("old derivative after content replacement = %s current=%d", derivativeState, current)
	}
	oldJob, err := db.ProcessingJobs().FindByKey(ctx, oldKey)
	if err != nil || oldJob.Status != mediaprocessing.JobCancelled {
		t.Fatalf("old processing job = %#v, %v", oldJob, err)
	}
}

func TestScanAmbiguousFingerprintNeverMergesItems(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	scans := db.Scans()

	firstRun, _ := scans.Begin(ctx, source.ID, now)
	if err := scans.Stage(ctx, firstRun, scanPhoto("old.jpg", "duplicate")); err != nil {
		t.Fatal(err)
	}
	if err := scans.Commit(ctx, firstRun, now); err != nil {
		t.Fatal(err)
	}
	original := loadGalleryItemsForTest(t, db, created.ID)[0]

	secondRun, _ := scans.Begin(ctx, source.ID, now.Add(time.Minute))
	if err := scans.Stage(ctx, secondRun, scanPhoto("copy-a.jpg", "duplicate")); err != nil {
		t.Fatal(err)
	}
	if err := scans.Stage(ctx, secondRun, scanPhoto("copy-b.jpg", "duplicate")); err != nil {
		t.Fatal(err)
	}
	if err := scans.Commit(ctx, secondRun, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	if len(items) != 3 {
		t.Fatalf("ambiguous fingerprint item count = %d, want 3", len(items))
	}
	uuids := make(map[string]struct{})
	for _, item := range items {
		uuids[item.UUID] = struct{}{}
		if item.UUID == original.UUID && item.Availability != gallery.AvailabilityMissing {
			t.Fatalf("original ambiguous Item availability = %s, want MISSING", item.Availability)
		}
	}
	if len(uuids) != 3 {
		t.Fatalf("ambiguous files shared UUIDs: %#v", items)
	}
}

func TestOverLimitScanPreservesPreviousItemsAndMarksSource(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	scans := db.Scans()
	initialRun, _ := scans.Begin(ctx, source.ID, now)
	if err := scans.Stage(ctx, initialRun, scanPhoto("existing.jpg", "existing")); err != nil {
		t.Fatal(err)
	}
	if err := scans.Commit(ctx, initialRun, now); err != nil {
		t.Fatal(err)
	}
	original := loadGalleryItemsForTest(t, db, created.ID)[0]

	runID, _ := scans.Begin(ctx, source.ID, now.Add(time.Minute))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 1001; index++ {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_scan_observations (
				scan_run_id, relative_path, media_kind, image_category,
				full_fingerprint, processing_state
			) VALUES (?, ?, 'STATIC_IMAGE', 'PHOTO', ?, 'READY')
		`, runID, fmt.Sprintf("%04d.jpg", index), fingerprint(fmt.Sprintf("%d", index))); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := scans.Commit(ctx, runID, now.Add(2*time.Minute)); !errors.Is(err, ErrScanOverLimit) {
		t.Fatalf("over-limit commit error = %v", err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	if len(items) != 1 || items[0].UUID != original.UUID || items[0].Availability != gallery.AvailabilityAvailable {
		t.Fatalf("over-limit scan changed committed Items: %#v", items)
	}
	persistedSource, err := findSource(ctx, db.DB, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !persistedSource.OverLimit || persistedSource.ReconcileState != gallery.ReconcileError {
		t.Fatalf("over-limit Source = %#v", persistedSource)
	}
}

func TestNewRuleExcludedItemDoesNotCountTowardGalleryLimit(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 28, 14, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	if _, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{
		Name: "one limit exception", Enabled: true, Order: 1, Subject: mediaexclusion.SubjectFileName,
		Operator: mediaexclusion.OperatorExact, Pattern: "1000.jpg", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionExclude,
	}, now); err != nil {
		t.Fatal(err)
	}
	runID, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 1001; index++ {
		path := fmt.Sprintf("%04d.jpg", index)
		if _, err := tx.ExecContext(ctx, `INSERT INTO gallery_scan_observations
			(scan_run_id,relative_path,media_kind,image_category,full_fingerprint,processing_state)
			VALUES (?,?,'STATIC_IMAGE','PHOTO',?,'READY')`, runID, path, fingerprint(path)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Commit(ctx, runID, now.Add(time.Minute)); err != nil {
		t.Fatalf("effective 1000-member rule scan failed: %v", err)
	}
	var total, excluded int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*),SUM(excluded) FROM gallery_items WHERE gallery_id=?`, created.ID).Scan(&total, &excluded); err != nil {
		t.Fatal(err)
	}
	if total != 1001 || excluded != 1 {
		t.Fatalf("stored/excluded members=%d/%d, want 1001/1", total, excluded)
	}
}

func TestRunDirectoryScanIsIdempotentAndSourceFailurePreservesItems(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 21, 0, 0, 0, time.UTC)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("\xff\xd8\xff stable content"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Physical scan"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: root, Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Run(ctx, source.ID, archivecheck.DefaultLimits(), now); err != nil {
		t.Fatal(err)
	}
	first := loadGalleryItemsForTest(t, db, created.ID)
	if len(first) != 1 {
		t.Fatalf("first physical scan Items = %#v", first)
	}
	if err := db.Scans().Run(ctx, source.ID, archivecheck.DefaultLimits(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	second := loadGalleryItemsForTest(t, db, created.ID)
	if len(second) != 1 || second[0].UUID != first[0].UUID || second[0].ContentRevision != first[0].ContentRevision {
		t.Fatalf("unchanged rescan was not idempotent: first %#v second %#v", first, second)
	}

	movedRoot := root + "-offline"
	if err := os.Rename(root, movedRoot); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Run(ctx, source.ID, archivecheck.DefaultLimits(), now.Add(2*time.Minute)); err == nil {
		t.Fatal("unavailable source scan unexpectedly succeeded")
	}
	afterFailure := loadGalleryItemsForTest(t, db, created.ID)
	if len(afterFailure) != 1 || afterFailure[0].Availability != gallery.AvailabilityAvailable {
		t.Fatalf("source failure changed member availability: %#v", afterFailure)
	}
	persistedSource, err := findSource(ctx, db.DB, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedSource.Availability != gallery.AvailabilityUnreadable || persistedSource.ReconcileState != gallery.ReconcileError {
		t.Fatalf("failed physical Source state = %#v", persistedSource)
	}
}

func TestUnsafeArchiveScanKeepsLastCommittedSnapshot(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 21, 0, 0, 0, time.UTC)
	filename := filepath.Join(t.TempDir(), "unsafe.cbz")
	archiveFile, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archiveFile)
	part, err := writer.Create("../escape.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("\xff\xd8\xff content")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatal(err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Unsafe archive"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		Type: gallery.SourceTypeArchive, Path: filename, Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "previous.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Run(ctx, source.ID, archivecheck.DefaultLimits(), now.Add(time.Minute)); !errors.Is(err, ErrSourceScanUnsafe) {
		t.Fatalf("unsafe archive scan error = %v", err)
	}
	persisted, err := db.Galleries().FindItem(ctx, previous.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Availability != gallery.AvailabilityAvailable {
		t.Fatalf("unsafe archive scan changed previous Item: %#v", persisted)
	}
	var blocking int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM gallery_source_issues
		WHERE source_id = ? AND code = 'SCAN_UNSAFE_ENTRY_PATH'
			AND severity = 'BLOCKING' AND resolved_at_utc IS NULL
	`, source.ID).Scan(&blocking); err != nil {
		t.Fatal(err)
	}
	if blocking != 1 {
		t.Fatal("unsafe archive issue was not persisted")
	}
}

func createEmptySourceFixture(t *testing.T, db *Database, now time.Time) (gallery.Gallery, gallery.Source) {
	t.Helper()
	ctx := context.Background()
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Scan fixture"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: filepath.Join(t.TempDir(), "source"),
		Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return created, source
}

func loadGalleryItemsForTest(t *testing.T, db *Database, galleryID int64) []gallery.Item {
	t.Helper()
	rows, err := db.Query(`
		SELECT id FROM gallery_items WHERE gallery_id = ?
		ORDER BY CASE
			WHEN media_kind = 'STATIC_IMAGE' AND image_category = 'PHOTO' THEN 0
			WHEN media_kind = 'STATIC_IMAGE' AND image_category = 'SELFIE' THEN 1
			WHEN media_kind = 'ANIMATED_IMAGE' THEN 2 ELSE 3 END,
			position
	`, galleryID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	items := make([]gallery.Item, 0, len(ids))
	for _, id := range ids {
		item, err := findItem(context.Background(), db.DB, id)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, item)
	}
	return items
}

func scanPhoto(relativePath string, fingerprintValue string) ScanObservation {
	return ScanObservation{
		RelativePath: relativePath, MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, ByteSize: 100,
		FullFingerprint: fingerprint(fingerprintValue), ProcessingState: gallery.ProcessingReady,
	}
}

func scanAnimated(relativePath string, fingerprintValue string) ScanObservation {
	return ScanObservation{
		RelativePath: relativePath, MediaKind: gallery.MediaKindAnimatedImage,
		ByteSize: 100, FullFingerprint: fingerprint(fingerprintValue), ProcessingState: gallery.ProcessingReady,
	}
}

func scanVideo(relativePath string, fingerprintValue string) ScanObservation {
	return ScanObservation{
		RelativePath: relativePath, MediaKind: gallery.MediaKindVideo,
		ByteSize: 100, FullFingerprint: fingerprint(fingerprintValue), ProcessingState: gallery.ProcessingReady,
	}
}

func fingerprint(value string) string {
	return "blake3-v1:" + value
}
