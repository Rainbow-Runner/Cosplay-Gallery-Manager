package productdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSchemaV12MigratesToManifestInspectionV13(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "product.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	old, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.ExecContext(ctx, `DROP TABLE gallery_manifest_inspections; DROP TABLE gallery_manifest_inspection_progress;
		ALTER TABLE media_libraries DROP COLUMN metadata_writeback_enabled;
		UPDATE cgm_product_identity SET database_schema_version=12 WHERE singleton_id=1`); err != nil {
		old.Close()
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	if migrated.Identity().DatabaseSchemaVersion != 14 {
		t.Fatalf("schema = %d", migrated.Identity().DatabaseSchemaVersion)
	}
	if err := validateSchemaV13(ctx, migrated.DB); err != nil {
		t.Fatal(err)
	}
}
