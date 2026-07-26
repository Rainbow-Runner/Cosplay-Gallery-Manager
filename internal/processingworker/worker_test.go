package processingworker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

type fakeGenerator struct{}

func (fakeGenerator) Supports(mediaprocessing.GenerateRequest) bool { return true }
func (fakeGenerator) Generate(_ context.Context, request mediaprocessing.GenerateRequest) (mediaprocessing.GenerateResult, error) {
	if err := os.WriteFile(request.DestinationPath, []byte("generated jpeg"), 0o600); err != nil {
		return mediaprocessing.GenerateResult{}, err
	}
	return mediaprocessing.GenerateResult{MIMEType: "image/jpeg", Width: 480, Height: 640}, nil
}

func TestWorkerClaimsMaterializesPublishesAndCompletesDerivative(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Round(0)
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	galleryRecord, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Worker"}, now)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "photo.jpg"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, galleryRecord.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, galleryRecord.ID, source.ID, productdb.CreateItemInput{RelativePath: "photo.jpg", MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage, ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingPending}, now)
	if err != nil {
		t.Fatal(err)
	}
	revision := item.ContentRevision
	profile := mediaprocessing.DefaultProfileHash()
	key := productdb.ItemDerivativeJobKey(item.UUID, mediaprocessing.VariantCard480, revision, profile)
	job, err := db.ProcessingJobs().Enqueue(ctx, productdb.EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemDerivative, GalleryID: &galleryRecord.ID, ItemUUID: item.UUID, Variant: mediaprocessing.VariantCard480, ContentRevision: &revision, ProfileHash: profile, Payload: map[string]any{"cache_tier": mediaprocessing.CacheBase}, Priority: 100}, now)
	if err != nil {
		t.Fatal(err)
	}
	cacheRoot := t.TempDir()
	worker := Worker{Database: db, Materializer: mediaaccess.Materializer{TemporaryRoot: t.TempDir()}, Cache: mediaprocessing.CacheWriter{Root: cacheRoot}, Generators: []mediaprocessing.Generator{fakeGenerator{}}}
	completed, err := worker.RunOne(ctx, "worker", time.Minute, now)
	if err != nil || completed.ID != job.ID {
		t.Fatalf("worker result = %#v, %v", completed, err)
	}
	stored, err := db.ProcessingJobs().FindByKey(ctx, key)
	if err != nil || stored.Status != mediaprocessing.JobCompleted {
		t.Fatalf("stored job = %#v, %v", stored, err)
	}
	derivative, err := db.Derivatives().Current(ctx, item.UUID, mediaprocessing.VariantCard480, time.Now())
	if err != nil || derivative.MIMEType != "image/jpeg" || derivative.CacheTier != mediaprocessing.CacheBase {
		t.Fatalf("published derivative = %#v, %v", derivative, err)
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, filepath.FromSlash(derivative.CacheRelativePath))); err != nil {
		t.Fatal(err)
	}
	cover, err := db.Covers().Find(ctx, galleryRecord.ID)
	if err != nil || cover.PreferredKind != gallery.CoverAutoRandom || cover.EffectiveItemUUID != item.UUID {
		t.Fatalf("initialized cover = %#v, %v", cover, err)
	}
}
