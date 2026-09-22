package productdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSchemaV15UpgradesToRecoveryV16(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "product.sqlite")
	current, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	old, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.ExecContext(ctx, `DROP TABLE owner_recovery_tokens; UPDATE cgm_product_identity SET database_schema_version=15 WHERE singleton_id=1`); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if upgraded.Identity().DatabaseSchemaVersion != 16 {
		t.Fatalf("schema = %d", upgraded.Identity().DatabaseSchemaVersion)
	}
	if err := validateSchemaV16(ctx, upgraded.DB); err != nil {
		t.Fatal(err)
	}
}
