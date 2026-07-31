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

func TestCoserIndexSeparatesCosplayAndAlbumWithoutSplittingIdentity(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	cosplayOnly, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Cosplay Only"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	albumOnly, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Album Only"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	both, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Both"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	work, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "Character"}, now)
	if err != nil {
		t.Fatal(err)
	}
	for index, person := range []struct {
		coserUUID string
		cosplay   bool
	}{
		{cosplayOnly.UUID, true},
		{albumOnly.UUID, false},
		{both.UUID, true},
		{both.UUID, false},
	} {
		galleryRecord, _ := createBrowseGallery(t, db, person.coserUUID, gallery.ContentRatingNonAdult, now.Add(time.Duration(index)*time.Minute))
		creditID, err := db.Galleries().AddCredit(ctx, galleryRecord.ID, person.coserUUID, 1024, galleryRecord.MetadataRevision, now)
		if err != nil {
			t.Fatal(err)
		}
		if person.cosplay {
			if err := db.Galleries().AddCast(ctx, galleryRecord.ID, creditID, character.UUID, 1024, galleryRecord.MetadataRevision+1, now); err != nil {
				t.Fatal(err)
			}
		}
		activateBrowseFixture(t, db, galleryRecord.ID, now.Add(time.Duration(index)*time.Minute))
	}
	cosers, err := db.Browse().EntityIndexByCollection(ctx, browse.SearchCoser, browse.ScopeAll, 1, browse.EntitySortName, browse.CollectionCosplay)
	if err != nil || cosers.TotalItems != 2 || cosers.Items[0].UUID != both.UUID || cosers.Items[1].UUID != cosplayOnly.UUID {
		t.Fatalf("COSPLAY people = %#v, %v", cosers, err)
	}
	models, err := db.Browse().EntityIndexByCollection(ctx, browse.SearchCoser, browse.ScopeAll, 1, browse.EntitySortName, browse.CollectionAlbum)
	if err != nil || models.TotalItems != 2 || models.Items[0].UUID != albumOnly.UUID || models.Items[1].UUID != both.UUID {
		t.Fatalf("ALBUM people = %#v, %v", models, err)
	}
	filtered, err := db.Browse().EntityIndexFiltered(ctx, browse.SearchCoser, browse.ScopeAll, 1, browse.EntitySortName, browse.CollectionAlbum, "Both")
	if err != nil || filtered.TotalItems != 1 || filtered.Items[0].UUID != both.UUID {
		t.Fatalf("filtered ALBUM people = %#v, %v", filtered, err)
	}
	recent, err := db.Browse().EntityIndexByCollection(ctx, browse.SearchCoser, browse.ScopeAll, 1, browse.EntitySortRecentlyAdded, browse.CollectionCosplay)
	if err != nil || recent.TotalItems != 2 || recent.Items[0].UUID != both.UUID {
		t.Fatalf("recent COSPLAY people = %#v, %v", recent, err)
	}
}
