package productdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestGalleryAndItemPersonalStatesAreIndependent(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC)
	created, _, item := createCompleteAlbumFixture(t, db, now)
	states := db.PersonalStates()

	if err := states.SetGalleryFavorite(ctx, created.ID, true, now); err != nil {
		t.Fatalf("favoriting Gallery: %v", err)
	}
	if err := states.SetItemFavorite(ctx, item.ID, true, now.Add(time.Minute)); err != nil {
		t.Fatalf("favoriting Item: %v", err)
	}
	rating := 9
	if err := states.SetGalleryRating(ctx, created.ID, &rating, created.MetadataRevision, now); err != nil {
		t.Fatalf("rating Gallery: %v", err)
	}
	afterGalleryRating, err := db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	itemRating := 6
	if err := states.SetItemRating(
		ctx,
		item.ID,
		&itemRating,
		afterGalleryRating.MetadataRevision,
		now.Add(time.Minute),
	); err != nil {
		t.Fatalf("rating Item: %v", err)
	}

	galleryState, err := states.Gallery(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !galleryState.Favorite || galleryState.RatingHalfSteps == nil || *galleryState.RatingHalfSteps != 9 {
		t.Fatalf("Gallery personal state = %#v", galleryState)
	}
	finalGallery, err := db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalGallery.MetadataRevision != created.MetadataRevision+2 {
		t.Fatalf("rating revisions = %d, want %d", finalGallery.MetadataRevision, created.MetadataRevision+2)
	}
}

func TestPersonalRatingRejectsInvalidAndStaleValues(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC)
	created, _, _ := createCompleteAlbumFixture(t, db, now)
	states := db.PersonalStates()

	invalid := 11
	if err := states.SetGalleryRating(ctx, created.ID, &invalid, created.MetadataRevision, now); err == nil {
		t.Fatal("invalid rating unexpectedly succeeded")
	}
	valid := 10
	if err := states.SetGalleryRating(ctx, created.ID, &valid, created.MetadataRevision-1, now); !errors.Is(err, ErrMetadataRevisionConflict) {
		t.Fatalf("stale rating error = %v, want ErrMetadataRevisionConflict", err)
	}
	state, err := states.Gallery(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.RatingHalfSteps != nil {
		t.Fatalf("stale transaction leaked rating: %#v", state)
	}
}

func TestGalleryHistoryLastItemMustBelongToGallery(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC)
	first, _, firstItem := createCompleteAlbumFixture(t, db, now)
	second, _, secondItem := createCompleteAlbumFixture(t, db, now.Add(time.Minute))
	states := db.PersonalStates()

	if err := states.RecordGalleryView(ctx, first.ID, &firstItem.ID, now); err != nil {
		t.Fatalf("recording valid history: %v", err)
	}
	if err := states.RecordGalleryView(ctx, first.ID, &secondItem.ID, now); err == nil {
		t.Fatal("cross-Gallery last_item_id unexpectedly succeeded")
	}
	state, err := states.Gallery(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastItemID == nil || *state.LastItemID != firstItem.ID || state.LastViewedAtUTC == nil {
		t.Fatalf("history state = %#v", state)
	}
	if second.ID == first.ID {
		t.Fatal("test fixtures unexpectedly share Gallery identity")
	}

	if err := states.ClearGalleryHistory(ctx, first.ID); err != nil {
		t.Fatalf("clearing history: %v", err)
	}
	state, err = states.Gallery(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastItemID != nil || state.LastViewedAtUTC != nil {
		t.Fatalf("history was not cleared independently: %#v", state)
	}
}

func TestSourceIssueBlocksActivationUntilResolved(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC)
	created, source, _ := createCompleteAlbumFixture(t, db, now)
	store := db.Galleries()

	if err := store.PutSourceIssue(
		ctx,
		source.ID,
		"ARCHIVE_UNSUPPORTED_MEDIA",
		gallery.IssueSeverityBlocking,
		"archive contains video",
		now,
	); err != nil {
		t.Fatal(err)
	}
	_, err := store.SetState(ctx, created.ID, created.MetadataRevision, gallery.StateActive, now)
	var activationErr *ActivationError
	if !errors.As(err, &activationErr) {
		t.Fatalf("activation error = %v, want ActivationError", err)
	}
	if err := store.ResolveSourceIssue(ctx, source.ID, "ARCHIVE_UNSUPPORTED_MEDIA", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetState(ctx, created.ID, created.MetadataRevision, gallery.StateActive, now.Add(time.Minute)); err != nil {
		t.Fatalf("activation after resolving issue: %v", err)
	}
}
