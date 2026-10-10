package productdb

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/portablecatalog"
)

func TestPortableOwnerContinuityRestoresOnlySelectedLightweightState(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	created, _, item := createCompleteAlbumFixture(t, db, now)
	if err := db.PersonalStates().RecordGalleryView(ctx, created.ID, &item.ID, now); err != nil {
		t.Fatal(err)
	}
	rating := 8
	if err := db.PersonalStates().SetGalleryRating(ctx, created.ID, &rating, created.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT metadata_revision FROM galleries WHERE id=?`, created.ID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	itemRating := 6
	if err := db.PersonalStates().SetItemRating(ctx, item.ID, &itemRating, revision, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO portable_import_sessions(import_id,export_id,package_sha256,package_relative_path,format_version,state,identity_count,core_entity_count,gallery_claim_count,item_claim_count,link_claim_count,asset_count,created_at_utc,updated_at_utc)
		VALUES('11111111-1111-4111-8111-111111111111','22222222-2222-4222-8222-222222222222',?,'portable-imports/11111111-1111-4111-8111-111111111111/package.zip',2,'GALLERIES_REBUILT',0,0,0,0,0,0,?,?)`, strings.Repeat("0", 64), formatTime(now), formatTime(now)); err != nil {
		t.Fatal(err)
	}
	favoriteAt := now.Add(time.Minute).Format(time.RFC3339Nano)
	owner := portablecatalog.OwnerContinuity{SchemaVersion: 1, IncludesPersonalFlags: true, Galleries: []portablecatalog.GalleryOwnerContinuity{{SetID: created.SetID, Favorite: true, FavoritedAt: favoriteAt, Hidden: true, Items: []portablecatalog.ItemOwnerContinuity{{ItemUUID: item.UUID, Favorite: true, FavoritedAt: favoriteAt}}}}}
	result, err := db.ApplyPortableOwnerContinuity(ctx, "11111111-1111-4111-8111-111111111111", owner, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if result.GalleryCount != 1 || result.ItemCount != 1 {
		t.Fatalf("result = %+v", result)
	}
	state, err := db.PersonalStates().Gallery(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Favorite || !state.Hidden || state.RatingHalfSteps == nil || *state.RatingHalfSteps != rating || state.LastViewedAtUTC == nil || state.LastItemID == nil {
		t.Fatalf("unselected rating/history changed: %+v", state)
	}
	var retainedItemRating int
	if err := db.QueryRowContext(ctx, `SELECT rating_half_steps FROM gallery_item_personal_states WHERE gallery_item_id=?`, item.ID).Scan(&retainedItemRating); err != nil || retainedItemRating != itemRating {
		t.Fatalf("unselected Item rating changed: %d, %v", retainedItemRating, err)
	}
}
