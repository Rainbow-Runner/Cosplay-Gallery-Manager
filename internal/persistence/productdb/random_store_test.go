package productdb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestRandomItemsUseSoftQuotaSafeResourcesAndMediaFilter(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 21, 0, 0, 0, time.UTC)
	created, source := createBrowseGallery(t, db, "Random Gallery Title Must Not Be A Media Field", gallery.ContentRatingNonAdult, now)
	profile := mediaprocessing.DefaultProfileHash()
	position := int64(1024)
	add := func(kind gallery.MediaKind, category gallery.ImageCategory, index int) {
		extension := "jpg"
		variant := mediaprocessing.VariantCard480
		if kind == gallery.MediaKindAnimatedImage {
			extension = "gif"
			variant = mediaprocessing.VariantStaticPoster
		} else if kind == gallery.MediaKindVideo {
			extension = "mp4"
			variant = mediaprocessing.VariantStaticPoster
		}
		item := addBrowseItem(t, db, created.ID, source.ID, fmt.Sprintf("%s-%02d.%s", kind, index, extension), kind,
			category, gallery.AvailabilityAvailable, gallery.ProcessingReady, position, now)
		position += 1024
		if _, err := db.Derivatives().Publish(ctx, PublishDerivativeInput{ItemUUID: item.UUID, Variant: variant,
			CacheTier: mediaprocessing.CacheBase, ContentRevision: item.ContentRevision, ProfileHash: profile,
			CacheRelativePath: fmt.Sprintf("random/%s.jpg", item.UUID), MIMEType: "image/jpeg", ByteSize: 10}, now); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 17; index++ {
		category := gallery.ImageCategoryPhoto
		if index%5 == 0 {
			category = gallery.ImageCategorySelfie
		}
		add(gallery.MediaKindStaticImage, category, index)
	}
	for index := 0; index < 2; index++ {
		add(gallery.MediaKindAnimatedImage, "", index)
	}
	for index := 0; index < 5; index++ {
		add(gallery.MediaKindVideo, "", index)
	}
	activateBrowseFixture(t, db, created.ID, now)

	items, err := db.Browse().RandomItems(ctx, browse.ScopeList, browse.RandomAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 24 {
		t.Fatalf("random item count = %d", len(items))
	}
	counts := map[gallery.MediaKind]int{}
	seen := map[string]bool{}
	for _, item := range items {
		counts[item.MediaKind]++
		if seen[item.ItemUUID] || item.GallerySetID != created.SetID || item.GallerySlug == "" ||
			item.Resource.ItemUUID != item.ItemUUID || item.Resource.Variant == "" || item.Resource.ProfileHash == "" {
			t.Fatalf("random Item DTO = %#v", item)
		}
		seen[item.ItemUUID] = true
		if len(item.Characters) != 0 {
			t.Fatalf("Album random card acquired Character row content: %#v", item.Characters)
		}
	}
	if counts[gallery.MediaKindStaticImage] != 17 || counts[gallery.MediaKindAnimatedImage] != 2 || counts[gallery.MediaKindVideo] != 5 {
		t.Fatalf("random quota counts = %#v", counts)
	}
	videos, err := db.Browse().RandomItems(ctx, browse.ScopeList, browse.RandomVideo)
	if err != nil || len(videos) != 5 {
		t.Fatalf("video filter = %d, %v", len(videos), err)
	}
	for _, item := range videos {
		if item.MediaKind != gallery.MediaKindVideo {
			t.Fatalf("video filter returned %#v", item)
		}
	}
}

func TestSoftQuotaLargestRemainderMatchesDefaultSeventyTenTwenty(t *testing.T) {
	result := softQuotaCounts(24, 0.70, 0.10, 0.20)
	if len(result) != 3 || result[0] != 17 || result[1] != 2 || result[2] != 5 {
		t.Fatalf("soft quotas = %#v", result)
	}
}
