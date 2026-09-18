package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

// Inspection results are deliberately separate from synchronization baselines:
// an untracked or absent file must not be mistaken for an established baseline.
func createManifestInspectionSchemaV13(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE gallery_manifest_inspections (
			gallery_id INTEGER PRIMARY KEY REFERENCES galleries(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK(status IN ('NONE','CLEAN','DB_DIRTY','FILE_DIRTY','CONFLICT','MISSING','ERROR','SOURCE_UNAVAILABLE')),
			checked_at_utc TEXT NOT NULL,
			checked_metadata_revision INTEGER NOT NULL,
			source_path TEXT NOT NULL,
			file_hash TEXT NOT NULL DEFAULT '',
			error_code TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE gallery_manifest_inspection_progress (
			singleton_id INTEGER PRIMARY KEY CHECK(singleton_id=1),
			last_gallery_id INTEGER NOT NULL DEFAULT 0
		);
		INSERT INTO gallery_manifest_inspection_progress(singleton_id,last_gallery_id) VALUES(1,0);
	`)
	if err != nil {
		return fmt.Errorf("creating Gallery Manifest inspection schema v13: %w", err)
	}
	return nil
}

func validateSchemaV13(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV12(ctx, db); err != nil {
		return err
	}
	for _, table := range []string{"gallery_manifest_inspections", "gallery_manifest_inspection_progress"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			return fmt.Errorf("%w: table %s is missing", ErrInvalidProductSchema, table)
		}
	}
	return nil
}
