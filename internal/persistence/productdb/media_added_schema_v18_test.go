package productdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestSchemaV17UpgradePreservesLifecycleTimestampAndStartsEvidencePending(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "product.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Existing"}, now)
	if err != nil {
		t.Fatal(err)
	}
	// An empty Gallery cannot activate; install the legacy lifecycle value
	// directly because this test is about migration semantics.
	activatedAt := formatTime(now.Add(time.Hour))
	if _, err := db.ExecContext(ctx, `UPDATE galleries SET added_at_utc=? WHERE id=?`, activatedAt, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	legacy, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	for table, columns := range map[string][]string{
		"gallery_scan_observations": {"source_modified_origin", "source_modified_status", "source_modified_at_utc"},
		"gallery_items":             {"source_modified_checked_at_utc", "source_modified_origin", "source_modified_status", "source_modified_at_utc"},
		"galleries":                 {"publish_date_precision", "publish_date", "media_added_revision", "media_added_status", "media_added_end_at_utc", "media_added_start_at_utc", "first_activated_at_utc"},
	} {
		for _, column := range columns {
			if _, err := legacy.ExecContext(ctx, `ALTER TABLE `+table+` DROP COLUMN `+column); err != nil {
				legacy.Close()
				t.Fatal(err)
			}
		}
	}
	if _, err := legacy.ExecContext(ctx, `UPDATE cgm_product_identity SET database_schema_version=17 WHERE singleton_id=1`); err != nil {
		legacy.Close()
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
	var firstActivated, status string
	if err := upgraded.QueryRowContext(ctx, `SELECT first_activated_at_utc,media_added_status FROM galleries WHERE id=?`, created.ID).Scan(&firstActivated, &status); err != nil {
		t.Fatal(err)
	}
	if firstActivated != activatedAt || status != "PENDING" {
		t.Fatalf("migrated lifecycle/evidence = %q/%q", firstActivated, status)
	}
}
