package productdb

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestRequestLightboxIsVisibleIdempotentAndReopensEvictedJob(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	created, source := createBrowseGallery(t, db, "On demand", gallery.ContentRatingNonAdult, now)
	item := addBrowseItem(t, db, created.ID, source.ID, "photo.jpg", gallery.MediaKindStaticImage,
		gallery.ImageCategoryPhoto, gallery.AvailabilityAvailable, gallery.ProcessingPending, 1024, now)
	profile := mediaprocessing.DefaultProfileHash()
	if _, err := db.Derivatives().Publish(ctx, PublishDerivativeInput{
		ItemUUID: item.UUID, Variant: mediaprocessing.VariantCard480, CacheTier: mediaprocessing.CacheBase,
		ContentRevision: item.ContentRevision, ProfileHash: profile, CacheRelativePath: "on-demand/card.jpg",
		MIMEType: "image/jpeg", ByteSize: 4,
	}, now); err != nil {
		t.Fatal(err)
	}
	activateBrowseFixture(t, db, created.ID, now)

	first, err := db.Browse().RequestLightbox(ctx, item.UUID, now.Add(time.Minute))
	if err != nil || first.Status != gallery.ProcessingPending || first.Resource != nil {
		t.Fatalf("first request = %#v, %v", first, err)
	}
	key := ItemDerivativeJobKey(item.UUID, mediaprocessing.VariantLightbox4096, item.ContentRevision, profile)
	job, err := db.ProcessingJobs().FindByKey(ctx, key)
	if err != nil || job.Status != mediaprocessing.JobPending || !strings.Contains(string(job.PayloadJSON), "ENHANCED") {
		t.Fatalf("queued job = %#v, %v", job, err)
	}
	if _, err := db.Browse().RequestLightbox(ctx, item.UUID, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	claimed, err := db.ProcessingJobs().ClaimNext(ctx, "test", time.Minute, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ProcessingJobs().Complete(ctx, claimed.ID, "test", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Browse().RequestLightbox(ctx, item.UUID, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	reopened, err := db.ProcessingJobs().FindByKey(ctx, key)
	if err != nil || reopened.Status != mediaprocessing.JobPending {
		t.Fatalf("reopened job = %#v, %v", reopened, err)
	}

	if _, err := db.Browse().LightboxStatus(ctx, "01900000-0000-7000-8000-000000000000"); err != ErrBrowseGalleryNotVisible {
		t.Fatalf("invisible status error = %v", err)
	}
}

func TestAdoptOnDemandLightboxPolicyMigratesExistingRowsAndJobs(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 31, 11, 0, 0, 0, time.UTC)
	created, source := createBrowseGallery(t, db, "Migration", gallery.ContentRatingNonAdult, now)
	item := addBrowseItem(t, db, created.ID, source.ID, "photo.jpg", gallery.MediaKindStaticImage,
		gallery.ImageCategoryPhoto, gallery.AvailabilityAvailable, gallery.ProcessingReady, 1024, now)
	profile := mediaprocessing.DefaultProfileHash()
	if _, err := db.ExecContext(ctx, `INSERT INTO media_derivatives (item_uuid,variant,cache_tier,content_revision,
		profile_hash,state,is_current,cache_relative_path,mime_type,byte_size,width,height,created_at_utc,last_accessed_at_utc)
		VALUES (?,?, 'BASE',?,?,'READY',1,'migration/lightbox.jpg','image/jpeg',10,10,10,?,?)`,
		item.UUID, mediaprocessing.VariantLightbox4096, item.ContentRevision, profile, formatTime(now), formatTime(now)); err != nil {
		t.Fatal(err)
	}
	revision := item.ContentRevision
	key := ItemDerivativeJobKey(item.UUID, mediaprocessing.VariantLightbox4096, revision, profile)
	if _, err := db.ProcessingJobs().Enqueue(ctx, EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemDerivative,
		GalleryID: &created.ID, ItemUUID: item.UUID, Variant: mediaprocessing.VariantLightbox4096,
		ContentRevision: &revision, ProfileHash: profile, Payload: map[string]any{"cache_tier": mediaprocessing.CacheBase}}, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Derivatives().AdoptOnDemandLightboxPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Derivatives().AdoptOnDemandLightboxPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	var tier string
	if err := db.QueryRowContext(ctx, `SELECT cache_tier FROM media_derivatives WHERE item_uuid=? AND variant=?`, item.UUID,
		mediaprocessing.VariantLightbox4096).Scan(&tier); err != nil || tier != string(mediaprocessing.CacheEnhanced) {
		t.Fatalf("tier = %q, %v", tier, err)
	}
	job, err := db.ProcessingJobs().FindByKey(ctx, key)
	if err != nil || !strings.Contains(string(job.PayloadJSON), "ENHANCED") {
		t.Fatalf("migrated job = %#v, %v", job, err)
	}
}
