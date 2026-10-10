package processingworker

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

func TestCardDerivativeStoresAndBackfillsCoverSignature(t *testing.T) {
	ctx, now := context.Background(), time.Now().UTC()
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	created, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Cover"}, now)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "cover.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, productdb.CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: root, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, created.ID, source.ID, productdb.CreateItemInput{
		RelativePath: "cover.jpg", MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable,
		ProcessingState: gallery.ProcessingPending}, now)
	if err != nil {
		t.Fatal(err)
	}
	profile, revision := mediaprocessing.DefaultProfileHash(), item.ContentRevision
	if _, err := db.ProcessingJobs().Enqueue(ctx, productdb.EnqueueJobInput{
		Key:  productdb.ItemDerivativeJobKey(item.UUID, mediaprocessing.VariantCard480, revision, profile),
		Kind: mediaprocessing.JobItemDerivative, GalleryID: &created.ID, ItemUUID: item.UUID,
		Variant: mediaprocessing.VariantCard480, ContentRevision: &revision, ProfileHash: profile,
		Payload: map[string]any{"cache_tier": mediaprocessing.CacheBase}, Priority: 100}, now); err != nil {
		t.Fatal(err)
	}
	worker := Worker{Database: db, Materializer: mediaaccess.Materializer{TemporaryRoot: t.TempDir()},
		Cache: mediaprocessing.CacheWriter{Root: t.TempDir()}, Generators: []mediaprocessing.Generator{mediaprocessing.ImageGenerator{}}}
	if _, err := worker.RunOne(ctx, "cover-test", time.Minute, now); err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cover_image_signatures WHERE item_uuid=? AND content_revision=?`,
			item.UUID, revision).Scan(&count); err != nil || count != 1 {
			t.Fatalf("signatures=%d err=%v", count, err)
		}
	}
	check()
	if _, err := db.ExecContext(ctx, `DELETE FROM cover_image_signatures WHERE item_uuid=?`, item.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.RunOne(ctx, "cover-test", time.Minute, now.Add(time.Minute)); !errors.Is(err, productdb.ErrJobNotClaimable) {
		t.Fatalf("idle backfill result: %v", err)
	}
	check()
}
