package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredMediaExclusionSchemaV5Tables = []string{
	"media_exclusion_rules",
	"media_exclusion_decisions",
}

func createMediaExclusionSchemaV5(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE media_exclusion_rules (
			id INTEGER NOT NULL PRIMARY KEY,
			library_id INTEGER REFERENCES media_libraries(id) ON DELETE CASCADE,
			name TEXT NOT NULL CHECK (name <> '' AND length(name) <= 300),
			enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
			sort_order INTEGER NOT NULL DEFAULT 100,
			match_subject TEXT NOT NULL CHECK (match_subject IN ('PARENT_FOLDER','PARENT_PATH','FILE_NAME','FILE_STEM','RELATIVE_PATH')),
			match_operator TEXT NOT NULL CHECK (match_operator IN ('EXACT','GLOB','RE2')),
			pattern TEXT NOT NULL CHECK (pattern <> '' AND length(pattern) <= 4000),
			case_sensitive INTEGER NOT NULL DEFAULT 0 CHECK (case_sensitive IN (0,1)),
			media_kind TEXT NOT NULL DEFAULT 'ALL' CHECK (media_kind IN ('ALL','STATIC_IMAGE','ANIMATED_IMAGE','VIDEO')),
			decision TEXT NOT NULL CHECK (decision IN ('EXCLUDE','INCLUDE')),
			revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
			system_default INTEGER NOT NULL DEFAULT 0 CHECK (system_default IN (0,1)),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL
		);
		CREATE INDEX media_exclusion_rules_effective
			ON media_exclusion_rules(enabled,sort_order,library_id,id);

		CREATE TABLE media_exclusion_decisions (
			id INTEGER NOT NULL PRIMARY KEY,
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			item_uuid TEXT NOT NULL REFERENCES gallery_items(item_uuid) ON DELETE CASCADE,
			rule_id INTEGER REFERENCES media_exclusion_rules(id) ON DELETE SET NULL,
			rule_revision INTEGER NOT NULL CHECK (rule_revision > 0),
			rule_name TEXT NOT NULL CHECK (rule_name <> '' AND length(rule_name) <= 300),
			decision TEXT NOT NULL CHECK (decision IN ('EXCLUDE','INCLUDE')),
			matched_subject TEXT NOT NULL CHECK (matched_subject IN ('PARENT_FOLDER','PARENT_PATH','FILE_NAME','FILE_STEM','RELATIVE_PATH')),
			matched_value TEXT NOT NULL CHECK (matched_value <> '' AND length(matched_value) <= 4096),
			status TEXT NOT NULL CHECK (status IN ('PENDING','APPLIED','REJECTED','SUPERSEDED','REVERSED')),
			created_at_utc TEXT NOT NULL,
			resolved_at_utc TEXT,
			UNIQUE(item_uuid,rule_id,rule_revision)
		);
		CREATE INDEX media_exclusion_decisions_queue
			ON media_exclusion_decisions(gallery_id,status,id);
		CREATE INDEX media_exclusion_decisions_item
			ON media_exclusion_decisions(item_uuid,status,id);
	`); err != nil {
		return fmt.Errorf("creating media exclusion schema version 5: %w", err)
	}
	return nil
}

func validateMediaExclusionSchemaV5(ctx context.Context, db *sql.DB) error {
	for _, table := range requiredMediaExclusionSchemaV5Tables {
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
