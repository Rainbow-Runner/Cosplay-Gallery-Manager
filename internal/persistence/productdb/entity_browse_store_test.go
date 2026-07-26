package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
)

func TestEntityIndexesUseConfirmedPageSizesScopeAndVisibleAssociations(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	visible, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Visible", Aliases: []string{"Alias"}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Standalone"}}, now); err != nil {
		t.Fatal(err)
	}
	galleryRecord, _ := createBrowseGallery(t, db, "Entity", gallery.ContentRatingNonAdult, now)
	if _, err := db.Galleries().AddCredit(ctx, galleryRecord.ID, visible.UUID, 1024, galleryRecord.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	activateBrowseFixture(t, db, galleryRecord.ID, now)
	page, err := db.Browse().EntityIndex(ctx, browse.SearchCoser, browse.ScopeList, 1, browse.EntitySortName)
	if err != nil {
		t.Fatal(err)
	}
	if page.PageSize != 30 || page.TotalItems != 1 || len(page.Items) != 1 || page.Items[0].UUID != visible.UUID ||
		len(page.Items[0].Aliases) != 1 || page.Items[0].Aliases[0] != "Alias" {
		t.Fatalf("Coser entity page = %#v", page)
	}
	magic, err := db.Browse().EntityIndex(ctx, browse.SearchCoser, browse.ScopeMagic, 1, browse.EntitySortName)
	if err != nil || magic.TotalItems != 0 || magic.PageSize != 30 {
		t.Fatalf("MAGIC Coser page = %#v, %v", magic, err)
	}
	for _, kind := range []browse.SearchEntityKind{browse.SearchWork, browse.SearchCharacter, browse.SearchTag} {
		empty, err := db.Browse().EntityIndex(ctx, kind, browse.ScopeAll, 1, browse.EntitySortName)
		if err != nil || empty.PageSize != 60 {
			t.Fatalf("%s page = %#v, %v", kind, empty, err)
		}
	}
}
