package productdb

import (
	"context"
	"database/sql"
)

func createScanOptimizationSchemaV25(ctx context.Context, tx *sql.Tx) error {
	for _, c := range []struct{ table, name, ddl string }{
		{"library_automation_runs", "retry_drafts", `ALTER TABLE library_automation_runs ADD COLUMN retry_drafts INTEGER NOT NULL DEFAULT 0 CHECK(retry_drafts IN (0,1))`},
		{"runtime_settings", "library_monitor_enabled", `ALTER TABLE runtime_settings ADD COLUMN library_monitor_enabled INTEGER NOT NULL DEFAULT 0 CHECK(library_monitor_enabled IN (0,1))`},
		{"discovery_snapshots", "archive_reused_count", `ALTER TABLE discovery_snapshots ADD COLUMN archive_reused_count INTEGER NOT NULL DEFAULT 0`},
		{"discovery_snapshots", "archive_inspected_count", `ALTER TABLE discovery_snapshots ADD COLUMN archive_inspected_count INTEGER NOT NULL DEFAULT 0`},
		{"discovery_snapshots", "elapsed_ms", `ALTER TABLE discovery_snapshots ADD COLUMN elapsed_ms INTEGER NOT NULL DEFAULT 0`},
		{"gallery_scan_runs", "content_read_count", `ALTER TABLE gallery_scan_runs ADD COLUMN content_read_count INTEGER NOT NULL DEFAULT 0`},
		{"gallery_scan_runs", "reused_count", `ALTER TABLE gallery_scan_runs ADD COLUMN reused_count INTEGER NOT NULL DEFAULT 0`},
		{"gallery_scan_runs", "elapsed_ms", `ALTER TABLE gallery_scan_runs ADD COLUMN elapsed_ms INTEGER NOT NULL DEFAULT 0`},
	} {
		present, err := tableColumnExists(ctx, tx, c.table, c.name)
		if err != nil {
			return err
		}
		if !present {
			if _, err := tx.ExecContext(ctx, c.ddl); err != nil {
				return err
			}
		}
	}
	_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS library_draft_retry_targets (
		run_id INTEGER NOT NULL REFERENCES library_automation_runs(id) ON DELETE CASCADE,
		gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
		title TEXT NOT NULL, outcome TEXT NOT NULL DEFAULT 'PENDING', reason TEXT NOT NULL DEFAULT '',
		PRIMARY KEY(run_id,gallery_id));
		CREATE TABLE IF NOT EXISTS archive_discovery_cache (
		library_id INTEGER NOT NULL REFERENCES media_libraries(id) ON DELETE CASCADE,
		path TEXT NOT NULL, byte_size INTEGER NOT NULL, modified_at TEXT NOT NULL,
		limits_json TEXT NOT NULL, version INTEGER NOT NULL, observation_json TEXT NOT NULL,
		reason TEXT NOT NULL, PRIMARY KEY(library_id,path));
		CREATE TABLE IF NOT EXISTS library_path_changes (
		library_id INTEGER NOT NULL REFERENCES media_libraries(id) ON DELETE CASCADE,
		path TEXT NOT NULL, changed_at_utc TEXT NOT NULL, PRIMARY KEY(library_id,path));`)
	return err
}

func validateSchemaV25(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV24(ctx, db); err != nil {
		return err
	}
	for table, columns := range map[string][]string{
		"library_automation_runs":     {"retry_drafts"},
		"library_draft_retry_targets": {"run_id", "gallery_id", "title", "outcome", "reason"},
		"runtime_settings":            {"library_monitor_enabled"},
		"archive_discovery_cache":     {"library_id", "path", "byte_size", "modified_at", "limits_json", "version", "observation_json", "reason"},
		"library_path_changes":        {"library_id", "path", "changed_at_utc"},
		"discovery_snapshots":         {"archive_reused_count", "archive_inspected_count", "elapsed_ms"},
		"gallery_scan_runs":           {"content_read_count", "reused_count", "elapsed_ms"},
	} {
		if err := requireTableColumns(ctx, db, table, columns); err != nil {
			return err
		}
	}
	return nil
}
