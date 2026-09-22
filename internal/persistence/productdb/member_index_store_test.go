package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestGalleryMemberIndexIsCompleteGroupedPathFreeAndKeepsPending(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 22, 0, 0, 0, time.UTC)
	created, source := createBrowseGallery(t, db, "Members", gallery.ContentRatingNonAdult, now)
	selfie := addBrowseItem(t, db, created.ID, source.ID, "selfie.jpg", gallery.MediaKindStaticImage, gallery.ImageCategorySelfie,
		gallery.AvailabilityAvailable, gallery.ProcessingPending, 1024, now)
	photo := addBrowseItem(t, db, created.ID, source.ID, "photo.jpg", gallery.MediaKindStaticImage, gallery.ImageCategoryPhoto,
		gallery.AvailabilityAvailable, gallery.ProcessingReady, 4096, now)
	animation := addBrowseItem(t, db, created.ID, source.ID, "animation.gif", gallery.MediaKindAnimatedImage, "",
		gallery.AvailabilityAvailable, gallery.ProcessingError, 2048, now)
	addBrowseItem(t, db, created.ID, source.ID, "missing.mp4", gallery.MediaKindVideo, "",
		gallery.AvailabilityMissing, gallery.ProcessingError, 3072, now)
	profile := mediaprocessing.DefaultProfileHash()
	for _, variant := range []string{mediaprocessing.VariantCard480, mediaprocessing.VariantLightbox4096} {
		tier := mediaprocessing.CacheBase
		if variant == mediaprocessing.VariantLightbox4096 {
			tier = mediaprocessing.CacheEnhanced
		}
		if _, err := db.Derivatives().Publish(ctx, PublishDerivativeInput{ItemUUID: photo.UUID, Variant: variant,
			CacheTier: tier, ContentRevision: photo.ContentRevision, ProfileHash: profile,
			CacheRelativePath: "members/" + variant + ".jpg", MIMEType: "image/jpeg", ByteSize: 10}, now); err != nil {
			t.Fatal(err)
		}
	}
	activateBrowseFixture(t, db, created.ID, now)
	index, err := db.Browse().GalleryMemberIndex(ctx, created.SetID, browse.ScopeList)
	if err != nil {
		t.Fatal(err)
	}
	if index.SetID != created.SetID || index.MetadataRevision == 0 || index.ScanRevision == 0 || len(index.Items) != 3 {
		t.Fatalf("member index = %#v", index)
	}
	if index.Items[0].ItemUUID != photo.UUID || index.Items[1].ItemUUID != selfie.UUID || index.Items[2].ItemUUID != animation.UUID {
		t.Fatalf("member group order = %#v", index.Items)
	}
	if index.Items[0].CardResource == nil || index.Items[0].LargeResource == nil || index.Items[1].CardResource != nil ||
		index.Items[1].ProcessingState != gallery.ProcessingPending || index.Items[2].CardResource != nil {
		t.Fatalf("member resource/state contract = %#v", index.Items)
	}
}
