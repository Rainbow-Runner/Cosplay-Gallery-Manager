package productdb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/discovery"
	"github.com/stashapp/stash/internal/gallery"
)

func TestAutomationPolicyDefaultsToManualAndRequiresExplicitTrustedActivation(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 29, 2, 0, 0, 0, time.UTC)
	library := createTestLibrary(t, db, now)

	policy, err := db.Automation().FindPolicy(ctx, library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode != AutomationManual || policy.Revision != 0 || !policy.ExcludeNewRootMedia || policy.AutoActivate {
		t.Fatalf("implicit policy = %#v", policy)
	}

	policy.Mode = AutomationAssisted
	policy.AutoActivate = true
	policy.DefaultContentRating = gallery.ContentRatingNonAdult
	if _, err := db.Automation().SavePolicy(ctx, policy, 0, now); err == nil {
		t.Fatal("ASSISTED policy unexpectedly enabled automatic activation")
	}
	policy.Mode = AutomationTrusted
	policy.DefaultContentRating = ""
	if _, err := db.Automation().SavePolicy(ctx, policy, 0, now); err == nil {
		t.Fatal("TRUSTED automatic activation unexpectedly allowed an empty default rating")
	}
}

func TestAutomationPolicyPersistsWithRevisionGuard(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 29, 3, 0, 0, 0, time.UTC)
	library := createTestLibrary(t, db, now)
	input := LibraryAutomationPolicy{
		LibraryID: library.ID, Mode: AutomationTrusted,
		DefaultContentRating: gallery.ContentRatingNonAdult,
		ExcludeNewRootMedia:  true, AutoAcceptUniqueEntities: true,
		AutoAcceptMediaClassification: true, AutoActivate: true,
	}

	created, err := db.Automation().SavePolicy(ctx, input, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision != 1 || !created.AutoAcceptUniqueEntities || !created.AutoAcceptMediaClassification {
		t.Fatalf("created policy = %#v", created)
	}
	queued, err := db.Automation().EnqueueRun(ctx, created, now.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	created.AutoActivate = false
	updated, err := db.Automation().SavePolicy(ctx, created, created.Revision, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.AutoActivate {
		t.Fatalf("updated policy = %#v", updated)
	}
	frozen, err := db.Automation().FindRun(ctx, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !frozen.AutoActivate || frozen.PolicyRevision != 1 || frozen.DefaultContentRating != gallery.ContentRatingNonAdult {
		t.Fatalf("queued policy snapshot changed with policy edit: %#v", frozen)
	}
	if _, err := db.Automation().SavePolicy(ctx, updated, 1, now.Add(2*time.Minute)); !errors.Is(err, ErrMetadataRevisionConflict) {
		t.Fatalf("stale policy update error = %v", err)
	}
}

func TestAutomationRunAllowsOnlyOneRunningPassPerLibrary(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 29, 3, 30, 0, 0, time.UTC)
	library := createTestLibrary(t, db, now)
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{
		LibraryID: library.ID, Mode: AutomationAssisted, ExcludeNewRootMedia: true,
	}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.Automation().EnqueueRun(ctx, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "QUEUED" {
		t.Fatalf("enqueued status = %q", first.Status)
	}
	if _, err := db.Automation().EnqueueRun(ctx, policy, now); err == nil {
		t.Fatal("second concurrent automation pass unexpectedly started")
	}
	claimed, err := db.Automation().ClaimNextRun(ctx, "worker", time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Automation().CompleteRun(ctx, claimed, "worker", "COMPLETED", "", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Automation().EnqueueRun(ctx, policy, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("new pass after completion: %v", err)
	}
}

func TestQueuedAutomationCanBeCancelledAndReplaced(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 29, 3, 45, 0, 0, time.UTC)
	library := createTestLibrary(t, db, now)
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{LibraryID: library.ID, Mode: AutomationAssisted}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := db.Automation().EnqueueRun(ctx, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := db.Automation().RequestCancel(ctx, queued.ID, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "CANCELLED" || cancelled.CompletedAtUTC == nil {
		t.Fatalf("cancelled run = %#v", cancelled)
	}
	if _, err := db.Automation().EnqueueRun(ctx, policy, now.Add(2*time.Second)); err != nil {
		t.Fatalf("replacement run: %v", err)
	}
}

func TestExpiredAutomationLeaseReturnsRunToQueue(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 29, 3, 50, 0, 0, time.UTC)
	library := createTestLibrary(t, db, now)
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{LibraryID: library.ID, Mode: AutomationAssisted}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Automation().EnqueueRun(ctx, policy, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Automation().ClaimNextRun(ctx, "lost-worker", time.Second, now); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := db.Automation().ClaimNextRun(ctx, "replacement-worker", time.Minute, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed.Status != "RUNNING" || reclaimed.LeaseOwner != "replacement-worker" || reclaimed.ErrorCode != "" {
		t.Fatalf("reclaimed run = %#v", reclaimed)
	}
}

func TestRunningAutomationCancellationStopsBeforeDiscovery(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 30, 1, 0, 0, 0, time.UTC)
	library := createTestLibrary(t, db, now)
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{LibraryID: library.ID, Mode: AutomationAssisted}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := db.Automation().EnqueueRun(ctx, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := db.Automation().ClaimNextRun(ctx, "cancel-worker", time.Minute, now)
	if err != nil || claimed.ID != queued.ID {
		t.Fatalf("claim = %#v err=%v", claimed, err)
	}
	requested, err := db.Automation().RequestCancel(ctx, claimed.ID, now.Add(time.Second))
	if err != nil || !requested.CancellationRequested || requested.Status != "RUNNING" {
		t.Fatalf("cancel request = %#v err=%v", requested, err)
	}
	finished, done, err := db.Automation().ProcessClaimedRunBatch(ctx, claimed.ID, "cancel-worker", 25, time.Minute, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !done || finished.Status != "CANCELLED" || finished.DiscoveryCompleted || finished.CandidatesSeen != 0 {
		t.Fatalf("cancelled run = %#v", finished)
	}
}

func TestAutomationHeartbeatRenewsLeaseWhileWorkIsBlocked(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Now().UTC()
	library := createTestLibrary(t, db, now)
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{LibraryID: library.ID, Mode: AutomationAssisted}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Automation().EnqueueRun(ctx, policy, now); err != nil {
		t.Fatal(err)
	}
	const lease = 90 * time.Millisecond
	claimed, err := db.Automation().ClaimNextRun(ctx, "heartbeat-worker", lease, now)
	if err != nil {
		t.Fatal(err)
	}
	_, stop := db.Automation().startAutomationRunHeartbeat(ctx, claimed.ID, "heartbeat-worker", lease)
	defer stop()
	deadline := time.Now().Add(time.Second)
	var renewed AutomationRun
	for time.Now().Before(deadline) {
		renewed, err = db.Automation().FindRun(ctx, claimed.ID)
		if err != nil {
			t.Fatal(err)
		}
		if renewed.LeaseExpiresAtUTC != nil && claimed.LeaseExpiresAtUTC != nil && renewed.LeaseExpiresAtUTC.After(*claimed.LeaseExpiresAtUTC) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if renewed.LeaseExpiresAtUTC == nil || claimed.LeaseExpiresAtUTC == nil || !renewed.LeaseExpiresAtUTC.After(*claimed.LeaseExpiresAtUTC) {
		t.Fatalf("lease was not renewed: initial=%v renewed=%v", claimed.LeaseExpiresAtUTC, renewed.LeaseExpiresAtUTC)
	}
	if _, err := db.Automation().ClaimNextRun(ctx, "replacement-worker", time.Minute, claimed.LeaseExpiresAtUTC.Add(time.Millisecond)); !errors.Is(err, ErrAutomationRunNotClaimable) {
		t.Fatalf("renewed run was reclaimed at its original expiry: %v", err)
	}
}

func TestAutomationResumesAfterCrashFromPersistedBatchCursor(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 30, 2, 0, 0, 0, time.UTC)
	library := createTestLibrary(t, db, now)
	insertAutomationDraftFixture(t, db, library.ID, library.RootPath, 5)
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{LibraryID: library.ID, Mode: AutomationAssisted}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Automation().EnqueueRun(ctx, policy, now); err != nil {
		t.Fatal(err)
	}
	claimed, err := db.Automation().ClaimNextRun(ctx, "first-worker", time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	claimed.DiscoveryCompleted = true
	if _, err := db.Automation().SaveRunProgress(ctx, claimed, "first-worker", now); err != nil {
		t.Fatal(err)
	}
	firstBatch, done, err := db.Automation().ProcessClaimedRunBatch(ctx, claimed.ID, "first-worker", 2, time.Minute, now)
	if err != nil || done {
		t.Fatalf("first batch done=%v run=%#v err=%v", done, firstBatch, err)
	}
	if firstBatch.Status != "QUEUED" || firstBatch.CursorGalleryID != 2 || firstBatch.NeedsReview != 2 {
		t.Fatalf("first batch progress = %#v", firstBatch)
	}
	if _, err := db.Automation().ClaimNextRun(ctx, "crashed-worker", 50*time.Millisecond, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	recovered, err := db.Automation().ClaimNextRun(ctx, "replacement-worker", time.Minute, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	finished, done, err := db.Automation().ProcessClaimedRunBatch(ctx, recovered.ID, "replacement-worker", 25, time.Minute, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !done || finished.Status != "COMPLETED" || finished.CursorGalleryID != 5 || finished.NeedsReview != 5 || finished.ErrorCode != "" {
		t.Fatalf("recovered run = %#v", finished)
	}
}

func TestAutomationPreviewIsEmptyBeforeDiscovery(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	library := createTestLibrary(t, db, time.Date(2026, 8, 29, 4, 0, 0, 0, time.UTC))

	preview, err := db.Automation().Preview(ctx, library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview != (AutomationPreview{}) {
		t.Fatalf("preview = %#v", preview)
	}
}

func TestAutomationPolicyFreezesArchiveImportConsentIntoRun(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 30, 19, 0, 0, 0, time.UTC)
	library := createTestLibrary(t, db, now)
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{
		LibraryID: library.ID, Mode: AutomationAssisted, ExcludeNewRootMedia: true, AutoImportArchives: true,
	}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.AutoImportArchives {
		t.Fatalf("saved policy = %#v", policy)
	}
	run, err := db.Automation().EnqueueRun(ctx, policy, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !run.AutoImportArchives {
		t.Fatalf("queued run did not freeze archive consent: %#v", run)
	}
}

func TestTrustedAutomationKeepsBlockedGalleryDraftAndRecordsIssue(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 29, 5, 0, 0, 0, time.UTC)
	library := createTestLibrary(t, db, now)
	root := filepath.Join(library.RootPath, "Miku Set")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cosplay-root"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "images"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "images", "01.jpg"), []byte("\xff\xd8\xff stable content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{
		LibraryID: library.ID, Name: "marker", Kind: discovery.RuleKindMarker,
		Enabled: true, AutoCreateDraft: true, Order: 1,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{
		LibraryID: library.ID, Mode: AutomationTrusted,
		DefaultContentRating: gallery.ContentRatingNonAdult, ExcludeNewRootMedia: true, AutoActivate: true,
	}, 0, now); err != nil {
		t.Fatal(err)
	}

	queued, err := db.Automation().EnqueueRun(ctx, mustAutomationPolicy(t, db, library.ID), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := db.Automation().ClaimNextRun(ctx, "test-worker", time.Hour, now.Add(time.Minute))
	if err != nil || claimed.ID != queued.ID {
		t.Fatalf("claim = %#v err=%v", claimed, err)
	}
	run, done, err := db.Automation().ProcessClaimedRunBatch(ctx, claimed.ID, "test-worker", 25, time.Hour, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if done || run.Status != "QUEUED" || run.Phase != "WAITING_FOR_MEDIA" || run.Scanned != 1 || run.NeedsReview != 0 {
		t.Fatalf("automation did not wait for media preparation: %#v", run)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET processing_state='ERROR'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE processing_jobs SET status='FAILED',last_error_code='TEST_DECODE_FAILED'`); err != nil {
		t.Fatal(err)
	}
	claimed, err = db.Automation().ClaimNextRun(ctx, "test-worker", time.Hour, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	run, done, err = db.Automation().ProcessClaimedRunBatch(ctx, claimed.ID, "test-worker", 25, time.Hour, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("automation run did not complete after media processing became terminal")
	}
	if run.Status != "COMPLETED" || run.CandidatesSeen != 1 || run.DraftsCreated != 1 || run.Scanned != 1 || run.Activated != 0 || run.NeedsReview != 1 || run.IssueCount != 1 {
		t.Fatalf("automation run = %#v", run)
	}
	var state, title, rating, reconcile string
	if err := db.QueryRowContext(ctx, `SELECT gallery.state,gallery.title,COALESCE(gallery.content_rating,''),source.reconcile_state
		FROM galleries gallery JOIN gallery_sources source ON source.gallery_id=gallery.id WHERE source.library_id=?`, library.ID).
		Scan(&state, &title, &rating, &reconcile); err != nil {
		t.Fatal(err)
	}
	if state != "DRAFT" || title != "images" || rating != "NON_ADULT" || reconcile != "IN_SYNC" {
		t.Fatalf("automated gallery = state=%q title=%q rating=%q reconcile=%q", state, title, rating, reconcile)
	}
}

func mustAutomationPolicy(t *testing.T, db *Database, libraryID int64) LibraryAutomationPolicy {
	t.Helper()
	value, err := db.Automation().FindPolicy(context.Background(), libraryID)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func insertAutomationDraftFixture(t *testing.T, db *Database, libraryID int64, root string, count int) {
	t.Helper()
	ctx := context.Background()
	timestamp := "2026-08-30T00:00:00.000Z"
	statements := []struct {
		query string
		args  []any
	}{
		{`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<?)
		 INSERT INTO portable_uuid_registry(uuid,entity_kind,created_at_utc)
		 SELECT printf('90000000-0000-4000-8000-%012d',value),'GALLERY',? FROM n`, []any{count, timestamp}},
		{`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<?)
		 INSERT INTO galleries(id,set_id,slug,state,title,created_at_utc,updated_at_utc)
		 SELECT value,printf('90000000-0000-4000-8000-%012d',value),printf('automation-gallery-%d',value),'DRAFT',
		 printf('Automation Gallery %d',value),?,? FROM n`, []any{count, timestamp, timestamp}},
		{`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<?)
		 INSERT INTO gallery_sources(id,gallery_id,library_id,source_type,source_path,availability_state,reconcile_state,created_at_utc,updated_at_utc)
		 SELECT value,value,?,'DIRECTORY',? || '/automation-' || value,'AVAILABLE','IN_SYNC',?,? FROM n`, []any{count, libraryID, root, timestamp, timestamp}},
	}
	for index, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("automation fixture statement %d: %v", index+1, err)
		}
	}
}
