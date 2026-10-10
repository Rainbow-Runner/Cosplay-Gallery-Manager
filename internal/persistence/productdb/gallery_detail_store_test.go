package productdb

import (
	"context"
	"path/filepath"
	"reflect"
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
	excluded := addBrowseItem(t, db, created.ID, source.ID, "Disc 10/excluded.jpg", gallery.MediaKindStaticImage, gallery.ImageCategoryPhoto,
		gallery.AvailabilityAvailable, gallery.ProcessingPending, 1536, now)
	addBrowseItem(t, db, created.ID, source.ID, "Disc 2/unreadable.jpg", gallery.MediaKindStaticImage, gallery.ImageCategoryPhoto,
		gallery.AvailabilityUnreadable, gallery.ProcessingPending, 1792, now)
	missing := addBrowseItem(t, db, created.ID, source.ID, "missing.jpg", gallery.MediaKindStaticImage, gallery.ImageCategoryPhoto,
		gallery.AvailabilityMissing, gallery.ProcessingPending, 2048, now)
	missingNested := addBrowseItem(t, db, created.ID, source.ID, "Missing only/file.jpg", gallery.MediaKindStaticImage, gallery.ImageCategoryPhoto,
		gallery.AvailabilityMissing, gallery.ProcessingPending, 2560, now)
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET excluded=1 WHERE id=?`, excluded.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET byte_size=100 WHERE id=?`, available.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET byte_size=999 WHERE id=?`, missing.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET byte_size=999 WHERE id=?`, missingNested.ID); err != nil {
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
	wantDirectories := []string{source.Path, filepath.Join(source.Path, "Disc 2"), filepath.Join(source.Path, "Disc 10")}
	if !reflect.DeepEqual(detail.MediaParentDirectories, wantDirectories) {
		t.Fatalf("media parent directories = %#v, want %#v", detail.MediaParentDirectories, wantDirectories)
	}
}

func TestGalleryDetailUsesArchiveContainingDirectory(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Archive", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "Archive.cbz")
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{Type: gallery.SourceTypeArchive, Path: archivePath, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	addBrowseItem(t, db, created.ID, source.ID, "inside/001.jpg", gallery.MediaKindStaticImage, gallery.ImageCategoryPhoto,
		gallery.AvailabilityAvailable, gallery.ProcessingPending, 1024, now)
	activateBrowseFixture(t, db, created.ID, now)
	detail, err := db.Browse().GalleryDetailBySlug(ctx, browse.ScopeList, created.Slug)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Dir(archivePath)}
	if !reflect.DeepEqual(detail.MediaParentDirectories, want) {
		t.Fatalf("archive directories = %#v, want %#v", detail.MediaParentDirectories, want)
	}
}
