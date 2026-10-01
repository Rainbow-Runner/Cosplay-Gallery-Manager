package productdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/settings"
)

func TestSchemaV21UpgradeAddsLocalVideoHardwareSettings(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "product.sqlite")
	current, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `ALTER TABLE runtime_settings DROP COLUMN video_hardware_device;
		ALTER TABLE runtime_settings DROP COLUMN video_hardware_fallback_enabled;
		ALTER TABLE runtime_settings DROP COLUMN video_hardware_mode;
		UPDATE cgm_product_identity SET database_schema_version=21 WHERE singleton_id=1`); err != nil {
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
	if upgraded.Identity().DatabaseSchemaVersion != 22 {
		t.Fatalf("schema=%d", upgraded.Identity().DatabaseSchemaVersion)
	}
	runtime, err := upgraded.Settings().Find(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.VideoHardwareMode != settings.VideoHardwareSoftware || !runtime.VideoHardwareFallbackEnabled || runtime.VideoHardwareDevice != "" {
		t.Fatalf("hardware defaults=%#v", runtime)
	}
	backups, err := filepath.Glob(path + ".pre-schema-v21-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups=%v error=%v", backups, err)
	}
	snapshot, err := sql.Open(sqliteDriver, sqliteDSN(backups[0], true))
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	identity, err := readIdentity(ctx, snapshot)
	if err != nil || identity.DatabaseSchemaVersion != 21 {
		t.Fatalf("snapshot=%#v error=%v", identity, err)
	}
}
