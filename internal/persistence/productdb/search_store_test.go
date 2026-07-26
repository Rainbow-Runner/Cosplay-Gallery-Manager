package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
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
	listGallery, _ := createBrowseGallery(t, db, "Portrait Collection", gallery.ContentRatingNonAdult, now)
	if _, err := db.Galleries().AddCredit(ctx, listGallery.ID, coser.UUID, 1024, listGallery.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	activateBrowseFixture(t, db, listGallery.ID, now)
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
	magic, err := db.Browse().SearchPreview(ctx, browse.ScopeMagic, "Alicia")
	if err != nil || len(magic.Galleries) != 1 || magic.Galleries[0].UUID != adultGallery.SetID || len(magic.Cosers) != 0 {
		t.Fatalf("MAGIC search = %#v, %v", magic, err)
	}
	direct, err := db.Browse().SearchPreview(ctx, browse.ScopeList, "Portrait Collection")
	if err != nil || len(direct.Galleries) != 1 || direct.Galleries[0].MatchLevel != 1 {
		t.Fatalf("direct Gallery search = %#v, %v", direct.Galleries, err)
	}
}
