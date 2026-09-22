package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredMediaClassificationSchemaV4Tables = []string{
	"media_classification_rules",
	"media_classification_suggestions",
}

func createMediaClassificationSchemaV4(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE media_classification_rules (
			id INTEGER NOT NULL PRIMARY KEY,
			library_id INTEGER REFERENCES media_libraries(id) ON DELETE CASCADE,
			name TEXT NOT NULL CHECK (name <> '' AND length(name) <= 300),
			enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
			sort_order INTEGER NOT NULL DEFAULT 100,
			match_subject TEXT NOT NULL CHECK (match_subject IN ('PARENT_FOLDER','FILE_NAME','FILE_STEM','RELATIVE_PATH')),
			match_operator TEXT NOT NULL CHECK (match_operator IN ('EXACT','GLOB','RE2')),
			pattern TEXT NOT NULL CHECK (pattern <> '' AND length(pattern) <= 4000),
			case_sensitive INTEGER NOT NULL DEFAULT 0 CHECK (case_sensitive IN (0,1)),
			result_category TEXT NOT NULL CHECK (result_category IN ('PHOTO','SELFIE')),
			revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
			system_default INTEGER NOT NULL DEFAULT 0 CHECK (system_default IN (0,1)),
			default_key TEXT UNIQUE CHECK (default_key IS NULL OR default_key IN ('DEFAULT_FOLDER','OPTIONAL_FILENAME')),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL
		);
		CREATE INDEX media_classification_rules_effective
			ON media_classification_rules(enabled,sort_order,library_id,id);

		CREATE TABLE media_classification_suggestions (
			id INTEGER NOT NULL PRIMARY KEY,
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			item_uuid TEXT NOT NULL REFERENCES gallery_items(item_uuid) ON DELETE CASCADE,
			rule_id INTEGER REFERENCES media_classification_rules(id) ON DELETE SET NULL,
			rule_revision INTEGER NOT NULL CHECK (rule_revision > 0),
			rule_name TEXT NOT NULL CHECK (rule_name <> '' AND length(rule_name) <= 300),
			proposed_category TEXT NOT NULL CHECK (proposed_category IN ('PHOTO','SELFIE')),
			matched_subject TEXT NOT NULL CHECK (matched_subject IN ('PARENT_FOLDER','FILE_NAME','FILE_STEM','RELATIVE_PATH')),
			matched_value TEXT NOT NULL CHECK (matched_value <> '' AND length(matched_value) <= 4096),
			status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','ACCEPTED','REJECTED','SUPERSEDED')),
			created_at_utc TEXT NOT NULL,
			resolved_at_utc TEXT,
			UNIQUE(item_uuid,rule_id,rule_revision)
		);
		CREATE INDEX media_classification_suggestions_queue
			ON media_classification_suggestions(gallery_id,status,id);

		INSERT INTO media_classification_rules (
			library_id,name,enabled,sort_order,match_subject,match_operator,pattern,
			case_sensitive,result_category,revision,system_default,default_key,created_at_utc,updated_at_utc
		) VALUES (
			NULL,'Default selfie folders',1,100,'PARENT_FOLDER','EXACT',
			'selfie
selfies
self-portrait
自拍
自拍照
自拍写真
自撮り
			セルフィー
셀카',0,'SELFIE',1,1,'DEFAULT_FOLDER',
			strftime('%Y-%m-%dT%H:%M:%fZ','now'),strftime('%Y-%m-%dT%H:%M:%fZ','now')
		);
		INSERT INTO media_classification_rules (
			library_id,name,enabled,sort_order,match_subject,match_operator,pattern,
			case_sensitive,result_category,revision,system_default,default_key,created_at_utc,updated_at_utc
		) VALUES (
			NULL,'Optional selfie filenames',0,200,'FILE_STEM','GLOB',
			'selfie_*
*_selfie
自拍_*
*_自拍',0,'SELFIE',1,1,'OPTIONAL_FILENAME',
			strftime('%Y-%m-%dT%H:%M:%fZ','now'),strftime('%Y-%m-%dT%H:%M:%fZ','now')
		);

		INSERT INTO media_classification_suggestions (
			gallery_id,item_uuid,rule_id,rule_revision,rule_name,proposed_category,
			matched_subject,matched_value,status,created_at_utc,resolved_at_utc
		)
		SELECT legacy.gallery_id,legacy.item_uuid,rule.id,rule.revision,rule.name,'SELFIE',
			'PARENT_FOLDER','legacy directory semantic',legacy.status,legacy.created_at_utc,legacy.resolved_at_utc
		FROM gallery_item_suggestions legacy
		JOIN media_classification_rules rule ON rule.default_key='DEFAULT_FOLDER'
		WHERE legacy.suggestion_kind='SELFIE_CATEGORY' AND legacy.source_kind='DIRECTORY_SEMANTIC'
		ON CONFLICT(item_uuid,rule_id,rule_revision) DO NOTHING;
	`); err != nil {
		return fmt.Errorf("creating media classification schema version 4: %w", err)
	}
	return nil
}

func validateMediaClassificationSchemaV4(ctx context.Context, db *sql.DB) error {
	for _, table := range requiredMediaClassificationSchemaV4Tables {
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
