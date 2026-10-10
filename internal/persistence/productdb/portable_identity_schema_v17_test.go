package productdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSchemaV16UpgradesToPortableIdentityV17WithoutChangingBusinessRows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "product.sqlite")
	current, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := current.ExecContext(ctx, `INSERT INTO portable_import_sessions(
		import_id,export_id,package_sha256,package_relative_path,format_version,state,identity_count,core_entity_count,
		gallery_claim_count,item_claim_count,link_claim_count,asset_count,created_at_utc,updated_at_utc
	) VALUES(
		'11111111-1111-4111-8111-111111111111','22222222-2222-4222-8222-222222222222',
		'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','portable-imports/test/package.zip',2,'CORE_IMPORTED',0,0,0,0,0,0,
		'2026-09-26T00:00:00Z','2026-09-26T00:00:00Z'
	)`); err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	old, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.ExecContext(ctx, `UPDATE cgm_product_identity SET database_schema_version=16 WHERE singleton_id=1`); err != nil {
		t.Fatal(err)
	}
	// Recreate the true v16 shape after the current Open initialized v17.
	for _, column := range []string{"package_profile", "auto_adopt_enabled", "auto_activate_enabled", "automation_authorized_at_utc"} {
		if _, err := old.ExecContext(ctx, `ALTER TABLE portable_import_sessions DROP COLUMN `+column); err != nil {
			t.Fatal(err)
		}
	}
	for _, column := range []string{"exported_relative_source", "resolved_target_library_id", "resolved_relative_source", "source_resolution", "resolution_snapshot_id", "resolution_manifest_hash", "resolution_token_hash", "adopted_manifest_schema", "adopted_manifest_revision", "adopted_manifest_hash", "adopted_from_status", "adopted_at_utc", "adoption_token_hash"} {
		if _, err := old.ExecContext(ctx, `ALTER TABLE portable_gallery_rebuilds DROP COLUMN `+column); err != nil {
			t.Fatal(err)
		}
	}
	for _, column := range []string{"identity_classification", "identity_issue_code", "manifest_schema", "manifest_revision", "manifest_hash", "inspection_token_hash"} {
		if _, err := old.ExecContext(ctx, `ALTER TABLE gallery_candidates DROP COLUMN `+column); err != nil {
			t.Fatal(err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if err := validateSchemaV17(ctx, upgraded.DB); err != nil {
		t.Fatal(err)
	}
	var profile string
	if err := upgraded.QueryRowContext(ctx, `SELECT package_profile FROM portable_import_sessions WHERE import_id='11111111-1111-4111-8111-111111111111'`).Scan(&profile); err != nil {
		t.Fatal(err)
	}
	if profile != "GALLERY_IDENTITY_ASSISTED_LEGACY" {
		t.Fatalf("legacy profile = %q", profile)
	}
}
