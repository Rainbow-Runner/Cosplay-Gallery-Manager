package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredGallerySchemaV1Tables = []string{
	"discovery_snapshots",
	"galleries",
	"gallery_candidate_suggestions",
	"gallery_candidates",
	"gallery_cast",
	"gallery_credits",
	"gallery_identity_suggestions",
	"gallery_aliases",
	"gallery_external_links",
	"gallery_item_personal_states",
	"gallery_item_suggestions",
	"gallery_items",
	"gallery_metadata_suggestions",
	"gallery_manifest_conflicts",
	"gallery_manifest_sync",
	"gallery_personal_states",
	"gallery_source_issues",
	"gallery_sources",
	"gallery_tags",
	"gallery_recognition_rules",
	"gallery_scan_observations",
	"gallery_scan_runs",
	"ignored_gallery_sources",
	"media_libraries",
	"unassigned_media_diagnostics",
}

var requiredGallerySchemaV1Triggers = []string{
	"galleries_set_id_kind",
	"galleries_set_id_immutable",
	"gallery_cast_character_kind",
	"gallery_credits_coser_kind",
	"gallery_items_aggregate_immutable",
	"gallery_items_hard_limit_insert",
	"gallery_items_hard_limit_restore",
	"gallery_items_uuid_kind",
	"gallery_personal_last_item_insert",
	"gallery_personal_last_item_update",
	"gallery_sources_gallery_immutable",
	"gallery_external_links_uuid_kind",
}

