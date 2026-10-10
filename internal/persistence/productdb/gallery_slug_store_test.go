package productdb

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGallerySlugIsStableAndHistoricalRouteRedirects(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 17, 0, 0, 0, time.UTC)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "可读作品集"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if created.Slug == "" {
		t.Fatal("Gallery did not receive a stable Slug")
	}
	updated, err := db.Galleries().UpdateMetadata(ctx, created.ID, created.MetadataRevision, UpdateGalleryMetadataInput{Title: "Renamed"}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Slug != created.Slug {
		t.Fatalf("ordinary title update changed Slug from %q to %q", created.Slug, updated.Slug)
	}
	changed, err := db.Galleries().ChangeSlug(ctx, created.SetID, updated.MetadataRevision, "custom-gallery", now.Add(2*time.Minute))
	if err != nil || changed != "custom-gallery" {
		t.Fatalf("changed Slug = %q, %v", changed, err)
	}
	resolved, redirected, err := db.Galleries().ResolveSlug(ctx, created.Slug)
	if err != nil || !redirected || resolved.SetID != created.SetID || resolved.Slug != changed {
		t.Fatalf("historical resolution = %#v, %v, %v", resolved, redirected, err)
	}
	current, redirected, err := db.Galleries().ResolveSlug(ctx, changed)
	if err != nil || redirected || current.SetID != created.SetID {
		t.Fatalf("current resolution = %#v, %v, %v", current, redirected, err)
	}
	if _, err := db.Galleries().ChangeSlug(ctx, created.SetID, updated.MetadataRevision, "stale", now); !errors.Is(err, ErrMetadataRevisionConflict) {
		t.Fatalf("stale revision error = %v", err)
	}
}
