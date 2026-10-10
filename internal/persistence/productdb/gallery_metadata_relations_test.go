package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestGalleryAliasesMarkdownTagsAndExternalLinks(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 23, 0, 0, 0, time.UTC)
	store := db.Galleries()
	created, err := store.Create(ctx, CreateGalleryInput{
		Title: "Metadata", Aliases: []string{"First alias"},
		Description: "## Notes\n\n[Source](https://example.test/source)",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Aliases) != 1 || created.Aliases[0] != "First alias" {
		t.Fatalf("Gallery aliases = %#v", created.Aliases)
	}
	if _, err := store.UpdateMetadata(ctx, created.ID, created.MetadataRevision, UpdateGalleryMetadataInput{
		Title: "Unsafe", Description: "<script>alert(1)</script>",
	}, now); err == nil {
		t.Fatal("Gallery accepted raw HTML description")
	}

	tag, err := db.CoreEntities().CreateTag(ctx, CreateTagInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "Portrait"},
		UseInRecommendation:    true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTag(ctx, created.ID, tag.UUID, 1024, created.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	current, _ := store.Find(ctx, created.ID)
	link, err := store.AddExternalLink(ctx, created.ID, gallery.ExternalLinkSource,
		"Original", "https://example.test/gallery", 1024, current.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	if link.UUID == "" || link.URL != "https://example.test/gallery" {
		t.Fatalf("ExternalLink = %#v", link)
	}
	current, _ = store.Find(ctx, created.ID)
	if _, err := store.AddExternalLink(ctx, created.ID, gallery.ExternalLinkReference,
		"Local", "file:///tmp/media", 2048, current.MetadataRevision, now); err == nil {
		t.Fatal("ExternalLink accepted non-HTTP URL")
	}
}

func TestCoserSocialAccountsPreservePositionAcrossStatus(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 23, 0, 0, 0, time.UTC)
	store := db.CoreEntities()
	coser, err := store.CreateCoser(ctx, CreateCoserInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "Coser"},
		Biography:              "## Profile\n\nText",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AddSocialAccount(ctx, coser.UUID, "twitter", "Primary", "one",
		"https://example.test/one", "INACTIVE", true, 1024, coser.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AddSocialAccount(ctx, coser.UUID, "twitter", "Secondary", "two",
		"https://example.test/two", "ACTIVE", true, 2048, coser.MetadataRevision+1, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.PlatformKey != second.PlatformKey || first.Position != 1024 || second.Position != 2048 {
		t.Fatalf("same-platform SocialAccounts = %#v %#v", first, second)
	}
	var firstPosition, secondPosition int64
	if err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT position FROM coser_social_accounts WHERE account_uuid = ?),
			(SELECT position FROM coser_social_accounts WHERE account_uuid = ?)
	`, first.UUID, second.UUID).Scan(&firstPosition, &secondPosition); err != nil {
		t.Fatal(err)
	}
	if firstPosition >= secondPosition {
		t.Fatal("INACTIVE status changed SocialAccount ordering")
	}
}

func TestGalleryItemManualSelfieAndCaptionAreGalleryMetadata(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 23, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	item, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "photo.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := db.Galleries().Find(ctx, created.ID)
	updated, err := db.Galleries().UpdateItemMetadata(ctx, item.ID, gallery.ImageCategorySelfie,
		"Mirror selfie", current.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ImageCategory != gallery.ImageCategorySelfie || updated.Caption != "Mirror selfie" {
		t.Fatalf("updated Item metadata = %#v", updated)
	}
	currentAfter, _ := db.Galleries().Find(ctx, created.ID)
	if currentAfter.MetadataRevision != current.MetadataRevision+1 {
		t.Fatal("GalleryItem metadata did not increment Gallery metadata_revision")
	}
}
