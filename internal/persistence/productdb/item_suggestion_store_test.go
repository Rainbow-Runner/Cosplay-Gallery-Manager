package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestScanCreatesNonBlockingSelfieAndRAWCompanionSuggestions(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 13, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	runID, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	selfie := scanPhoto("自拍/portrait.jpg", "selfie")
	raw := scanPhoto("shoot/a.dng", "raw")
	raw.ContentFormat = gallery.ContentFormatRAW
	jpeg := scanPhoto("shoot/a.jpg", "jpeg")
	for _, observation := range []ScanObservation{selfie, raw, jpeg} {
		if err := db.Scans().Stage(ctx, runID, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().Commit(ctx, runID, now); err != nil {
		t.Fatal(err)
	}
	var selfieCount, companionCount int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM gallery_item_suggestions WHERE gallery_id=? AND suggestion_kind='SELFIE_CATEGORY' AND status='PENDING'),
		(SELECT COUNT(*) FROM gallery_item_suggestions WHERE gallery_id=? AND suggestion_kind='RAW_COMPANION' AND status='PENDING')`, created.ID, created.ID).Scan(&selfieCount, &companionCount); err != nil {
		t.Fatal(err)
	}
	if selfieCount != 1 || companionCount != 2 {
		t.Fatalf("suggestion counts = selfie %d companion %d", selfieCount, companionCount)
	}
	var suggestionID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM gallery_item_suggestions WHERE gallery_id=? AND suggestion_kind='SELFIE_CATEGORY'`, created.ID).Scan(&suggestionID); err != nil {
		t.Fatal(err)
	}
	resolved, err := db.ItemSuggestions().Resolve(ctx, suggestionID, false, created.MetadataRevision, now)
	if err != nil || resolved.Status != "REJECTED" {
		t.Fatalf("rejected suggestion = %#v, %v", resolved, err)
	}

	second, err := db.Scans().Begin(ctx, source.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, observation := range []ScanObservation{selfie, raw, jpeg} {
		if err := db.Scans().Stage(ctx, second, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().Commit(ctx, second, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM gallery_item_suggestions WHERE id=?`, suggestionID).Scan(&resolved.Status); err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "REJECTED" {
		t.Fatalf("rescan restored rejected SELFIE suggestion: %s", resolved.Status)
	}
}

func TestAcceptedEXIFDateIsExplicitGalleryMetadataChange(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)
	galleryID, items := createDerivativeTestGallery(t, db, now)
	suggestion, err := db.ItemSuggestions().SuggestCaptureDate(ctx, items[0].UUID, "2024-05-17", "EXIF", now)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := db.Galleries().Find(ctx, galleryID)
	if before.ShootDate != "" {
		t.Fatal("EXIF suggestion silently changed Gallery date")
	}
	resolved, err := db.ItemSuggestions().Resolve(ctx, suggestion.ID, true, before.MetadataRevision, now.Add(time.Minute))
	if err != nil || resolved.Status != "ACCEPTED" {
		t.Fatalf("accepted date suggestion = %#v, %v", resolved, err)
	}
	after, _ := db.Galleries().Find(ctx, galleryID)
	if after.ShootDate != "2024-05-17" || after.ShootDatePrecision != gallery.ShootDatePrecisionDay || after.MetadataRevision != before.MetadataRevision+1 {
		t.Fatalf("Gallery after explicit EXIF acceptance = %#v", after)
	}
}
