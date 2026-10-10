package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

// Archive evidence is local, discardable scan state. It is deliberately not
// part of the portable Gallery identity or Manifest contract.
func createSourceScanEvidenceSchemaV19(ctx context.Context, tx *sql.Tx) error {
	present, err := tableColumnExists(ctx, tx, "gallery_items", "scan_evidence_version")
	if err != nil {
		return err
	}
	if !present {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE gallery_items ADD COLUMN
			scan_evidence_version INTEGER NOT NULL DEFAULT 0 CHECK(scan_evidence_version >= 0)`); err != nil {
			return fmt.Errorf("adding GalleryItem scan evidence version: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS gallery_source_scan_evidence (
		source_id INTEGER PRIMARY KEY REFERENCES gallery_sources(id) ON DELETE CASCADE,
		container_size INTEGER NOT NULL CHECK(container_size >= 0),
		container_modified_at_utc TEXT NOT NULL,
		archive_limits_json TEXT NOT NULL,
		scanner_version INTEGER NOT NULL CHECK(scanner_version > 0),
		checked_at_utc TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("creating source scan evidence schema version 19: %w", err)
	}
	return nil
}

func validateSchemaV19(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV18(ctx, db); err != nil {
		return err
	}
	if err := requireTableColumns(ctx, db, "gallery_items", []string{"scan_evidence_version"}); err != nil {
		return err
	}
	return requireTableColumns(ctx, db, "gallery_source_scan_evidence", []string{
		"source_id", "container_size", "container_modified_at_utc", "archive_limits_json", "scanner_version", "checked_at_utc",
	})
}
