package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
)

func TestGalleryDetailUsesUnifiedCardDirectTagsWeakLinksAndAvailableSize(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 24, 1, 0, 0, 0, time.UTC)
	created, source := createBrowseGallery(t, db, "Detail", gallery.ContentRatingNonAdult, now)
	available := addBrowseItem(t, db, created.ID, source.ID, "available.jpg", gallery.MediaKindStaticImage, gallery.ImageCategoryPhoto,
		gallery.AvailabilityAvailable, gallery.ProcessingPending, 1024, now)
	missing := addBrowseItem(t, db, created.ID, source.ID, "missing.jpg", gallery.MediaKindStaticImage, gallery.ImageCategoryPhoto,
		gallery.AvailabilityMissing, gallery.ProcessingPending, 2048, now)
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET byte_size=100 WHERE id=?`, available.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET byte_size=999 WHERE id=?`, missing.ID); err != nil {
		t.Fatal(err)
	}
	tag, err := db.CoreEntities().CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Direct"}, UseInRecommendation: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := db.Galleries().Find(ctx, created.ID)
	if err := db.Galleries().AddTag(ctx, created.ID, tag.UUID, 1024, current.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	current, _ = db.Galleries().Find(ctx, created.ID)
	if _, err := db.Galleries().AddExternalLink(ctx, created.ID, gallery.ExternalLinkSource, "Original",
		"https://example.test/source", 1024, current.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	activateBrowseFixture(t, db, created.ID, now)
	detail, err := db.Browse().GalleryDetailBySlug(ctx, browse.ScopeList, created.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Card.SetID != created.SetID || detail.AvailableBytes != 100 || len(detail.Tags) != 1 || detail.Tags[0].UUID != tag.UUID ||
		len(detail.ExternalLinks) != 1 || detail.ExternalLinks[0].URL != "https://example.test/source" {
		t.Fatalf("Gallery detail = %#v", detail)
	}
}
