package productdb

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestVideoProbeBackfillIsBoundedIdempotentAndDoesNotChangeGalleryMetadata(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	created, source := createBrowseGallery(t, db, "Video backfill", gallery.ContentRatingNonAdult, now)
	first := addBrowseItem(t, db, created.ID, source.ID, "first.mp4", gallery.MediaKindVideo, "", gallery.AvailabilityAvailable, gallery.ProcessingPending, 1024, now)
	second := addBrowseItem(t, db, created.ID, source.ID, "second.mp4", gallery.MediaKindVideo, "", gallery.AvailabilityAvailable, gallery.ProcessingPending, 2048, now)
	var before int64
	if err := db.QueryRowContext(ctx, `SELECT metadata_revision FROM galleries WHERE id=?`, created.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	profile := mediaprocessing.VideoProbeProfileHash("6.1")
	queued, err := db.VideoMetadata().EnqueueBackfill(ctx, profile, 1, now.Add(time.Minute))
	if err != nil || queued != 1 {
		t.Fatalf("first bounded backfill = %d, %v", queued, err)
	}
	queued, err = db.VideoMetadata().EnqueueBackfill(ctx, profile, 10, now.Add(2*time.Minute))
	if err != nil || queued != 1 {
		t.Fatalf("second bounded backfill = %d, %v", queued, err)
	}
	queued, err = db.VideoMetadata().EnqueueBackfill(ctx, profile, 10, now.Add(3*time.Minute))
	if err != nil || queued != 0 {
		t.Fatalf("idempotent backfill = %d, %v", queued, err)
	}
	for _, item := range []gallery.Item{first, second} {
		metadata, err := db.VideoMetadata().Find(ctx, item.UUID)
		if err != nil || metadata.ProbeState != mediaprocessing.VideoProbePending || metadata.ContentRevision != item.ContentRevision || metadata.ProbeProfileHash != profile {
			t.Fatalf("metadata for %s = %#v, %v", item.UUID, metadata, err)
		}
		if _, err := db.ProcessingJobs().FindByKey(ctx, ItemTechnicalMetadataJobKey(item.UUID, item.ContentRevision, profile)); err != nil {
			t.Fatalf("probe job for %s: %v", item.UUID, err)
		}
	}
	var after int64
	if err := db.QueryRowContext(ctx, `SELECT metadata_revision FROM galleries WHERE id=?`, created.ID).Scan(&after); err != nil || after != before {
		t.Fatalf("metadata revision changed: before=%d after=%d err=%v", before, after, err)
	}
}

func TestVideoProbeBackfillDoesNotAutomaticallyRetryRecordedError(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 15, 13, 0, 0, 0, time.UTC)
	created, source := createBrowseGallery(t, db, "Video error", gallery.ContentRatingNonAdult, now)
	item := addBrowseItem(t, db, created.ID, source.ID, "broken.mp4", gallery.MediaKindVideo, "", gallery.AvailabilityAvailable, gallery.ProcessingPending, 1024, now)
	profile := mediaprocessing.VideoProbeProfileHash("6.1")
	if err := db.VideoMetadata().MarkPending(ctx, item.UUID, item.ContentRevision, profile); err != nil {
		t.Fatal(err)
	}
	if err := db.VideoMetadata().PublishError(ctx, item.UUID, item.ContentRevision, profile, mediaprocessing.ErrorVideoTrackMissing, now); err != nil {
		t.Fatal(err)
	}
	queued, err := db.VideoMetadata().EnqueueBackfill(ctx, profile, 10, now.Add(time.Minute))
	if err != nil || queued != 0 {
		t.Fatalf("error backfill = %d, %v", queued, err)
	}
	if _, err := db.ProcessingJobs().FindByKey(ctx, ItemTechnicalMetadataJobKey(item.UUID, item.ContentRevision, profile)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("error row was automatically retried: %v", err)
	}
}
