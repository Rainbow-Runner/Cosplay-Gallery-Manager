package productdb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/portableid"
)

func TestGalleryPermanentDeleteRequiresArchiveAndPreservesSourceFiles(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 26, 15, 0, 0, 0, time.UTC)
	sourceRoot := t.TempDir()
	mediaPath := filepath.Join(sourceRoot, "photo.jpg")
	manifestPath := filepath.Join(sourceRoot, ".cosplay.json")
	if err := os.WriteFile(mediaPath, []byte("source-media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}

	created, err := db.Galleries().Create(ctx, CreateGalleryInput{
		Title: "Delete fixture", ContentRating: gallery.ContentRatingNonAdult,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "photo.jpg", MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable,
		ProcessingState: gallery.ProcessingPending,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	link, err := db.Galleries().AddExternalLink(ctx, created.ID, gallery.ExternalLinkSource, "Source", "https://example.test/source", 1024, created.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	revision := created.MetadataRevision + 1
	contentRevision := int64(1)
	if _, err := db.ProcessingJobs().Enqueue(ctx, EnqueueJobInput{
		Key: "delete-fixture", Kind: mediaprocessing.JobItemDerivative, GalleryID: &created.ID,
		ItemUUID: item.UUID, Variant: "card", ContentRevision: &contentRevision, ProfileHash: "profile",
	}, now); err != nil {
		t.Fatal(err)
	}

	preview, err := db.Galleries().PreviewDelete(ctx, created.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanDelete() || preview.ItemCount != 1 || preview.ExternalLinkCount != 1 || preview.ExecutableJobCount != 1 || !preview.HasSource {
		t.Fatalf("draft preview = %#v", preview)
	}
	if err := db.Galleries().Delete(ctx, created.SetID, revision, now); !errors.Is(err, ErrGalleryDeleteRequiresArchived) {
		t.Fatalf("draft delete error = %v", err)
	}

	archived, err := db.Galleries().SetState(ctx, created.ID, revision, gallery.StateArchived, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Galleries().Delete(ctx, created.SetID, archived.MetadataRevision-1, now.Add(2*time.Minute)); !errors.Is(err, ErrMetadataRevisionConflict) {
		t.Fatalf("stale delete error = %v", err)
	}
	if err := db.Galleries().Delete(ctx, created.SetID, archived.MetadataRevision, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	for _, check := range []struct {
		uuid string
		kind portableid.Kind
	}{{created.SetID, portableid.KindGallery}, {item.UUID, portableid.KindGalleryItem}, {link.UUID, portableid.KindExternalLink}} {
		record, err := db.UUIDRegistry().Lookup(ctx, check.uuid)
		if err != nil {
			t.Fatal(err)
		}
		if record.Kind != check.kind || record.State != PortableUUIDTombstone {
			t.Fatalf("UUID record for %s = %#v", check.uuid, record)
		}
	}
	var galleryCount, itemCount, ignoredCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM galleries WHERE set_id=?`, created.SetID).Scan(&galleryCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_items WHERE item_uuid=?`, item.UUID).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ignored_gallery_sources WHERE set_id=? AND source_path=? AND reason='GALLERY_DELETED'`, created.SetID, sourceRoot).Scan(&ignoredCount); err != nil {
		t.Fatal(err)
	}
	if galleryCount != 0 || itemCount != 0 || ignoredCount != 1 {
		t.Fatalf("deleted rows gallery=%d item=%d ignored=%d", galleryCount, itemCount, ignoredCount)
	}
	job, err := db.ProcessingJobs().FindByKey(ctx, "delete-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != mediaprocessing.JobCancelled || job.GalleryID != nil || job.ItemUUID != "" {
		t.Fatalf("retained job = %#v", job)
	}
	for path, want := range map[string]string{mediaPath: "source-media", manifestPath: `{"schema_version":1}`} {
		value, err := os.ReadFile(path)
		if err != nil || string(value) != want {
			t.Fatalf("preserved source %q = %q, error = %v", path, value, err)
		}
	}
}

func TestGalleryPermanentDeleteAllowsArchivedGalleryWithoutSource(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 26, 16, 0, 0, 0, time.UTC)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "No source"}, now)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := db.Galleries().SetState(ctx, created.ID, created.MetadataRevision, gallery.StateArchived, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Galleries().Delete(ctx, created.SetID, archived.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	var ignored int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ignored_gallery_sources WHERE set_id=?`, created.SetID).Scan(&ignored); err != nil {
		t.Fatal(err)
	}
	if ignored != 0 {
		t.Fatalf("source-free Gallery created %d ignored paths", ignored)
	}
}
