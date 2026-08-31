package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Schema v8 records the built-in ARCHIVE_FILE recognition method and freezes
// archive auto-import consent into each automation run. Existing policies and
// runs remain opted out.
func createArchiveDiscoverySchemaV8(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE gallery_candidates_v8 (
			id INTEGER NOT NULL PRIMARY KEY,
			snapshot_id INTEGER NOT NULL REFERENCES discovery_snapshots(id) ON DELETE CASCADE,
			library_id INTEGER NOT NULL REFERENCES media_libraries(id) ON DELETE CASCADE,
			gallery_id INTEGER REFERENCES galleries(id) ON DELETE SET NULL,
			manifest_set_id TEXT CHECK (manifest_set_id IS NULL OR length(manifest_set_id) = 36),
			rebind_gallery_id INTEGER REFERENCES galleries(id) ON DELETE SET NULL,
			root_path TEXT NOT NULL CHECK (root_path <> '' AND length(root_path) <= 4096),
			source_type TEXT NOT NULL CHECK (source_type IN ('DIRECTORY', 'ARCHIVE')),
			recognition_method TEXT NOT NULL
				CHECK (recognition_method IN ('MANIFEST', 'MARKER', 'PATH_TEMPLATE', 'FIXED_DEPTH', 'ARCHIVE_FILE')),
			rule_id INTEGER REFERENCES gallery_recognition_rules(id) ON DELETE SET NULL,
			auto_create_draft INTEGER NOT NULL DEFAULT 0 CHECK (auto_create_draft IN (0, 1)),
			has_conflict INTEGER NOT NULL DEFAULT 0 CHECK (has_conflict IN (0, 1)),
			over_limit INTEGER NOT NULL DEFAULT 0 CHECK (over_limit IN (0, 1)),
			media_count INTEGER NOT NULL DEFAULT 0 CHECK (media_count >= 0),
			status TEXT NOT NULL DEFAULT 'PENDING'
				CHECK (status IN ('PENDING', 'IMPORTED', 'REJECTED', 'SOURCE_REBIND_CANDIDATE', 'REBOUND')),
			created_at_utc TEXT NOT NULL,
			UNIQUE (snapshot_id, root_path)
		);
		CREATE TABLE gallery_candidate_suggestions_v8 (
			id INTEGER NOT NULL PRIMARY KEY,
			candidate_id INTEGER NOT NULL REFERENCES gallery_candidates_v8(id) ON DELETE CASCADE,
			field_name TEXT NOT NULL
				CHECK (field_name IN ('title', 'coser', 'work', 'character', 'year', 'month')),
			value TEXT NOT NULL CHECK (value <> '' AND length(value) <= 300),
			status TEXT NOT NULL DEFAULT 'PENDING'
				CHECK (status IN ('PENDING', 'ACCEPTED', 'REJECTED')),
			UNIQUE (candidate_id, field_name, value)
		);
		INSERT INTO gallery_candidates_v8 SELECT * FROM gallery_candidates;
		INSERT INTO gallery_candidate_suggestions_v8 SELECT * FROM gallery_candidate_suggestions;
		DROP TABLE gallery_candidate_suggestions;
		DROP TABLE gallery_candidates;
		ALTER TABLE gallery_candidates_v8 RENAME TO gallery_candidates;
		ALTER TABLE gallery_candidate_suggestions_v8 RENAME TO gallery_candidate_suggestions;
		ALTER TABLE library_automation_policies ADD COLUMN auto_import_archives INTEGER NOT NULL DEFAULT 0 CHECK(auto_import_archives IN (0,1));
		ALTER TABLE library_automation_runs ADD COLUMN auto_import_archives INTEGER NOT NULL DEFAULT 0 CHECK(auto_import_archives IN (0,1));
	`); err != nil {
		return fmt.Errorf("creating archive discovery schema version 8: %w", err)
	}
	return nil
}

func validateArchiveDiscoverySchemaV8(ctx context.Context, db *sql.DB) error {
	var candidateSQL string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='gallery_candidates'`).Scan(&candidateSQL); err != nil {
		return err
	}
	if !strings.Contains(candidateSQL, "'ARCHIVE_FILE'") {
		return fmt.Errorf("%w: gallery_candidates does not allow ARCHIVE_FILE", ErrInvalidProductSchema)
	}
	for _, table := range []string{"library_automation_policies", "library_automation_runs"} {
		rows, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			return err
		}
		found := false
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				_ = rows.Close()
				return err
			}
			found = found || name == "auto_import_archives"
		}
		closeErr := rows.Close()
		if err := errors.Join(rows.Err(), closeErr); err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: %s.auto_import_archives is missing", ErrInvalidProductSchema, table)
		}
	}
	return nil
}

func validateSchemaV8(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV7(ctx, db); err != nil {
		return err
	}
	return validateArchiveDiscoverySchemaV8(ctx, db)
}
