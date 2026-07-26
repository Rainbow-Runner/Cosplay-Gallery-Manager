package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredCoreEntitySchemaV1Tables = []string{
	"characters",
	"character_aliases",
	"cosers",
	"coser_aliases",
	"coser_manifest_conflicts",
	"coser_manifest_sync",
	"coser_social_accounts",
	"slug_redirects",
	"tag_aliases",
	"tag_edges",
	"tags",
	"work_aliases",
	"works",
}

var requiredCoreEntitySchemaV1Triggers = []string{
	"characters_uuid_kind",
	"coser_social_accounts_uuid_kind",
	"cosers_uuid_kind",
	"tag_edges_no_cycle",
	"tags_uuid_kind",
	"works_uuid_kind",
}

func createCoreEntitySchemaV1(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE cosers (
			uuid TEXT NOT NULL PRIMARY KEY REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			name TEXT NOT NULL CHECK (name <> '' AND length(name) <= 300),
			sort_name TEXT NOT NULL DEFAULT '' CHECK (length(sort_name) <= 300),
			slug TEXT NOT NULL UNIQUE CHECK (slug <> '' AND length(slug) <= 400),
			profile_summary TEXT NOT NULL DEFAULT '' CHECK (length(profile_summary) <= 20000),
			biography TEXT NOT NULL DEFAULT '' CHECK (length(biography) <= 20000),
			country_or_region TEXT NOT NULL DEFAULT '' CHECK (length(country_or_region) <= 100),
			avatar_path TEXT NOT NULL DEFAULT '' CHECK (length(avatar_path) <= 4096),
			banner_path TEXT NOT NULL DEFAULT '' CHECK (length(banner_path) <= 4096),
			avatar_crop_x REAL, avatar_crop_y REAL, avatar_crop_size REAL,
			banner_focal_x REAL, banner_focal_y REAL,
			metadata_revision INTEGER NOT NULL DEFAULT 1 CHECK (metadata_revision > 0),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL,
			CHECK ((avatar_crop_x IS NULL AND avatar_crop_y IS NULL AND avatar_crop_size IS NULL) OR
				(avatar_crop_x BETWEEN 0 AND 1 AND avatar_crop_y BETWEEN 0 AND 1 AND avatar_crop_size > 0 AND avatar_crop_size <= 1)),
			CHECK ((banner_focal_x IS NULL AND banner_focal_y IS NULL) OR
				(banner_focal_x BETWEEN 0 AND 1 AND banner_focal_y BETWEEN 0 AND 1))
		);

		CREATE TABLE coser_aliases (
			coser_uuid TEXT NOT NULL REFERENCES cosers(uuid) ON DELETE CASCADE,
			alias TEXT NOT NULL CHECK (alias <> '' AND length(alias) <= 300),
			normalized_alias TEXT NOT NULL CHECK (normalized_alias <> ''),
			position INTEGER NOT NULL CHECK (position > 0),
			PRIMARY KEY (coser_uuid, normalized_alias),
			UNIQUE (coser_uuid, position)
		);

		CREATE TABLE works (
			uuid TEXT NOT NULL PRIMARY KEY REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			name TEXT NOT NULL CHECK (name <> '' AND length(name) <= 300),
			sort_name TEXT NOT NULL DEFAULT '' CHECK (length(sort_name) <= 300),
			slug TEXT NOT NULL UNIQUE CHECK (slug <> '' AND length(slug) <= 400),
			metadata_revision INTEGER NOT NULL DEFAULT 1 CHECK (metadata_revision > 0),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL
		);

		CREATE TABLE work_aliases (
			work_uuid TEXT NOT NULL REFERENCES works(uuid) ON DELETE CASCADE,
			alias TEXT NOT NULL CHECK (alias <> '' AND length(alias) <= 300),
			normalized_alias TEXT NOT NULL CHECK (normalized_alias <> ''),
			position INTEGER NOT NULL CHECK (position > 0),
			PRIMARY KEY (work_uuid, normalized_alias),
			UNIQUE (work_uuid, position)
		);

		CREATE TABLE characters (
			uuid TEXT NOT NULL PRIMARY KEY REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			work_uuid TEXT NOT NULL REFERENCES works(uuid) ON DELETE RESTRICT,
			name TEXT NOT NULL CHECK (name <> '' AND length(name) <= 300),
			normalized_name TEXT NOT NULL CHECK (normalized_name <> ''),
			sort_name TEXT NOT NULL DEFAULT '' CHECK (length(sort_name) <= 300),
			slug TEXT NOT NULL UNIQUE CHECK (slug <> '' AND length(slug) <= 400),
			metadata_revision INTEGER NOT NULL DEFAULT 1 CHECK (metadata_revision > 0),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL,
			UNIQUE (work_uuid, normalized_name)
		);

		CREATE TABLE character_aliases (
			character_uuid TEXT NOT NULL REFERENCES characters(uuid) ON DELETE CASCADE,
			alias TEXT NOT NULL CHECK (alias <> '' AND length(alias) <= 300),
			normalized_alias TEXT NOT NULL CHECK (normalized_alias <> ''),
			position INTEGER NOT NULL CHECK (position > 0),
			PRIMARY KEY (character_uuid, normalized_alias),
			UNIQUE (character_uuid, position)
		);

		CREATE TABLE tags (
			uuid TEXT NOT NULL PRIMARY KEY REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			name TEXT NOT NULL CHECK (name <> '' AND length(name) <= 300),
			normalized_name TEXT NOT NULL UNIQUE CHECK (normalized_name <> ''),
			sort_name TEXT NOT NULL DEFAULT '' CHECK (length(sort_name) <= 300),
			slug TEXT NOT NULL UNIQUE CHECK (slug <> '' AND length(slug) <= 400),
			use_in_recommendation INTEGER NOT NULL DEFAULT 1 CHECK (use_in_recommendation IN (0, 1)),
			metadata_revision INTEGER NOT NULL DEFAULT 1 CHECK (metadata_revision > 0),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL
		);

		CREATE TABLE tag_aliases (
			tag_uuid TEXT NOT NULL REFERENCES tags(uuid) ON DELETE CASCADE,
			alias TEXT NOT NULL CHECK (alias <> '' AND length(alias) <= 300),
			normalized_alias TEXT NOT NULL UNIQUE CHECK (normalized_alias <> ''),
			position INTEGER NOT NULL CHECK (position > 0),
			UNIQUE (tag_uuid, position)
		);

		CREATE TABLE tag_edges (
			parent_uuid TEXT NOT NULL REFERENCES tags(uuid) ON DELETE CASCADE,
			child_uuid TEXT NOT NULL REFERENCES tags(uuid) ON DELETE CASCADE,
			position INTEGER NOT NULL CHECK (position > 0),
			PRIMARY KEY (parent_uuid, child_uuid),
			UNIQUE (child_uuid, position),
			CHECK (parent_uuid <> child_uuid)
		);

		CREATE TABLE coser_social_accounts (
			account_uuid TEXT NOT NULL PRIMARY KEY REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			coser_uuid TEXT NOT NULL REFERENCES cosers(uuid) ON DELETE CASCADE,
			platform_key TEXT NOT NULL CHECK (platform_key <> '' AND length(platform_key) <= 64),
			label TEXT NOT NULL DEFAULT '' CHECK (length(label) <= 100),
			handle TEXT NOT NULL DEFAULT '' CHECK (length(handle) <= 200),
			url TEXT NOT NULL CHECK (url <> '' AND length(url) <= 2048),
			status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'INACTIVE')),
			visible INTEGER NOT NULL DEFAULT 1 CHECK (visible IN (0, 1)),
			position INTEGER NOT NULL CHECK (position > 0),
			UNIQUE (coser_uuid, position)
		);

		CREATE TABLE coser_manifest_sync (
			coser_uuid TEXT NOT NULL PRIMARY KEY REFERENCES cosers(uuid) ON DELETE CASCADE,
			manifest_path TEXT NOT NULL CHECK (manifest_path <> '' AND length(manifest_path) <= 4096),
			status TEXT NOT NULL CHECK (status IN ('CLEAN', 'DB_DIRTY', 'FILE_DIRTY', 'CONFLICT', 'MISSING', 'ERROR')),
			schema_version INTEGER NOT NULL CHECK (schema_version > 0),
			manifest_revision INTEGER NOT NULL CHECK (manifest_revision >= 0),
			file_hash TEXT NOT NULL CHECK (file_hash <> '' AND length(file_hash) <= 200),
			baseline_json BLOB NOT NULL CHECK (length(baseline_json) <= 2097152),
			baseline_metadata_revision INTEGER NOT NULL CHECK (baseline_metadata_revision > 0),
			checked_at_utc TEXT NOT NULL,
			last_error_code TEXT NOT NULL DEFAULT '' CHECK (length(last_error_code) <= 100)
		);

		CREATE TABLE coser_manifest_conflicts (
			id INTEGER NOT NULL PRIMARY KEY,
			coser_uuid TEXT NOT NULL REFERENCES cosers(uuid) ON DELETE CASCADE,
			field_path TEXT NOT NULL CHECK (field_path <> '' AND length(field_path) <= 1000),
			baseline_json BLOB, database_json BLOB, file_json BLOB,
			created_at_utc TEXT NOT NULL,
			UNIQUE (coser_uuid, field_path)
		);

		CREATE TABLE slug_redirects (
			entity_kind TEXT NOT NULL CHECK (entity_kind IN ('GALLERY', 'COSER', 'WORK', 'CHARACTER', 'TAG')),
			old_slug TEXT NOT NULL CHECK (old_slug <> '' AND length(old_slug) <= 400),
			target_uuid TEXT NOT NULL REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			created_at_utc TEXT NOT NULL,
			PRIMARY KEY (entity_kind, old_slug)
		);

		CREATE TRIGGER cosers_uuid_kind BEFORE INSERT ON cosers BEGIN
			SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.uuid AND entity_kind = 'COSER')
			THEN RAISE(ABORT, 'Coser UUID kind mismatch') END;
		END;
		CREATE TRIGGER works_uuid_kind BEFORE INSERT ON works BEGIN
			SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.uuid AND entity_kind = 'WORK')
			THEN RAISE(ABORT, 'Work UUID kind mismatch') END;
		END;
		CREATE TRIGGER characters_uuid_kind BEFORE INSERT ON characters BEGIN
			SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.uuid AND entity_kind = 'CHARACTER')
			THEN RAISE(ABORT, 'Character UUID kind mismatch') END;
		END;
		CREATE TRIGGER tags_uuid_kind BEFORE INSERT ON tags BEGIN
			SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.uuid AND entity_kind = 'TAG')
			THEN RAISE(ABORT, 'Tag UUID kind mismatch') END;
		END;
		CREATE TRIGGER coser_social_accounts_uuid_kind BEFORE INSERT ON coser_social_accounts BEGIN
			SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.account_uuid AND entity_kind = 'SOCIAL_ACCOUNT')
			THEN RAISE(ABORT, 'SocialAccount UUID kind mismatch') END;
		END;

		CREATE TRIGGER tag_edges_no_cycle BEFORE INSERT ON tag_edges BEGIN
			SELECT CASE WHEN NEW.parent_uuid = NEW.child_uuid OR EXISTS (
				WITH RECURSIVE descendants(uuid) AS (
					SELECT child_uuid FROM tag_edges WHERE parent_uuid = NEW.child_uuid
					UNION
					SELECT edge.child_uuid FROM tag_edges edge JOIN descendants d ON edge.parent_uuid = d.uuid
				)
				SELECT 1 FROM descendants WHERE uuid = NEW.parent_uuid
			) THEN RAISE(ABORT, 'Tag DAG cycle detected') END;
			SELECT CASE WHEN (SELECT COUNT(*) FROM tag_edges WHERE child_uuid = NEW.child_uuid) >= 50
			THEN RAISE(ABORT, 'Tag direct parent limit exceeded') END;
		END;
	`); err != nil {
		return fmt.Errorf("creating core entity schema version 1: %w", err)
	}
	return nil
}
