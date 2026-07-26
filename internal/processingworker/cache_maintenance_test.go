package processingworker

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

func TestMaintainEnhancedCacheNeverEvictsBaseResources(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Round(0)
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	galleryRecord, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Cache"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, galleryRecord.ID, productdb.CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: t.TempDir(), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, galleryRecord.ID, source.ID, productdb.CreateItemInput{
		RelativePath: "photo.jpg", MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable,
		ProcessingState: gallery.ProcessingPending,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	profile := mediaprocessing.DefaultProfileHash()
	cacheRoot := t.TempDir()
	cache := mediaprocessing.CacheWriter{Root: cacheRoot}
	enhancedPath, err := cache.RelativePath(item.UUID, item.ContentRevision, mediaprocessing.VariantCard960, profile, "jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.WriteAtomic(enhancedPath, func(output io.Writer) error {
		_, err := output.Write([]byte("enhanced"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	derivative, err := db.Derivatives().Publish(ctx, productdb.PublishDerivativeInput{
		ItemUUID: item.UUID, Variant: mediaprocessing.VariantCard960, CacheTier: mediaprocessing.CacheEnhanced,
		ContentRevision: item.ContentRevision, ProfileHash: profile, CacheRelativePath: enhancedPath,
		MIMEType: "image/jpeg", ByteSize: 8, Width: 480, Height: 640,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	key := productdb.ItemDerivativeJobKey(item.UUID, derivative.Variant, derivative.ContentRevision, derivative.ProfileHash)
	revision := item.ContentRevision
	job, err := db.ProcessingJobs().Enqueue(ctx, productdb.EnqueueJobInput{
		Key: key, Kind: mediaprocessing.JobItemDerivative, GalleryID: &galleryRecord.ID, ItemUUID: item.UUID,
		Variant: derivative.Variant, ContentRevision: &revision, ProfileHash: derivative.ProfileHash,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := db.ProcessingJobs().ClaimNext(ctx, "cache-test", time.Minute, now)
	if err != nil || claimed.ID != job.ID {
		t.Fatalf("claim = %#v, %v", claimed, err)
	}
	if err := db.ProcessingJobs().Complete(ctx, claimed.ID, "cache-test", now); err != nil {
		t.Fatal(err)
	}

	result, err := MaintainEnhancedCache(ctx, db, cache, mediaprocessing.CachePressure{
		EnhancedBytes: 8, MaximumEnhancedBytes: 1, AvailableBytes: 100, TotalBytes: 100,
		MinimumFreeBytes: 1, MinimumFreePercent: 0.01,
	}, 10, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || result.FreedBytes != 8 {
		t.Fatalf("maintenance result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, filepath.FromSlash(enhancedPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("enhanced cache still exists: %v", err)
	}
	if _, err := db.Derivatives().Current(ctx, item.UUID, mediaprocessing.VariantCard960, now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("evicted derivative still current: %v", err)
	}
	requeued, err := db.ProcessingJobs().FindByKey(ctx, key)
	if err != nil || requeued.Status != mediaprocessing.JobPending {
		t.Fatalf("requeued job = %#v, %v", requeued, err)
	}
}
