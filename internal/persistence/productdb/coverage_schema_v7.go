package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredCoverageSchemaV7Tables = []string{"library_coverage_summaries", "library_coverage_diagnostics"}

func createCoverageSchemaV7(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE library_coverage_summaries (
			snapshot_id INTEGER NOT NULL PRIMARY KEY REFERENCES discovery_snapshots(id) ON DELETE CASCADE,
			library_id INTEGER NOT NULL REFERENCES media_libraries(id) ON DELETE CASCADE,
			regular_file_count INTEGER NOT NULL CHECK(regular_file_count >= 0),
			supported_media_count INTEGER NOT NULL CHECK(supported_media_count >= 0),
			supported_archive_count INTEGER NOT NULL CHECK(supported_archive_count >= 0),
			unsupported_archive_count INTEGER NOT NULL CHECK(unsupported_archive_count >= 0),
			control_file_count INTEGER NOT NULL CHECK(control_file_count >= 0),
			ignored_other_count INTEGER NOT NULL CHECK(ignored_other_count >= 0),
			actionable_issue_count INTEGER NOT NULL CHECK(actionable_issue_count >= 0)
		);
		CREATE TABLE library_coverage_diagnostics (
			id INTEGER NOT NULL PRIMARY KEY,
			snapshot_id INTEGER NOT NULL REFERENCES discovery_snapshots(id) ON DELETE CASCADE,
			library_id INTEGER NOT NULL REFERENCES media_libraries(id) ON DELETE CASCADE,
			path TEXT NOT NULL CHECK(path <> '' AND length(path) <= 4096),
			entry_kind TEXT NOT NULL CHECK(entry_kind IN ('DIRECTORY','ARCHIVE')),
			reason_code TEXT NOT NULL CHECK(reason_code IN (
				'UNASSIGNED_MEDIA_DIRECTORY','UNSUPPORTED_ARCHIVE_FORMAT','ENCRYPTED_ARCHIVE',
				'UNREADABLE_ARCHIVE','UNSAFE_ARCHIVE','ARCHIVE_WITHOUT_SUPPORTED_MEDIA'
			)),
			file_count INTEGER NOT NULL CHECK(file_count > 0),
			byte_size INTEGER NOT NULL DEFAULT 0 CHECK(byte_size >= 0),
			UNIQUE(snapshot_id,path,reason_code)
		);
		CREATE INDEX library_coverage_diagnostics_snapshot_reason
			ON library_coverage_diagnostics(snapshot_id,reason_code,path);
	`); err != nil {
		return fmt.Errorf("creating coverage schema version 7: %w", err)
	}
	return nil
}

func validateCoverageSchemaV7(ctx context.Context, db *sql.DB) error {
	for _, table := range requiredCoverageSchemaV7Tables {
		exists, err := schemaObjectExists(ctx, db, "table", table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required table %q is missing", ErrInvalidProductSchema, table)
		}
	}
	return nil
}

func validateSchemaV7(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV6(ctx, db); err != nil {
		return err
	}
	return validateCoverageSchemaV7(ctx, db)
}
