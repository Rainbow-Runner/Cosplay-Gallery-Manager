package productdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSchemaV20UpgradeAddsDurableScanSummary(t *testing.T) {
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
	for _, name := range []string{"added_count", "missing_count", "changed_count", "rebound_count", "cleared_count", "cover_reselected"} {
		if _, err := legacy.ExecContext(ctx, `ALTER TABLE gallery_scan_runs DROP COLUMN `+name); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	if _, err := legacy.ExecContext(ctx, `UPDATE cgm_product_identity SET database_schema_version=20 WHERE singleton_id=1`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	legacy.Close()
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if upgraded.Identity().DatabaseSchemaVersion != 22 {
		t.Fatalf("schema=%d", upgraded.Identity().DatabaseSchemaVersion)
	}
	if err := validateSchemaV21(ctx, upgraded.DB); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(path + ".pre-schema-v20-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups=%v error=%v", backups, err)
	}
}
