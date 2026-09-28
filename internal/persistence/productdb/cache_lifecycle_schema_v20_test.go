package productdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSchemaV19MigratesCacheLifecycleWithAccurateSnapshot(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "product.sqlite")
	initial, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSchemaV20(ctx, initial.DB); err != nil {
		t.Fatal(err)
	}
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `DROP TRIGGER media_derivative_cleanup_outbox;
		DROP TRIGGER media_derivative_repath_outbox;
		DROP TABLE cache_cleanup_outbox; DROP TABLE cache_cleanup_scan_state;
		DROP INDEX media_derivatives_cache_path; DROP INDEX processing_jobs_cache_identity;
		UPDATE cgm_product_identity SET database_schema_version=19 WHERE singleton_id=1`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if upgraded.Identity().DatabaseSchemaVersion != 20 {
		t.Fatal("upgrade did not advance schema")
	}
	if err := validateSchemaV20(ctx, upgraded.DB); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := upgraded.QueryRowContext(ctx, `SELECT COUNT(*) FROM cache_cleanup_outbox`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("queue=%d %v", count, err)
	}
	backups, err := filepath.Glob(path + ".pre-schema-v19-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups=%v %v", backups, err)
	}
	snapshot, err := sql.Open(sqliteDriver, sqliteDSN(backups[0], true))
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	identity, err := readIdentity(ctx, snapshot)
	if err != nil || identity.DatabaseSchemaVersion != 19 {
		t.Fatalf("snapshot=%+v %v", identity, err)
	}
	if err := validateSchemaV19(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='cache_cleanup_outbox'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("snapshot included new cache schema")
	}
}
