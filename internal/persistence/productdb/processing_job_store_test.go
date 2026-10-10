package productdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestManualGalleryScanPromotesCurrentBaseJobsAheadOfBackgroundAndOlderManualWork(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.ProcessingJobs()
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	var galleryIDs [2]int64
	var firstItemUUID string
	var firstRevision int64
	for index := range galleryIDs {
		created, source := createEmptySourceFixture(t, db, now)
		galleryIDs[index] = created.ID
		run, err := db.Scans().Begin(ctx, source.ID, now)
		if err != nil {
			t.Fatal(err)
		}
		photo := scanPhoto("chapter/photo.jpg", "photo")
		photo.ProcessingState = gallery.ProcessingPending
		if err := db.Scans().Stage(ctx, run, photo); err != nil {
			t.Fatal(err)
		}
		if err := db.Scans().Commit(ctx, run, now); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			item := loadGalleryItemsForTest(t, db, created.ID)[0]
			firstItemUUID, firstRevision = item.UUID, item.ContentRevision
		}
	}
	interactive, err := store.Enqueue(ctx, EnqueueJobInput{Key: ItemDerivativeJobKey(firstItemUUID, mediaprocessing.VariantCard960, firstRevision, "test-profile"),
		Kind: mediaprocessing.JobItemDerivative, GalleryID: &galleryIDs[0], ItemUUID: firstItemUUID, Variant: mediaprocessing.VariantCard960,
		ContentRevision: &firstRevision, ProfileHash: "test-profile", Priority: 600}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, galleryID := range galleryIDs {
		count, err := store.PrioritizeManualGalleryScan(ctx, galleryID, now)
		if err != nil || count != 1 {
			t.Fatalf("promote gallery %d: count=%d err=%v", galleryID, count, err)
		}
	}
	for _, wanted := range []int64{galleryIDs[1], galleryIDs[0], galleryIDs[0]} {
		job, err := store.ClaimNext(ctx, "worker", time.Minute, now)
		if err != nil || job.GalleryID == nil || *job.GalleryID != wanted {
			t.Fatalf("claim gallery=%d: job=%#v err=%v", wanted, job, err)
		}
		if wanted == galleryIDs[0] && job.ID == interactive.ID && job.Priority != 600 {
			t.Fatalf("interactive job was unexpectedly promoted: %#v", job)
		}
	}
}

func TestManualGalleryScanPromotesDateBackfillWithoutRetryingFailedJobs(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	run, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Stage(ctx, run, scanPhoto("chapter/photo.jpg", "photo")); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Commit(ctx, run, now); err != nil {
		t.Fatal(err)
	}
	queued, err := db.CaptureDates().EnqueueGallery(ctx, created.ID, true, now)
	if err != nil || queued != 1 {
		t.Fatalf("date queue=%d err=%v", queued, err)
	}
	count, err := db.ProcessingJobs().PrioritizeManualGalleryScan(ctx, created.ID, now)
	if err != nil || count != 1 {
		t.Fatalf("date promotion=%d err=%v", count, err)
	}
	job, err := db.ProcessingJobs().ClaimNext(ctx, "worker", time.Minute, now)
	if err != nil || job.Variant != CaptureDateVariant || job.Priority < ManualGalleryPriorityFloor {
		t.Fatalf("claimed date job=%#v err=%v", job, err)
	}
	if _, err := db.ProcessingJobs().Fail(ctx, job.ID, "worker", "INVALID", true, now); err != nil {
		t.Fatal(err)
	}
	count, err = db.ProcessingJobs().PrioritizeManualGalleryScan(ctx, created.ID, now.Add(time.Minute))
	if err != nil || count != 0 {
		t.Fatalf("terminal failure was revived: count=%d err=%v", count, err)
	}
}

func TestProcessingJobsAreIdempotentPriorityLeasedAndRecoverable(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.ProcessingJobs()
	now := time.Date(2026, 7, 23, 6, 0, 0, 0, time.UTC)

	created, err := store.Enqueue(ctx, EnqueueJobInput{Key: "cache:maintenance", Kind: mediaprocessing.JobCache, Priority: 10}, now)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := store.Enqueue(ctx, EnqueueJobInput{Key: "cache:maintenance", Kind: mediaprocessing.JobCache, Priority: 100}, now)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != created.ID || duplicate.Priority != 100 {
		t.Fatalf("idempotent enqueue = %#v then %#v", created, duplicate)
	}
	claimed, err := store.ClaimNext(ctx, "worker-a", time.Minute, now)
	if err != nil || claimed.ID != created.ID || claimed.Status != mediaprocessing.JobRunning || claimed.AttemptCount != 1 {
		t.Fatalf("claimed job = %#v, %v", claimed, err)
	}
	if err := store.Heartbeat(ctx, claimed.ID, "worker-b", time.Minute, now); !errors.Is(err, ErrJobLeaseLost) {
		t.Fatalf("foreign heartbeat error = %v", err)
	}
	if _, err := store.ClaimNext(ctx, "worker-b", time.Minute, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("expired job was not recovered: %v", err)
	}
	if err := store.Complete(ctx, claimed.ID, "worker-a", now); !errors.Is(err, ErrJobLeaseLost) {
		t.Fatalf("expired owner completed job: %v", err)
	}
	if err := store.Complete(ctx, claimed.ID, "worker-b", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimNext(ctx, "worker-c", time.Minute, now.Add(3*time.Minute)); !errors.Is(err, ErrJobNotClaimable) {
		t.Fatalf("completed job was claimed: %v", err)
	}
}

func TestProcessingJobRetriesThenDeadLettersAndStructuralFailureDoesNotRetry(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.ProcessingJobs()
	now := time.Date(2026, 7, 23, 7, 0, 0, 0, time.UTC)

	if _, err := store.Enqueue(ctx, EnqueueJobInput{Key: "backup:one", Kind: mediaprocessing.JobBackup, Priority: 1, MaxAttempts: 2}, now); err != nil {
		t.Fatal(err)
	}
	first, err := store.ClaimNext(ctx, "worker", time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Fail(ctx, first.ID, "worker", "TEMPORARY", false, now)
	if err != nil || status != mediaprocessing.JobRetryWait {
		t.Fatalf("first failure = %s, %v", status, err)
	}
	second, err := store.ClaimNext(ctx, "worker", time.Minute, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	status, err = store.Fail(ctx, second.ID, "worker", "TEMPORARY", false, now.Add(3*time.Second))
	if err != nil || status != mediaprocessing.JobFailed {
		t.Fatalf("exhausted failure = %s, %v", status, err)
	}

	if _, err := store.Enqueue(ctx, EnqueueJobInput{Key: "cache:unsupported", Kind: mediaprocessing.JobCache, Priority: 1}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	structural, err := store.ClaimNext(ctx, "worker", time.Minute, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	status, err = store.Fail(ctx, structural.ID, "worker", "UNSUPPORTED", true, now.Add(time.Minute))
	if err != nil || status != mediaprocessing.JobFailed {
		t.Fatalf("structural failure = %s, %v", status, err)
	}
}
