package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
)

func TestFavoriteAndHistoryPagesUsePersonalTimestampsAndScope(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	first, _ := createBrowseGallery(t, db, "First", gallery.ContentRatingNonAdult, now)
	second, _ := createBrowseGallery(t, db, "Second", gallery.ContentRatingNonAdult, now.Add(time.Minute))
	adult, _ := createBrowseGallery(t, db, "Adult", gallery.ContentRatingAdult, now.Add(2*time.Minute))
	for _, value := range []gallery.Gallery{first, second, adult} { activateBrowseFixture(t, db, value.ID, now) }
	if err := db.PersonalStates().SetGalleryFavoriteBySetID(ctx, first.SetID, true, now); err != nil { t.Fatal(err) }
	if err := db.PersonalStates().SetGalleryFavoriteBySetID(ctx, second.SetID, true, now.Add(time.Minute)); err != nil { t.Fatal(err) }
	if err := db.PersonalStates().SetGalleryFavoriteBySetID(ctx, adult.SetID, true, now.Add(2*time.Minute)); err != nil { t.Fatal(err) }
	if err := db.PersonalStates().RecordGalleryViewBySetID(ctx, first.SetID, nil, now); err != nil { t.Fatal(err) }
	if err := db.PersonalStates().RecordGalleryViewBySetID(ctx, second.SetID, nil, now.Add(time.Minute)); err != nil { t.Fatal(err) }
	favorites, err := db.Browse().FavoriteGalleries(ctx, browse.ScopeList, 1)
	if err != nil { t.Fatal(err) }
	if len(favorites.Items) != 2 || favorites.Items[0].SetID != second.SetID || favorites.Items[1].SetID != first.SetID { t.Fatalf("favorites = %#v", favorites.Items) }
	history, err := db.Browse().GalleryHistory(ctx, browse.ScopeAll, 1)
	if err != nil { t.Fatal(err) }
	if len(history.Items) != 2 || history.Items[0].SetID != second.SetID { t.Fatalf("history = %#v", history.Items) }
}
