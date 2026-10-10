package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

// Schema v17 persists only evidence and user authorization needed by the two
// portable package profiles and Manifest identity review. Existing sessions
// and candidates keep conservative legacy defaults; no Gallery, Manifest or
// UUID lifecycle row is rewritten by this migration.
func createPortableIdentitySchemaV17(ctx context.Context, tx *sql.Tx) error {
	columns := []struct{ table, name, definition string }{
		{"portable_import_sessions", "package_profile", `package_profile TEXT NOT NULL DEFAULT 'GALLERY_IDENTITY_ASSISTED_LEGACY' CHECK(package_profile IN ('CORE_CATALOG','GALLERY_IDENTITY_ASSISTED','GALLERY_IDENTITY_ASSISTED_LEGACY'))`},
		{"portable_import_sessions", "auto_adopt_enabled", `auto_adopt_enabled INTEGER NOT NULL DEFAULT 0 CHECK(auto_adopt_enabled IN (0,1))`},
		{"portable_import_sessions", "auto_activate_enabled", `auto_activate_enabled INTEGER NOT NULL DEFAULT 0 CHECK(auto_activate_enabled IN (0,1))`},
		{"portable_import_sessions", "automation_authorized_at_utc", `automation_authorized_at_utc TEXT NOT NULL DEFAULT ''`},
		{"portable_gallery_rebuilds", "exported_relative_source", `exported_relative_source TEXT NOT NULL DEFAULT '' CHECK(length(exported_relative_source)<=4096)`},
		{"portable_gallery_rebuilds", "resolved_target_library_id", `resolved_target_library_id INTEGER REFERENCES media_libraries(id) ON DELETE RESTRICT`},
		{"portable_gallery_rebuilds", "resolved_relative_source", `resolved_relative_source TEXT NOT NULL DEFAULT '' CHECK(length(resolved_relative_source)<=4096)`},
		{"portable_gallery_rebuilds", "source_resolution", `source_resolution TEXT NOT NULL DEFAULT 'UNRESOLVED' CHECK(source_resolution IN ('EXACT','RELOCATED_UNIQUE','DUPLICATE_ACCESSIBLE_SOURCE','UNRESOLVED'))`},
		{"portable_gallery_rebuilds", "resolution_snapshot_id", `resolution_snapshot_id INTEGER REFERENCES discovery_snapshots(id) ON DELETE SET NULL`},
		{"portable_gallery_rebuilds", "resolution_manifest_hash", `resolution_manifest_hash TEXT NOT NULL DEFAULT '' CHECK(length(resolution_manifest_hash)<=200)`},
		{"portable_gallery_rebuilds", "resolution_token_hash", `resolution_token_hash TEXT NOT NULL DEFAULT '' CHECK(length(resolution_token_hash) IN (0,64))`},
		{"portable_gallery_rebuilds", "adopted_manifest_schema", `adopted_manifest_schema INTEGER NOT NULL DEFAULT 0 CHECK(adopted_manifest_schema>=0)`},
		{"portable_gallery_rebuilds", "adopted_manifest_revision", `adopted_manifest_revision INTEGER NOT NULL DEFAULT 0 CHECK(adopted_manifest_revision>=0)`},
		{"portable_gallery_rebuilds", "adopted_manifest_hash", `adopted_manifest_hash TEXT NOT NULL DEFAULT '' CHECK(length(adopted_manifest_hash)<=200)`},
		{"portable_gallery_rebuilds", "adopted_from_status", `adopted_from_status TEXT NOT NULL DEFAULT '' CHECK(adopted_from_status IN ('','CLEAN','MISSING','ERROR','DB_DIRTY','FILE_DIRTY','CONFLICT'))`},
		{"portable_gallery_rebuilds", "adopted_at_utc", `adopted_at_utc TEXT NOT NULL DEFAULT ''`},
		{"portable_gallery_rebuilds", "adoption_token_hash", `adoption_token_hash TEXT NOT NULL DEFAULT '' CHECK(length(adoption_token_hash) IN (0,64))`},
		{"gallery_candidates", "identity_classification", `identity_classification TEXT NOT NULL DEFAULT '' CHECK(identity_classification IN ('','UNCLAIMED','SAME_GALLERY_SOURCE_MOVE','DUPLICATE_ACCESSIBLE_SOURCE','PENDING_PORTABLE_CLAIM','UUID_KIND_CONFLICT','UUID_RETIRED','LOCAL_ITEM_OR_LINK_CONFLICT','CORE_REFERENCE_REVIEW'))`},
		{"gallery_candidates", "identity_issue_code", `identity_issue_code TEXT NOT NULL DEFAULT '' CHECK(length(identity_issue_code)<=100)`},
		{"gallery_candidates", "manifest_schema", `manifest_schema INTEGER NOT NULL DEFAULT 0 CHECK(manifest_schema>=0)`},
		{"gallery_candidates", "manifest_revision", `manifest_revision INTEGER NOT NULL DEFAULT 0 CHECK(manifest_revision>=0)`},
		{"gallery_candidates", "manifest_hash", `manifest_hash TEXT NOT NULL DEFAULT '' CHECK(length(manifest_hash)<=200)`},
		{"gallery_candidates", "inspection_token_hash", `inspection_token_hash TEXT NOT NULL DEFAULT '' CHECK(length(inspection_token_hash) IN (0,64))`},
	}
	for _, column := range columns {
		present, err := tableColumnExists(ctx, tx, column.table, column.name)
		if err != nil {
			return err
		}
		if present {
			continue
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE `+column.table+` ADD COLUMN `+column.definition); err != nil {
			return fmt.Errorf("creating portable identity schema version 17: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE portable_gallery_rebuilds SET exported_relative_source=relative_source WHERE exported_relative_source=''`); err != nil {
		return fmt.Errorf("backfilling portable identity schema version 17: %w", err)
	}
	return nil
}

func tableColumnExists(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, table, column string) (bool, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func validateSchemaV17(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV16(ctx, db); err != nil {
		return err
	}
	for table, columns := range map[string][]string{
		"portable_import_sessions":  {"package_profile", "auto_adopt_enabled", "auto_activate_enabled", "automation_authorized_at_utc"},
		"portable_gallery_rebuilds": {"exported_relative_source", "resolved_target_library_id", "resolved_relative_source", "source_resolution", "resolution_snapshot_id", "resolution_manifest_hash", "resolution_token_hash", "adopted_manifest_schema", "adopted_manifest_revision", "adopted_manifest_hash", "adopted_from_status", "adopted_at_utc", "adoption_token_hash"},
		"gallery_candidates":        {"identity_classification", "identity_issue_code", "manifest_schema", "manifest_revision", "manifest_hash", "inspection_token_hash"},
	} {
		if err := requireTableColumns(ctx, db, table, columns); err != nil {
			return err
		}
	}
	return nil
}

func requireTableColumns(ctx context.Context, db *sql.DB, table string, required []string) error {
	rows, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return err
	}
	found := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		found[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, name := range required {
		if !found[name] {
			return fmt.Errorf("%w: %s.%s is missing", ErrInvalidProductSchema, table, name)
		}
	}
	return nil
}
