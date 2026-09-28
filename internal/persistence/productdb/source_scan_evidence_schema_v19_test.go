package productdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSchemaV18MigratesToSourceScanEvidenceV19(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "product.sqlite")
	initial, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `DROP TABLE gallery_source_scan_evidence;
		ALTER TABLE gallery_items DROP COLUMN scan_evidence_version;
		UPDATE cgm_product_identity SET database_schema_version=18 WHERE singleton_id=1`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	if migrated.Identity().DatabaseSchemaVersion != 20 {
		t.Fatalf("schema version=%d", migrated.Identity().DatabaseSchemaVersion)
	}
	if err := validateSchemaV19(ctx, migrated.DB); err != nil {
		t.Fatal(err)
	}
}
