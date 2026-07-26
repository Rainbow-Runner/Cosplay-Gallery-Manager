package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
)

func TestCoserDetailIsUnifiedAcrossScopesAndPreservesSocialOrder(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alice"},
		ProfileSummary: "summary", Biography: "biography", CountryOrRegion: "CN"}, now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.CoreEntities().AddSocialAccount(ctx, coser.UUID, "twitter", "First", "alice", "https://example.test/one", "INACTIVE", true, 100, coser.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	coser.MetadataRevision++
	if _, err := db.CoreEntities().AddSocialAccount(ctx, coser.UUID, "twitter", "Second", "alice2", "https://example.test/two", "ACTIVE", true, 200, coser.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	listGallery, _ := createBrowseGallery(t, db, "List", gallery.ContentRatingNonAdult, now)
	if _, err := db.Galleries().AddCredit(ctx, listGallery.ID, coser.UUID, 1024, listGallery.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	activateBrowseFixture(t, db, listGallery.ID, now)
	magicGallery, _ := createBrowseGallery(t, db, "Magic", gallery.ContentRatingAdult, now.Add(time.Minute))
	if _, err := db.Galleries().AddCredit(ctx, magicGallery.ID, coser.UUID, 1024, magicGallery.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	activateBrowseFixture(t, db, magicGallery.ID, now.Add(time.Minute))

	detail, err := db.Browse().CoserDetail(ctx, coser.UUID, browse.ScopeAll, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Redirected || detail.Entity.Slug != coser.Slug || detail.Galleries.TotalItems != 2 ||
		len(detail.SocialAccounts) != 2 || detail.SocialAccounts[0].UUID != first.UUID || detail.SocialAccounts[0].Status != "INACTIVE" {
		t.Fatalf("Coser detail = %#v", detail)
	}
}
