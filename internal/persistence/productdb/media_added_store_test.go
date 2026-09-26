package productdb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
)

func TestMediaAddedBackfillUsesStatWithoutMediaDecode(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	if err := os.MkdirAll(source.Path, 0o700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(source.Path, "broken.jpg")
	if err := os.WriteFile(filename, []byte("not an image and intentionally never decoded"), 0o600); err != nil {
		t.Fatal(err)
	}
	modified := time.Date(2021, 7, 8, 9, 10, 11, 0, time.UTC)
	if err := os.Chtimes(filename, modified, modified); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "broken.jpg", MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable,
		ProcessingState: gallery.ProcessingPending,
	}, now); err != nil {
		t.Fatal(err)
	}
	processed, err := db.MediaAdded().BackfillOne(ctx, archivecheck.DefaultLimits(), now.Add(time.Minute))
	if err != nil || !processed {
		t.Fatalf("backfill processed=%v err=%v", processed, err)
	}
	var start, end, status, itemStatus, origin string
	if err := db.QueryRowContext(ctx, `SELECT gallery.media_added_start_at_utc,gallery.media_added_end_at_utc,
		gallery.media_added_status,item.source_modified_status,item.source_modified_origin
		FROM galleries gallery JOIN gallery_items item ON item.gallery_id=gallery.id WHERE gallery.id=?`, created.ID).
		Scan(&start, &end, &status, &itemStatus, &origin); err != nil {
		t.Fatal(err)
	}
	want := modified.Format(time.RFC3339Nano)
	if start != want || end != want || status != "COMPLETE" || itemStatus != "FOUND" || origin != "FILESYSTEM" {
		t.Fatalf("backfill evidence=%q/%q/%q/%q/%q", start, end, status, itemStatus, origin)
	}
}
