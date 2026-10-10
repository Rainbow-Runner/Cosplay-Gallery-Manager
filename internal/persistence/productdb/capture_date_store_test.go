package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestCaptureDateManualConflictRequiresReview(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	run, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, observation := range []ScanObservation{scanPhoto("a.jpg", "a"), scanPhoto("b.jpg", "b")} {
		if err := db.Scans().Stage(ctx, run, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().Commit(ctx, run, now); err != nil {
		t.Fatal(err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	if _, err := db.ExecContext(ctx, `UPDATE galleries SET shoot_date='2024-05-20',shoot_date_precision='DAY' WHERE id=?`, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.CaptureDates().Publish(ctx, items[0].UUID, items[0].ContentRevision, "2024-05-12", "exif.DateTimeOriginal", now); err != nil {
		t.Fatal(err)
	}
	summary, err := db.CaptureDates().Summary(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ReviewStatus != "" || summary.Images.Start != "" {
		t.Fatalf("incomplete capture evidence exposed: %+v", summary)
	}
	if err := db.CaptureDates().Publish(ctx, items[1].UUID, items[1].ContentRevision, "2024-05-15", "exif.DateTimeOriginal", now); err != nil {
		t.Fatal(err)
	}
	summary, err = db.CaptureDates().Summary(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Candidate != "2024-05-12" || summary.Manual != "2024-05-20" || summary.ReviewStatus != "PENDING" || summary.Images.End != "2024-05-15" {
		t.Fatalf("review/range: %+v", summary)
	}
	page, err := db.Manage().GalleryPage(ctx, 1, "CAPTURE_DATE", "")
	if err != nil || page.Summary.CaptureDateAttention != 1 || len(page.Items) != 1 || page.Items[0].CaptureDateReviewStatus != "PENDING" {
		t.Fatalf("date review filter=%+v err=%v", page, err)
	}
	current, err := db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ShootDate != "2024-05-20" {
		t.Fatalf("manual date overwritten: %s", current.ShootDate)
	}
	if err := db.CaptureDates().Resolve(ctx, created.ID, current.MetadataRevision, "2024-05-12", "KEEP_MANUAL", now); err != nil {
		t.Fatal(err)
	}
	page, err = db.Manage().GalleryPage(ctx, 1, "CAPTURE_DATE", "")
	if err != nil || page.Summary.CaptureDateAttention != 0 || len(page.Items) != 0 {
		t.Fatalf("resolved date review still filtered=%+v err=%v", page, err)
	}
	summary, err = db.CaptureDates().Summary(ctx, created.ID)
	if err != nil || summary.ReviewStatus != "KEEP_MANUAL" {
		t.Fatalf("kept review: %+v %v", summary, err)
	}
	if err := db.CaptureDates().Resolve(ctx, created.ID, current.MetadataRevision, "2024-05-12", "USE_EXTRACTED", now); err == nil {
		t.Fatal("accepted a review that was already decided")
	}
}

func TestCaptureDateAutomaticallySetsEmptyShootDate(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	run, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Stage(ctx, run, scanPhoto("a.jpg", "a")); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Commit(ctx, run, now); err != nil {
		t.Fatal(err)
	}
	item := loadGalleryItemsForTest(t, db, created.ID)[0]
	if err := db.CaptureDates().Publish(ctx, item.UUID, item.ContentRevision, "2024-05-12", "exif.DateTimeOriginal", now); err != nil {
		t.Fatal(err)
	}
	current, err := db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ShootDate != "2024-05-12" {
		t.Fatalf("automatic shoot date=%q", current.ShootDate)
	}
	var origin string
	if err := db.QueryRowContext(ctx, `SELECT shoot_date_origin FROM galleries WHERE id=?`, created.ID).Scan(&origin); err != nil || origin != "AUTO" {
		t.Fatalf("origin=%q err=%v", origin, err)
	}
	secondRun, err := db.Scans().Begin(ctx, source.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, observation := range []ScanObservation{scanPhoto("a.jpg", "a"), scanPhoto("b.jpg", "b")} {
		if err := db.Scans().Stage(ctx, secondRun, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().Commit(ctx, secondRun, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	current, err = db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ShootDate != "" {
		t.Fatalf("incomplete new capture evidence retained stale date: %q", current.ShootDate)
	}
}

func TestCaptureDateBackfillWaitsForPrimaryProcessingThenRecoversFailure(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	run, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	photo := scanPhoto("a.jpg", "a")
	photo.ProcessingState = gallery.ProcessingPending
	video := scanVideo("b.mp4", "b")
	video.ProcessingState = gallery.ProcessingPending
	for _, observation := range []ScanObservation{photo, video} {
		if err := db.Scans().Stage(ctx, run, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().Commit(ctx, run, now); err != nil {
		t.Fatal(err)
	}
	if items := loadGalleryItemsForTest(t, db, created.ID); len(items) != 2 {
		t.Fatalf("scanned items=%d", len(items))
	}
	queued, err := db.CaptureDates().EnqueueBackfill(ctx, 25, true, now)
	if err != nil || queued != 0 {
		t.Fatalf("primary processing should take precedence: queued=%d err=%v", queued, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE processing_jobs SET status='FAILED' WHERE job_kind='ITEM_DERIVATIVE' AND variant='CARD_480' AND gallery_id=?`, created.ID); err != nil {
		t.Fatal(err)
	}
	queued, err = db.CaptureDates().EnqueueBackfill(ctx, 25, true, now.Add(time.Minute))
	if err != nil || queued != 1 {
		t.Fatalf("failed primary processing lost date fallback: queued=%d err=%v", queued, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE processing_jobs SET status='FAILED' WHERE job_kind='ITEM_TECHNICAL_METADATA' AND variant='' AND gallery_id=?`, created.ID); err != nil {
		t.Fatal(err)
	}
	queued, err = db.CaptureDates().EnqueueBackfill(ctx, 25, true, now.Add(2*time.Minute))
	if err != nil || queued != 1 {
		t.Fatalf("failed video probe lost date fallback: queued=%d err=%v", queued, err)
	}
}