func createGallerySchemaV1(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE media_libraries (
			id INTEGER NOT NULL PRIMARY KEY,
			name TEXT NOT NULL CHECK (name <> '' AND length(name) <= 300),
			root_path TEXT NOT NULL UNIQUE CHECK (root_path <> '' AND length(root_path) <= 4096),
			enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
			read_only INTEGER NOT NULL DEFAULT 0 CHECK (read_only IN (0, 1)),
			capture_timezone TEXT NOT NULL DEFAULT 'UTC' CHECK (capture_timezone <> '' AND length(capture_timezone) <= 100),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL
		);

		CREATE TABLE gallery_recognition_rules (
			id INTEGER NOT NULL PRIMARY KEY,
			library_id INTEGER NOT NULL REFERENCES media_libraries(id) ON DELETE CASCADE,
			name TEXT NOT NULL CHECK (name <> '' AND length(name) <= 300),
			rule_kind TEXT NOT NULL CHECK (rule_kind IN ('MARKER', 'PATH_TEMPLATE', 'FIXED_DEPTH')),
			enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
			auto_create_draft INTEGER NOT NULL DEFAULT 0 CHECK (auto_create_draft IN (0, 1)),
			sort_order INTEGER NOT NULL DEFAULT 0,
			pattern TEXT,
			fixed_depth INTEGER,
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL,
			CHECK (
				(rule_kind = 'MARKER' AND pattern IS NULL AND fixed_depth IS NULL) OR
				(rule_kind = 'PATH_TEMPLATE' AND pattern IS NOT NULL AND fixed_depth IS NULL) OR
				(rule_kind = 'FIXED_DEPTH' AND pattern IS NULL AND fixed_depth BETWEEN 1 AND 64)
			)
		);

		CREATE TABLE ignored_gallery_sources (
			id INTEGER NOT NULL PRIMARY KEY,
			library_id INTEGER REFERENCES media_libraries(id) ON DELETE CASCADE,
			set_id TEXT,
			source_path TEXT NOT NULL UNIQUE CHECK (source_path <> '' AND length(source_path) <= 4096),
			reason TEXT NOT NULL DEFAULT '' CHECK (length(reason) <= 1000),
			created_at_utc TEXT NOT NULL
		);
		CREATE UNIQUE INDEX ignored_gallery_sources_set_id_unique
			ON ignored_gallery_sources(set_id) WHERE set_id IS NOT NULL;

		CREATE TABLE discovery_snapshots (
			id INTEGER NOT NULL PRIMARY KEY,
			library_id INTEGER NOT NULL REFERENCES media_libraries(id) ON DELETE CASCADE,
			completed_at_utc TEXT NOT NULL
		);

		CREATE TABLE gallery_candidates (
			id INTEGER NOT NULL PRIMARY KEY,
			snapshot_id INTEGER NOT NULL REFERENCES discovery_snapshots(id) ON DELETE CASCADE,
			library_id INTEGER NOT NULL REFERENCES media_libraries(id) ON DELETE CASCADE,
			gallery_id INTEGER REFERENCES galleries(id) ON DELETE SET NULL,
			manifest_set_id TEXT CHECK (manifest_set_id IS NULL OR length(manifest_set_id) = 36),
			rebind_gallery_id INTEGER REFERENCES galleries(id) ON DELETE SET NULL,
			root_path TEXT NOT NULL CHECK (root_path <> '' AND length(root_path) <= 4096),
			source_type TEXT NOT NULL CHECK (source_type IN ('DIRECTORY', 'ARCHIVE')),
			recognition_method TEXT NOT NULL
				CHECK (recognition_method IN ('MANIFEST', 'MARKER', 'PATH_TEMPLATE', 'FIXED_DEPTH')),
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

		CREATE TABLE gallery_candidate_suggestions (
			id INTEGER NOT NULL PRIMARY KEY,
			candidate_id INTEGER NOT NULL REFERENCES gallery_candidates(id) ON DELETE CASCADE,
			field_name TEXT NOT NULL
				CHECK (field_name IN ('title', 'coser', 'work', 'character', 'year', 'month')),
			value TEXT NOT NULL CHECK (value <> '' AND length(value) <= 300),
			status TEXT NOT NULL DEFAULT 'PENDING'
				CHECK (status IN ('PENDING', 'ACCEPTED', 'REJECTED')),
			UNIQUE (candidate_id, field_name, value)
		);

		CREATE TABLE unassigned_media_diagnostics (
			id INTEGER NOT NULL PRIMARY KEY,
			snapshot_id INTEGER NOT NULL REFERENCES discovery_snapshots(id) ON DELETE CASCADE,
			library_id INTEGER NOT NULL REFERENCES media_libraries(id) ON DELETE CASCADE,
			parent_path TEXT NOT NULL CHECK (parent_path <> '' AND length(parent_path) <= 4096),
			media_count INTEGER NOT NULL CHECK (media_count > 0),
			UNIQUE (snapshot_id, parent_path)
		);

		CREATE TABLE galleries (
			id INTEGER NOT NULL PRIMARY KEY,
			set_id TEXT NOT NULL UNIQUE
				REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			slug TEXT NOT NULL UNIQUE CHECK (slug <> '' AND length(slug) <= 400),
			state TEXT NOT NULL CHECK (state IN ('DRAFT', 'ACTIVE', 'ARCHIVED')),
			title TEXT NOT NULL DEFAULT '' CHECK (length(title) <= 300),
			description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 20000),
			shoot_date TEXT,
			shoot_date_precision TEXT CHECK (shoot_date_precision IN ('MONTH', 'DAY')),
			content_rating TEXT CHECK (content_rating IN ('NON_ADULT', 'ADULT')),
			photographer_name TEXT NOT NULL DEFAULT '' CHECK (length(photographer_name) <= 200),
			studio_name TEXT NOT NULL DEFAULT '' CHECK (length(studio_name) <= 200),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL,
			added_at_utc TEXT,
			metadata_revision INTEGER NOT NULL DEFAULT 1 CHECK (metadata_revision > 0),
			scan_revision INTEGER NOT NULL DEFAULT 0 CHECK (scan_revision >= 0),
			scrubber_revision INTEGER NOT NULL DEFAULT 0 CHECK (scrubber_revision >= 0),
			CHECK (
				(shoot_date IS NULL AND shoot_date_precision IS NULL) OR
				(shoot_date IS NOT NULL AND shoot_date_precision IS NOT NULL)
			),
			CHECK (state <> 'ACTIVE' OR added_at_utc IS NOT NULL)
		);

		CREATE TABLE gallery_sources (
			id INTEGER NOT NULL PRIMARY KEY,
			gallery_id INTEGER NOT NULL UNIQUE
				REFERENCES galleries(id) ON DELETE CASCADE,
			library_id INTEGER REFERENCES media_libraries(id) ON DELETE SET NULL,
			source_type TEXT NOT NULL CHECK (source_type IN ('DIRECTORY', 'ARCHIVE')),
			source_path TEXT NOT NULL UNIQUE CHECK (source_path <> '' AND length(source_path) <= 4096),
			availability_state TEXT NOT NULL
				CHECK (availability_state IN ('AVAILABLE', 'MISSING', 'UNREADABLE')),
			reconcile_state TEXT NOT NULL
				CHECK (reconcile_state IN ('NEVER_SCANNED', 'SCANNING', 'IN_SYNC', 'NEEDS_RESCAN', 'ERROR')),
			over_limit INTEGER NOT NULL DEFAULT 0 CHECK (over_limit IN (0, 1)),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL,
			UNIQUE (id, gallery_id)
		);

		CREATE TABLE gallery_aliases (
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			alias TEXT NOT NULL CHECK (alias <> '' AND length(alias) <= 300),
			normalized_alias TEXT NOT NULL CHECK (normalized_alias <> ''),
			position INTEGER NOT NULL CHECK (position > 0),
			PRIMARY KEY (gallery_id, normalized_alias),
			UNIQUE (gallery_id, position)
		);

		CREATE TABLE gallery_tags (
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			tag_uuid TEXT NOT NULL REFERENCES tags(uuid) ON DELETE RESTRICT,
			position INTEGER NOT NULL CHECK (position > 0),
			PRIMARY KEY (gallery_id, tag_uuid),
			UNIQUE (gallery_id, position)
		);

		CREATE TABLE gallery_external_links (
			link_uuid TEXT NOT NULL PRIMARY KEY REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			link_type TEXT NOT NULL CHECK (link_type IN ('SOURCE', 'PROFILE', 'REFERENCE')),
			label TEXT NOT NULL DEFAULT '' CHECK (length(label) <= 100),
			url TEXT NOT NULL CHECK (url <> '' AND length(url) <= 2048 AND
				(url LIKE 'http://%' OR url LIKE 'https://%')),
			position INTEGER NOT NULL CHECK (position > 0),
			UNIQUE (gallery_id, position),
			UNIQUE (gallery_id, url)
		);

		CREATE TABLE gallery_manifest_sync (
			gallery_id INTEGER NOT NULL PRIMARY KEY REFERENCES galleries(id) ON DELETE CASCADE,
			manifest_path TEXT NOT NULL CHECK (manifest_path <> '' AND length(manifest_path) <= 4096),
			status TEXT NOT NULL CHECK (status IN ('CLEAN', 'DB_DIRTY', 'FILE_DIRTY', 'CONFLICT', 'MISSING', 'ERROR')),
			schema_version INTEGER NOT NULL CHECK (schema_version > 0),
			manifest_revision INTEGER NOT NULL CHECK (manifest_revision >= 0),
			file_hash TEXT NOT NULL CHECK (file_hash <> '' AND length(file_hash) <= 200),
			baseline_json BLOB NOT NULL CHECK (length(baseline_json) <= 16777216),
			baseline_metadata_revision INTEGER NOT NULL CHECK (baseline_metadata_revision > 0),
			checked_at_utc TEXT NOT NULL,
			last_error_code TEXT NOT NULL DEFAULT '' CHECK (length(last_error_code) <= 100)
		);

		CREATE TABLE gallery_manifest_conflicts (
			id INTEGER NOT NULL PRIMARY KEY,
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			field_path TEXT NOT NULL CHECK (field_path <> '' AND length(field_path) <= 1000),
			baseline_json BLOB,
			database_json BLOB,
			file_json BLOB,
			created_at_utc TEXT NOT NULL,
			UNIQUE (gallery_id, field_path)
		);

		CREATE TABLE gallery_source_issues (
			id INTEGER NOT NULL PRIMARY KEY,
			source_id INTEGER NOT NULL REFERENCES gallery_sources(id) ON DELETE CASCADE,
			code TEXT NOT NULL CHECK (code <> '' AND length(code) <= 100),
			severity TEXT NOT NULL CHECK (severity IN ('INFO', 'WARNING', 'BLOCKING')),
			message TEXT NOT NULL DEFAULT '' CHECK (length(message) <= 2000),
			created_at_utc TEXT NOT NULL,
			resolved_at_utc TEXT,
			UNIQUE (source_id, code)
		);

		CREATE TABLE gallery_scan_runs (
			id INTEGER NOT NULL PRIMARY KEY,
			source_id INTEGER NOT NULL REFERENCES gallery_sources(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK (status IN ('STAGING', 'COMPLETED', 'FAILED', 'CANCELLED')),
			started_at_utc TEXT NOT NULL,
			completed_at_utc TEXT,
			error_code TEXT NOT NULL DEFAULT '' CHECK (length(error_code) <= 100)
		);

		CREATE TABLE gallery_scan_observations (
			id INTEGER NOT NULL PRIMARY KEY,
			scan_run_id INTEGER NOT NULL REFERENCES gallery_scan_runs(id) ON DELETE CASCADE,
			relative_path TEXT NOT NULL CHECK (
				relative_path <> '' AND length(relative_path) <= 4096 AND
				relative_path NOT LIKE '/%' AND instr(relative_path, '\\') = 0
			),
			media_kind TEXT NOT NULL CHECK (media_kind IN ('STATIC_IMAGE', 'ANIMATED_IMAGE', 'VIDEO')),
			content_format TEXT NOT NULL DEFAULT 'IMAGE' CHECK (content_format IN ('IMAGE','RAW','VIDEO')),
			image_category TEXT CHECK (image_category IN ('PHOTO', 'SELFIE')),
			byte_size INTEGER NOT NULL DEFAULT 0 CHECK (byte_size >= 0),
			quick_fingerprint TEXT NOT NULL DEFAULT '' CHECK (length(quick_fingerprint) <= 200),
			full_fingerprint TEXT NOT NULL DEFAULT '' CHECK (length(full_fingerprint) <= 200),
			processing_state TEXT NOT NULL CHECK (processing_state IN ('PENDING', 'READY', 'ERROR')),
			UNIQUE (scan_run_id, relative_path),
			CHECK (
				(media_kind = 'STATIC_IMAGE' AND image_category IS NOT NULL) OR
				(media_kind <> 'STATIC_IMAGE' AND image_category IS NULL)
			),
			CHECK ((content_format='RAW' AND media_kind='STATIC_IMAGE') OR
				(content_format='VIDEO' AND media_kind='VIDEO') OR
				(content_format='IMAGE' AND media_kind IN ('STATIC_IMAGE','ANIMATED_IMAGE')))
		);

		CREATE TABLE gallery_items (
			id INTEGER NOT NULL PRIMARY KEY,
			item_uuid TEXT NOT NULL UNIQUE
				REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			source_id INTEGER NOT NULL,
			relative_path TEXT NOT NULL CHECK (
				relative_path <> '' AND length(relative_path) <= 4096 AND
				relative_path NOT LIKE '/%' AND
				relative_path NOT LIKE '../%' AND
				relative_path NOT LIKE '%/../%' AND
				relative_path NOT LIKE '%/..' AND
				instr(relative_path, '\\') = 0
			),
			media_kind TEXT NOT NULL
				CHECK (media_kind IN ('STATIC_IMAGE', 'ANIMATED_IMAGE', 'VIDEO')),
			content_format TEXT NOT NULL DEFAULT 'IMAGE' CHECK (content_format IN ('IMAGE','RAW','VIDEO')),
			image_category TEXT CHECK (image_category IN ('PHOTO', 'SELFIE')),
			position INTEGER NOT NULL CHECK (position > 0),
			caption TEXT NOT NULL DEFAULT '' CHECK (length(caption) <= 1000),
			excluded INTEGER NOT NULL DEFAULT 0 CHECK (excluded IN (0, 1)),
			availability_state TEXT NOT NULL
				CHECK (availability_state IN ('AVAILABLE', 'MISSING', 'UNREADABLE')),
			processing_state TEXT NOT NULL
				CHECK (processing_state IN ('PENDING', 'PROCESSING', 'READY', 'ERROR')),
			byte_size INTEGER NOT NULL DEFAULT 0 CHECK (byte_size >= 0),
			quick_fingerprint TEXT NOT NULL DEFAULT '' CHECK (length(quick_fingerprint) <= 200),
			full_fingerprint TEXT NOT NULL DEFAULT '' CHECK (length(full_fingerprint) <= 200),
			content_revision INTEGER NOT NULL DEFAULT 1 CHECK (content_revision > 0),
			last_seen_scan_run_id INTEGER REFERENCES gallery_scan_runs(id) ON DELETE SET NULL,
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL,
			FOREIGN KEY (source_id, gallery_id)
				REFERENCES gallery_sources(id, gallery_id) ON DELETE CASCADE,
			UNIQUE (source_id, relative_path),
			UNIQUE (gallery_id, position),
			UNIQUE (id, gallery_id),
			CHECK (
				(media_kind = 'STATIC_IMAGE' AND image_category IS NOT NULL) OR
				(media_kind <> 'STATIC_IMAGE' AND image_category IS NULL)
			),
			CHECK ((content_format='RAW' AND media_kind='STATIC_IMAGE') OR
				(content_format='VIDEO' AND media_kind='VIDEO') OR
				(content_format='IMAGE' AND media_kind IN ('STATIC_IMAGE','ANIMATED_IMAGE')))
		);
		CREATE INDEX gallery_items_browse_counts ON gallery_items(gallery_id,excluded,availability_state,media_kind,image_category);

		CREATE TABLE gallery_credits (
			id INTEGER NOT NULL PRIMARY KEY,
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			coser_uuid TEXT NOT NULL REFERENCES cosers(uuid) ON DELETE RESTRICT,
			position INTEGER NOT NULL CHECK (position > 0),
			UNIQUE (gallery_id, coser_uuid),
			UNIQUE (gallery_id, position),
			UNIQUE (id, gallery_id)
		);

		CREATE TABLE gallery_cast (
			id INTEGER NOT NULL PRIMARY KEY,
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			gallery_credit_id INTEGER NOT NULL,
			character_uuid TEXT NOT NULL REFERENCES characters(uuid) ON DELETE RESTRICT,
			position INTEGER NOT NULL CHECK (position > 0),
			FOREIGN KEY (gallery_credit_id, gallery_id)
				REFERENCES gallery_credits(id, gallery_id) ON DELETE CASCADE,
			UNIQUE (gallery_id, gallery_credit_id, character_uuid),
			UNIQUE (gallery_credit_id, position)
		);

		CREATE TABLE gallery_identity_suggestions (
			id INTEGER NOT NULL PRIMARY KEY,
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			suggestion_kind TEXT NOT NULL CHECK (suggestion_kind IN ('COSER', 'WORK', 'CHARACTER')),
			value TEXT NOT NULL CHECK (value <> '' AND length(value) <= 300),
			status TEXT NOT NULL CHECK (status IN ('PENDING', 'ACCEPTED', 'REJECTED')),
			created_at_utc TEXT NOT NULL,
			resolved_at_utc TEXT
		);

		CREATE TABLE gallery_metadata_suggestions (
			id INTEGER NOT NULL PRIMARY KEY,
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			suggestion_kind TEXT NOT NULL CHECK (suggestion_kind IN ('TITLE', 'YEAR', 'MONTH')),
			value TEXT NOT NULL CHECK (value <> '' AND length(value) <= 300),
			status TEXT NOT NULL CHECK (status IN ('PENDING', 'ACCEPTED', 'REJECTED')),
			created_at_utc TEXT NOT NULL,
			resolved_at_utc TEXT
		);

		CREATE TABLE gallery_item_suggestions (
			id INTEGER NOT NULL PRIMARY KEY,
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			item_uuid TEXT NOT NULL REFERENCES gallery_items(item_uuid) ON DELETE CASCADE,
			suggestion_kind TEXT NOT NULL CHECK (suggestion_kind IN ('SELFIE_CATEGORY','CAPTURE_DATE','RAW_COMPANION')),
			value TEXT NOT NULL CHECK (value <> '' AND length(value) <= 4096),
			source_kind TEXT NOT NULL CHECK (source_kind IN ('DIRECTORY_SEMANTIC','EXIF','XMP','RAW_COMPANION')),
			status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','ACCEPTED','REJECTED')),
			created_at_utc TEXT NOT NULL,
			resolved_at_utc TEXT,
			UNIQUE(item_uuid,suggestion_kind,value)
		);
		CREATE INDEX gallery_item_suggestions_queue ON gallery_item_suggestions(gallery_id,status,suggestion_kind,id);

		CREATE TABLE gallery_personal_states (
			gallery_id INTEGER NOT NULL PRIMARY KEY REFERENCES galleries(id) ON DELETE CASCADE,
			favorite INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0, 1)),
			favorited_at_utc TEXT,
			rating_half_steps INTEGER CHECK (rating_half_steps BETWEEN 1 AND 10),
			rated_at_utc TEXT,
			hidden INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1)),
			last_viewed_at_utc TEXT,
			last_item_id INTEGER REFERENCES gallery_items(id) ON DELETE SET NULL,
			CHECK ((favorite = 1 AND favorited_at_utc IS NOT NULL) OR
				(favorite = 0 AND favorited_at_utc IS NULL)),
			CHECK ((rating_half_steps IS NULL AND rated_at_utc IS NULL) OR
				(rating_half_steps IS NOT NULL AND rated_at_utc IS NOT NULL))
		);

		CREATE TABLE gallery_item_personal_states (
			gallery_item_id INTEGER NOT NULL PRIMARY KEY REFERENCES gallery_items(id) ON DELETE CASCADE,
			favorite INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0, 1)),
			favorited_at_utc TEXT,
			rating_half_steps INTEGER CHECK (rating_half_steps BETWEEN 1 AND 10),
			rated_at_utc TEXT,
			CHECK ((favorite = 1 AND favorited_at_utc IS NOT NULL) OR
				(favorite = 0 AND favorited_at_utc IS NULL)),
			CHECK ((rating_half_steps IS NULL AND rated_at_utc IS NULL) OR
				(rating_half_steps IS NOT NULL AND rated_at_utc IS NOT NULL))
		);

		CREATE TRIGGER galleries_set_id_kind
		BEFORE INSERT ON galleries
		BEGIN
			SELECT CASE WHEN NOT EXISTS (
				SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.set_id AND entity_kind = 'GALLERY'
			) THEN RAISE(ABORT, 'Gallery set_id must be a registered Gallery UUID') END;
		END;

		CREATE TRIGGER galleries_set_id_immutable
		BEFORE UPDATE OF set_id ON galleries
		BEGIN
			SELECT RAISE(ABORT, 'Gallery set_id is immutable');
		END;

		CREATE TRIGGER gallery_sources_gallery_immutable
		BEFORE UPDATE OF gallery_id ON gallery_sources
		BEGIN
			SELECT RAISE(ABORT, 'GallerySource cannot move between Galleries implicitly');
		END;

		CREATE TRIGGER gallery_external_links_uuid_kind
		BEFORE INSERT ON gallery_external_links
		BEGIN
			SELECT CASE WHEN NOT EXISTS (
				SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.link_uuid AND entity_kind = 'EXTERNAL_LINK'
			) THEN RAISE(ABORT, 'ExternalLink UUID kind mismatch') END;
		END;

		CREATE TRIGGER gallery_items_uuid_kind
		BEFORE INSERT ON gallery_items
		BEGIN
			SELECT CASE WHEN NOT EXISTS (
				SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.item_uuid AND entity_kind = 'GALLERY_ITEM'
			) THEN RAISE(ABORT, 'item_uuid must be a registered GalleryItem UUID') END;
		END;

		CREATE TRIGGER gallery_items_aggregate_immutable
		BEFORE UPDATE OF item_uuid, gallery_id, source_id ON gallery_items
		BEGIN
			SELECT RAISE(ABORT, 'GalleryItem aggregate identity is immutable');
		END;

		CREATE TRIGGER gallery_items_hard_limit_insert
		BEFORE INSERT ON gallery_items
		WHEN NEW.excluded = 0
		BEGIN
			SELECT CASE WHEN (
				SELECT COUNT(*) FROM gallery_items
				WHERE gallery_id = NEW.gallery_id AND excluded = 0
			) >= 1000 THEN RAISE(ABORT, 'Gallery non-excluded member hard limit exceeded') END;
		END;

		CREATE TRIGGER gallery_items_hard_limit_restore
		BEFORE UPDATE OF excluded ON gallery_items
		WHEN OLD.excluded = 1 AND NEW.excluded = 0
		BEGIN
			SELECT CASE WHEN (
				SELECT COUNT(*) FROM gallery_items
				WHERE gallery_id = NEW.gallery_id AND excluded = 0
			) >= 1000 THEN RAISE(ABORT, 'Gallery non-excluded member hard limit exceeded') END;
		END;

		CREATE TRIGGER gallery_credits_coser_kind
		BEFORE INSERT ON gallery_credits
		BEGIN
			SELECT CASE WHEN NOT EXISTS (
				SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.coser_uuid AND entity_kind = 'COSER'
			) THEN RAISE(ABORT, 'GalleryCredit must reference a registered Coser UUID') END;
		END;

		CREATE TRIGGER gallery_cast_character_kind
		BEFORE INSERT ON gallery_cast
		BEGIN
			SELECT CASE WHEN NOT EXISTS (
				SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.character_uuid AND entity_kind = 'CHARACTER'
			) THEN RAISE(ABORT, 'GalleryCast must reference a registered Character UUID') END;
		END;

		CREATE TRIGGER gallery_personal_last_item_insert
		BEFORE INSERT ON gallery_personal_states
		WHEN NEW.last_item_id IS NOT NULL
		BEGIN
			SELECT CASE WHEN NOT EXISTS (
				SELECT 1 FROM gallery_items
				WHERE id = NEW.last_item_id AND gallery_id = NEW.gallery_id
			) THEN RAISE(ABORT, 'last_item_id must belong to the same Gallery') END;
		END;

		CREATE TRIGGER gallery_personal_last_item_update
		BEFORE UPDATE OF last_item_id ON gallery_personal_states
		WHEN NEW.last_item_id IS NOT NULL
		BEGIN
			SELECT CASE WHEN NOT EXISTS (
				SELECT 1 FROM gallery_items
				WHERE id = NEW.last_item_id AND gallery_id = NEW.gallery_id
			) THEN RAISE(ABORT, 'last_item_id must belong to the same Gallery') END;
		END
	`); err != nil {
		return fmt.Errorf("creating Gallery schema version 1: %w", err)
	}

	return nil
}
