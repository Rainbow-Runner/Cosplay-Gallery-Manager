package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestSearchUsesNamesAliasesIndirectGalleryAndCurrentScope(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 23, 0, 0, 0, time.UTC)
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alice", Aliases: []string{"Alicia"}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	standalone, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alicia Standalone"}}, now)
	if err != nil || standalone.UUID == "" {
		t.Fatal(err)
	}
	listGallery, source := createBrowseGallery(t, db, "Portrait Collection", gallery.ContentRatingNonAdult, now)
	if _, err := db.Galleries().AddCredit(ctx, listGallery.ID, coser.UUID, 1024, listGallery.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	activateBrowseFixture(t, db, listGallery.ID, now)
	photo := addBrowseItem(t, db, listGallery.ID, source.ID, "photo.jpg", gallery.MediaKindStaticImage, gallery.ImageCategoryPhoto, gallery.AvailabilityAvailable, gallery.ProcessingReady, 1024, now)
	profile := mediaprocessing.DefaultProfileHash()
	if _, err := db.Derivatives().Publish(ctx, PublishDerivativeInput{ItemUUID: photo.UUID, Variant: mediaprocessing.VariantCard480,
		CacheTier: mediaprocessing.CacheBase, ContentRevision: photo.ContentRevision, ProfileHash: profile,
		CacheRelativePath: "search/cover.webp", MIMEType: "image/webp", ByteSize: 10, Width: 480, Height: 640}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Covers().Initialize(ctx, listGallery.ID, now); err != nil {
		t.Fatal(err)
	}
	adultGallery, _ := createBrowseGallery(t, db, "Alicia Adult", gallery.ContentRatingAdult, now.Add(time.Minute))
	activateBrowseFixture(t, db, adultGallery.ID, now.Add(time.Minute))

	result, err := db.Browse().SearchPreview(ctx, browse.ScopeList, "Alicia")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Cosers) != 1 || result.Cosers[0].UUID != coser.UUID || result.Cosers[0].MatchLevel != 2 {
		t.Fatalf("Coser Alias results = %#v", result.Cosers)
	}
	if len(result.Galleries) != 1 || result.Galleries[0].UUID != listGallery.SetID || result.Galleries[0].MatchLevel != 7 {
		t.Fatalf("indirect Gallery results = %#v", result.Galleries)
	}
	if cover := result.Galleries[0].CoverResource; cover == nil || cover.ItemUUID != photo.UUID || cover.Variant != mediaprocessing.VariantCard480 {
		t.Fatalf("Gallery search cover = %#v", cover)
	}
	magic, err := db.Browse().SearchPreview(ctx, browse.ScopeMagic, "Alicia")
	if err != nil || len(magic.Galleries) != 1 || magic.Galleries[0].UUID != adultGallery.SetID || len(magic.Cosers) != 0 {
		t.Fatalf("MAGIC search = %#v, %v", magic, err)
	}
	if magic.Galleries[0].CoverResource != nil {
		t.Fatalf("unavailable cover should be absent: %#v", magic.Galleries[0].CoverResource)
	}
	direct, err := db.Browse().SearchPreview(ctx, browse.ScopeList, "Portrait Collection")
	if err != nil || len(direct.Galleries) != 1 || direct.Galleries[0].MatchLevel != 1 {
		t.Fatalf("direct Gallery search = %#v, %v", direct.Galleries, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE galleries SET shoot_date='2024-05',shoot_date_precision='MONTH',publish_date='2025-06-17',publish_date_precision='DAY',media_added_status='COMPLETE',media_added_start_at_utc='2023-01-01T00:00:00Z',media_added_end_at_utc='2023-02-01T00:00:00Z' WHERE id=?`, listGallery.ID); err != nil {
		t.Fatal(err)
	}
	dated, err := db.Browse().SearchPreview(ctx, browse.ScopeList, "Portrait Collection")
	if err != nil {
		t.Fatal(err)
	}
	hit := dated.Galleries[0]
	if hit.ShootDate != "2024-05" || hit.PublishDate != "2025-06-17" || hit.MediaAddedStartUTC != "2023-01-01T00:00:00Z" || hit.MediaAddedEndUTC != "2023-02-01T00:00:00Z" {
		t.Fatalf("search dates = %#v", hit)
	}
}
