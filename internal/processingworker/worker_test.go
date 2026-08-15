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

type fakeVideoGenerator struct {
	observed *mediaprocessing.VideoPlaybackPlan
}

func (generator fakeVideoGenerator) Supports(request mediaprocessing.GenerateRequest) bool {
	return request.Variant == mediaprocessing.VariantVideoPlayback && request.VideoPlan != nil
}

type fakeVideoProbe struct {
	result mediaprocessing.VideoTechnicalMetadata
}

func (probe fakeVideoProbe) Probe(context.Context, string) (mediaprocessing.VideoTechnicalMetadata, error) {
	return probe.result, nil
}

func (generator fakeVideoGenerator) Generate(_ context.Context, request mediaprocessing.GenerateRequest) (mediaprocessing.GenerateResult, error) {
	*generator.observed = *request.VideoPlan
	if err := os.WriteFile(request.DestinationPath, []byte("generated mp4 proxy"), 0o600); err != nil {
		return mediaprocessing.GenerateResult{}, err
	}
	return mediaprocessing.GenerateResult{MIMEType: "video/mp4", Width: 1920, Height: 1080}, nil
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

func TestWorkerBuildsPersistedVideoPlanAndPublishesEnhancedProxyWithoutChangingSource(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 18, 0, 0, 0, time.UTC)
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	galleryRecord, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Video worker"}, now)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := t.TempDir()
	sourceBytes := []byte("read-only source video bytes")
	sourcePath := filepath.Join(sourceRoot, "clip.mkv")
	if err := os.WriteFile(sourcePath, sourceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, galleryRecord.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, galleryRecord.ID, source.ID, productdb.CreateItemInput{RelativePath: "clip.mkv", MediaKind: gallery.MediaKindVideo,
		ContentFormat: gallery.ContentFormatVideo, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady}, now)
	if err != nil {
		t.Fatal(err)
	}
	probeProfile := mediaprocessing.VideoProbeProfileHash("6.1")
	if err := db.VideoMetadata().MarkPending(ctx, item.UUID, item.ContentRevision, probeProfile); err != nil {
		t.Fatal(err)
	}
	if err := db.VideoMetadata().PublishReady(ctx, mediaprocessing.VideoTechnicalMetadata{ItemUUID: item.UUID, ContentRevision: item.ContentRevision,
		ProbeProfileHash: probeProfile, Container: "matroska", VideoStreamIndex: 0, VideoCodec: "h264", DisplayWidth: 1920, DisplayHeight: 1080}, now); err != nil {
		t.Fatal(err)
	}
	profile := "profile-blake3-v1:video-worker"
	revision := item.ContentRevision
	key := productdb.ItemDerivativeJobKey(item.UUID, mediaprocessing.VariantVideoPlayback, revision, profile)
	if _, err := db.ProcessingJobs().Enqueue(ctx, productdb.EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemDerivative, GalleryID: &galleryRecord.ID,
		ItemUUID: item.UUID, Variant: mediaprocessing.VariantVideoPlayback, ContentRevision: &revision, ProfileHash: profile,
		Payload: map[string]any{"cache_tier": mediaprocessing.CacheEnhanced, "playback_mode": mediaprocessing.PlaybackRemux}, Priority: 600}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE runtime_settings SET enhanced_cache_maximum_bytes=?,minimum_free_bytes=1,minimum_free_percent=0.000001 WHERE id=1`, int64(1<<40)); err != nil {
		t.Fatal(err)
	}
	cacheRoot := t.TempDir()
	observed := mediaprocessing.VideoPlaybackPlan{}
	worker := Worker{Database: db, Materializer: mediaaccess.Materializer{TemporaryRoot: t.TempDir()}, Cache: mediaprocessing.CacheWriter{Root: cacheRoot}, Generators: []mediaprocessing.Generator{fakeVideoGenerator{observed: &observed}}}
	if _, err := worker.RunOne(ctx, "video-worker", time.Minute, now); err != nil {
		t.Fatal(err)
	}
	if observed.Mode != mediaprocessing.PlaybackRemux || !observed.CopyVideo || observed.SelectAudioTrack != -1 {
		t.Fatalf("worker playback plan = %#v", observed)
	}
	derivative, err := db.Derivatives().Current(ctx, item.UUID, mediaprocessing.VariantVideoPlayback, now.Add(time.Minute))
	if err != nil || derivative.CacheTier != mediaprocessing.CacheEnhanced || derivative.MIMEType != "video/mp4" {
		t.Fatalf("video derivative = %#v, %v", derivative, err)
	}
	after, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	afterBytes, err := os.ReadFile(sourcePath)
	if err != nil || string(afterBytes) != string(sourceBytes) || !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
		t.Fatalf("source changed: before=%#v after=%#v err=%v", before, after, err)
	}
}

func TestWorkerPublishesVideoTechnicalMetadataBeforeQueuingPoster(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 19, 0, 0, 0, time.UTC)
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	galleryRecord, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Video probe worker"}, now)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "clip.mp4"), []byte("probe source"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, galleryRecord.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, galleryRecord.ID, source.ID, productdb.CreateItemInput{RelativePath: "clip.mp4", MediaKind: gallery.MediaKindVideo,
		ContentFormat: gallery.ContentFormatVideo, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingPending}, now)
	if err != nil {
		t.Fatal(err)
	}
	probeProfile := mediaprocessing.VideoProbeProfileHash("6.1")
	posterProfile := mediaprocessing.VideoPosterProfileHash("6.1")
	initialProfile := mediaprocessing.DefaultProfileHash()
	if err := db.VideoMetadata().MarkPending(ctx, item.UUID, item.ContentRevision, initialProfile); err != nil {
		t.Fatal(err)
	}
	revision := item.ContentRevision
	key := productdb.ItemTechnicalMetadataJobKey(item.UUID, revision, initialProfile)
	if _, err := db.ProcessingJobs().Enqueue(ctx, productdb.EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemTechnicalMetadata,
		GalleryID: &galleryRecord.ID, ItemUUID: item.UUID, ContentRevision: &revision, ProfileHash: initialProfile, Payload: map[string]any{}, Priority: 500}, now); err != nil {
		t.Fatal(err)
	}
	worker := Worker{Database: db, Materializer: mediaaccess.Materializer{TemporaryRoot: t.TempDir()}, Cache: mediaprocessing.CacheWriter{Root: t.TempDir()},
		VideoProbe:       fakeVideoProbe{result: mediaprocessing.VideoTechnicalMetadata{Container: "mp4", DurationSeconds: 20, VideoStreamIndex: 2, VideoCodec: "h264", DisplayWidth: 1280, DisplayHeight: 720}},
		ProbeProfileHash: probeProfile, PosterProfileHash: posterProfile}
	if _, err := worker.RunOne(ctx, "probe-worker", time.Minute, now); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.RunOne(ctx, "probe-worker", time.Minute, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	metadata, err := db.VideoMetadata().Find(ctx, item.UUID)
	if err != nil || metadata.ProbeState != mediaprocessing.VideoProbeReady || metadata.DurationSeconds != 20 || metadata.VideoStreamIndex != 2 {
		t.Fatalf("technical metadata = %#v, %v", metadata, err)
	}
	poster, err := db.ProcessingJobs().FindByKey(ctx, productdb.ItemDerivativeJobKey(item.UUID, mediaprocessing.VariantStaticPoster, revision, posterProfile))
	if err != nil || poster.Status != mediaprocessing.JobPending {
		t.Fatalf("poster job = %#v, %v", poster, err)
	}
}
