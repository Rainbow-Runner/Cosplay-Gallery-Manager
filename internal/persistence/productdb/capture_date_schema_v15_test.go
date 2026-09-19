package productdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestSchemaV14UpgradePreservesManualShootDate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "product.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Existing", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE galleries SET shoot_date='2024-05-20',shoot_date_precision='DAY' WHERE id=?`, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `DROP TABLE item_capture_dates; DROP TABLE gallery_capture_date_reviews;
		ALTER TABLE galleries DROP COLUMN shoot_date_origin;
		UPDATE cgm_product_identity SET database_schema_version=14 WHERE singleton_id=1`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if upgraded.Identity().DatabaseSchemaVersion != 15 {
		t.Fatalf("schema=%d", upgraded.Identity().DatabaseSchemaVersion)
	}
	var date, origin string
	if err := upgraded.QueryRowContext(ctx, `SELECT shoot_date,shoot_date_origin FROM galleries WHERE id=?`, created.ID).Scan(&date, &origin); err != nil {
		t.Fatal(err)
	}
	if date != "2024-05-20" || origin != "MANUAL" {
		t.Fatalf("date=%q origin=%q", date, origin)
	}
}
