package productdb

import (
	"context"
	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"testing"
	"time"
)

func TestCoserTimelineCursorOrderingAndValidation(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 19, 0, 0, 0, time.UTC)
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Cursor Coser"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2024-04", "2024-05", "2024-05"} {
		item, _ := createBrowseGallery(t, db, date, gallery.ContentRatingNonAdult, now)
		if _, err := db.Galleries().AddCredit(ctx, item.ID, coser.UUID, 1024, item.MetadataRevision, now); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE galleries SET shoot_date=?,shoot_date_precision='MONTH' WHERE id=?`, date, item.ID); err != nil {
			t.Fatal(err)
		}
		activateBrowseFixture(t, db, item.ID, now)
	}
	first, err := db.Browse().CoserTimeline(ctx, browse.ScopeAll, coser.UUID, browse.TimelineShoot, 1, "")
	if err != nil || len(first.Items) != 1 || !first.HasNextPage || first.Items[0].Month != "2024-05" {
		t.Fatalf("first = %#v, %v", first, err)
	}
	second, err := db.Browse().CoserTimeline(ctx, browse.ScopeAll, coser.UUID, browse.TimelineShoot, 1, first.EndCursor)
	if err != nil || len(second.Items) != 1 || !second.HasNextPage || second.Items[0].Month != "2024-05" || second.Items[0].Card.SetID == first.Items[0].Card.SetID {
		t.Fatalf("second = %#v, %v", second, err)
	}
	last, err := db.Browse().CoserTimeline(ctx, browse.ScopeAll, coser.UUID, browse.TimelineShoot, 24, second.EndCursor)
	if err != nil || len(last.Items) != 1 || last.HasNextPage || last.Items[0].Month != "2024-04" {
		t.Fatalf("last = %#v, %v", last, err)
	}
	for _, cursor := range []string{"bad!", first.EndCursor + "!"} {
		if _, err := db.Browse().CoserTimeline(ctx, browse.ScopeAll, coser.UUID, browse.TimelineShoot, 24, cursor); err == nil {
			t.Fatal("invalid cursor accepted")
		}
	}
	if _, err := db.Browse().CoserTimeline(ctx, browse.ScopeAll, coser.UUID, browse.TimelinePublish, 24, first.EndCursor); err == nil {
		t.Fatal("cross-mode cursor accepted")
	}
	if _, err := db.Browse().CoserTimeline(ctx, browse.ScopeAll, coser.UUID, browse.TimelineShoot, 25, ""); err == nil {
		t.Fatal("oversized batch accepted")
	}
	if _, err := db.Browse().CoserTimeline(ctx, browse.ScopeList, coser.UUID, browse.TimelineShoot, 24, first.EndCursor); err == nil {
		t.Fatal("cross-scope cursor accepted")
	}
	if _, err := db.Browse().CoserTimeline(ctx, browse.ScopeAll, "another-coser", browse.TimelineShoot, 24, first.EndCursor); err == nil {
		t.Fatal("cross-coser cursor accepted")
	}
	for _, mode := range []browse.TimelineDate{browse.TimelineShoot, browse.TimelinePublish, browse.TimelineMediaAdded, browse.TimelineCombined} {
		legacy, err := db.Browse().TimelineByDate(ctx, browse.ScopeAll, 1, coser.UUID, mode)
		if err != nil {
			t.Fatal(err)
		}
		connection, err := db.Browse().CoserTimeline(ctx, browse.ScopeAll, coser.UUID, mode, 24, "")
		if err != nil || len(connection.Items) != len(legacy.Items) {
			t.Fatalf("mode %s: %#v, %v", mode, connection, err)
		}
		for index := range legacy.Items {
			if connection.Items[index].Card.SetID != legacy.Items[index].SetID {
				t.Fatalf("mode %s differs from existing timeline", mode)
			}
		}
	}
}
