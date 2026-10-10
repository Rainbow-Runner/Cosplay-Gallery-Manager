package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestDerivativeProfileSwitchIsAtomicAndContentChangesHardInvalidate(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC)
	_, items := createDerivativeTestGallery(t, db, now)
	store := db.Derivatives()

	first, err := store.Publish(ctx, PublishDerivativeInput{ItemUUID: items[0].UUID, Variant: mediaprocessing.VariantCard960,
		CacheTier: mediaprocessing.CacheEnhanced, ContentRevision: 1, ProfileHash: "profile-a",
		CacheRelativePath: "items/a/card-480.webp", MIMEType: "image/webp", ByteSize: 100, Width: 480, Height: 640}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkProfileStale(ctx, items[0].UUID, mediaprocessing.VariantCard960, "profile-b"); err != nil {
		t.Fatal(err)
	}
	stale, err := store.Current(ctx, items[0].UUID, mediaprocessing.VariantCard960, now.Add(time.Minute))
	if err != nil || stale.ID != first.ID || stale.State != mediaprocessing.DerivativeStale {
		t.Fatalf("old profile did not remain serviceable: %#v, %v", stale, err)
	}
	replacement, err := store.Publish(ctx, PublishDerivativeInput{ItemUUID: items[0].UUID, Variant: mediaprocessing.VariantCard960,
		CacheTier: mediaprocessing.CacheEnhanced, ContentRevision: 1, ProfileHash: "profile-b",
		CacheRelativePath: "items/a/card-480-v2.webp", MIMEType: "image/webp", ByteSize: 110, Width: 480, Height: 640}, now.Add(2*time.Minute))
	if err != nil || replacement.ID == first.ID || !replacement.Current {
		t.Fatalf("replacement derivative = %#v, %v", replacement, err)
	}
	var oldCurrent int
	if err := db.QueryRowContext(ctx, `SELECT is_current FROM media_derivatives WHERE id=?`, first.ID).Scan(&oldCurrent); err != nil || oldCurrent != 0 {
		t.Fatalf("old derivative current = %d, %v", oldCurrent, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET content_revision=2 WHERE item_uuid=?`, items[0].UUID); err != nil {
		t.Fatal(err)
	}
	if err := store.InvalidateContent(ctx, items[0].UUID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Current(ctx, items[0].UUID, mediaprocessing.VariantCard960, now); err == nil {
		t.Fatal("old-content derivative remained current")
	}
	if _, err := store.Publish(ctx, PublishDerivativeInput{ItemUUID: items[0].UUID, Variant: mediaprocessing.VariantCard960,
		CacheTier: mediaprocessing.CacheEnhanced, ContentRevision: 1, ProfileHash: "profile-c",
		CacheRelativePath: "items/a/stale.webp", MIMEType: "image/webp"}, now); err == nil {
		t.Fatal("stale content revision was published")
	}
}

func TestGalleryScrubberIndexUsesMixedStaticPosterOrderAndSafeMembersOnly(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC)
	galleryID, items := createDerivativeTestGallery(t, db, now)
	store := db.Derivatives()
	for index, item := range items[:4] {
		variant := mediaprocessing.VariantCard480
		if item.MediaKind != gallery.MediaKindStaticImage {
			variant = mediaprocessing.VariantStaticPoster
		}
		if _, err := store.Publish(ctx, PublishDerivativeInput{ItemUUID: item.UUID, Variant: variant,
			CacheTier: mediaprocessing.CacheBase, ContentRevision: item.ContentRevision, ProfileHash: "profile",
			CacheRelativePath: "scrubber/item-" + string(rune('a'+index)) + ".webp", MIMEType: "image/webp", ByteSize: 10}, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Galleries().SetItemExcluded(ctx, items[4].ID, true, 1, now); err != nil {
		t.Fatal(err)
	}
	index, err := store.GalleryScrubberIndex(ctx, galleryID)
	if err != nil {
		t.Fatal(err)
	}
	if len(index) != 4 {
		t.Fatalf("Scrubber index = %#v", index)
	}
	want := []gallery.ImageCategory{gallery.ImageCategoryPhoto, gallery.ImageCategorySelfie, "", ""}
	for ordinal := range index {
		if index[ordinal].Ordinal != ordinal || index[ordinal].ItemUUID != items[ordinal].UUID || index[ordinal].ImageCategory != want[ordinal] {
			t.Fatalf("Scrubber[%d] = %#v", ordinal, index[ordinal])
		}
		if ordinal >= 2 && index[ordinal].Variant != mediaprocessing.VariantStaticPoster {
			t.Fatalf("animated/video Scrubber resource is not a Poster: %#v", index[ordinal])
		}
	}
	if err := store.ForgetGenerated(ctx, indexDerivativeID(t, db, items[2].UUID, mediaprocessing.VariantStaticPoster)); err == nil {
		t.Fatal("BASE Poster was accepted by LRU deletion")
	}
}

func createDerivativeTestGallery(t *testing.T, db *Database, now time.Time) (int64, []gallery.Item) {
	t.Helper()
	ctx := context.Background()
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Derivatives"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: t.TempDir(), Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	inputs := []CreateItemInput{
		{RelativePath: "photo.jpg", MediaKind: gallery.MediaKindStaticImage, ImageCategory: gallery.ImageCategoryPhoto, Position: 1024},
		{RelativePath: "selfie.jpg", MediaKind: gallery.MediaKindStaticImage, ImageCategory: gallery.ImageCategorySelfie, Position: 2048},
		{RelativePath: "animation.gif", MediaKind: gallery.MediaKindAnimatedImage, Position: 3072},
		{RelativePath: "video.mp4", MediaKind: gallery.MediaKindVideo, Position: 4096},
		{RelativePath: "excluded.jpg", MediaKind: gallery.MediaKindStaticImage, ImageCategory: gallery.ImageCategoryPhoto, Position: 5120},
	}
	items := make([]gallery.Item, 0, len(inputs))
	for _, input := range inputs {
		input.Availability, input.ProcessingState = gallery.AvailabilityAvailable, gallery.ProcessingReady
		item, err := db.Galleries().AddItem(ctx, created.ID, source.ID, input, now)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, item)
	}
	return created.ID, items
}

func indexDerivativeID(t *testing.T, db *Database, itemUUID, variant string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM media_derivatives WHERE item_uuid=? AND variant=? AND is_current=1`, itemUUID, variant).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
