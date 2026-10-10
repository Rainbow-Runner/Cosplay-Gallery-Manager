package productdb

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestRequestAnimatedPreviewUsesCompleteDurationProfileAndEnhancedJob(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 16, 14, 0, 0, 0, time.UTC)
	created, source := createBrowseGallery(t, db, "Animated preview", gallery.ContentRatingNonAdult, now)
	item := addBrowseItem(t, db, created.ID, source.ID, "motion.gif", gallery.MediaKindAnimatedImage,
		"", gallery.AvailabilityAvailable, gallery.ProcessingReady, 1024, now)
	activateBrowseFixture(t, db, created.ID, now)

	missing, err := db.Browse().RequestAnimatedPreview(ctx, item.UUID, "", mediaprocessing.ErrorFFmpegUnavailable, now.Add(time.Minute))
	if err != nil || missing.Status != gallery.ProcessingError || missing.ErrorCode != mediaprocessing.ErrorFFmpegUnavailable {
		t.Fatalf("missing FFmpeg = %#v, %v", missing, err)
	}
	first, err := db.Browse().RequestAnimatedPreview(ctx, item.UUID, "7.1.1", "", now.Add(2*time.Minute))
	if err != nil || first.Status != gallery.ProcessingPending || first.Resource != nil {
		t.Fatalf("first request = %#v, %v", first, err)
	}
	profile := mediaprocessing.AnimatedPreviewProfileHash("7.1.1")
	key := ItemDerivativeJobKey(item.UUID, mediaprocessing.VariantAnimatedPreview, item.ContentRevision, profile)
	job, err := db.ProcessingJobs().FindByKey(ctx, key)
	if err != nil || job.Status != mediaprocessing.JobPending || !strings.Contains(string(job.PayloadJSON), "ENHANCED") {
		t.Fatalf("animated job = %#v, %v", job, err)
	}
	if _, err := db.Browse().RequestAnimatedPreview(ctx, item.UUID, "7.1.1", "", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE item_uuid=? AND variant=?`, item.UUID, mediaprocessing.VariantAnimatedPreview).Scan(&count); err != nil || count != 1 {
		t.Fatalf("idempotent job count = %d, %v", count, err)
	}
}
