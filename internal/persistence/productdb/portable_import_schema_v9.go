package productdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

var requiredPortableImportSchemaV9Tables = []string{"portable_import_sessions", "portable_identity_claims", "portable_library_mappings", "portable_gallery_rebuilds"}
var requiredPortableImportSchemaV9Triggers = []string{
	"portable_identity_claim_fields_immutable",
	"portable_identity_claim_no_delete",
	"portable_identity_claim_transition",
	"portable_uuid_registry_pending_claim_guard",
}

// Schema v9 persists resumable portable-import technical state and reserves
// Gallery/Item/Link UUIDs without creating orphan ACTIVE registry records.
func createPortableImportSchemaV9(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE maintenance_state RENAME TO maintenance_state_v8;
		CREATE TABLE maintenance_state (
			id INTEGER NOT NULL PRIMARY KEY CHECK(id=1),
			state TEXT NOT NULL CHECK(state IN ('NORMAL','RESTORING','WAITING_VALIDATION','PORTABLE_IMPORTING')),
			restore_backup_id TEXT,
			last_error_code TEXT NOT NULL DEFAULT '' CHECK(length(last_error_code)<=100),
			updated_at_utc TEXT NOT NULL
		);
		INSERT INTO maintenance_state SELECT * FROM maintenance_state_v8;
		DROP TABLE maintenance_state_v8;

		CREATE TABLE portable_import_sessions (
			import_id TEXT NOT NULL PRIMARY KEY CHECK(import_id=lower(import_id) AND length(import_id)=36),
			export_id TEXT NOT NULL CHECK(export_id=lower(export_id) AND length(export_id)=36),
			package_sha256 TEXT NOT NULL CHECK(length(package_sha256)=64),
			package_relative_path TEXT NOT NULL UNIQUE CHECK(package_relative_path<>'' AND length(package_relative_path)<=4096),
			format_version INTEGER NOT NULL CHECK(format_version>0),
			state TEXT NOT NULL CHECK(state IN ('INSPECTED','IMPORTING','CORE_IMPORTED','LIBRARIES_MAPPED','GALLERIES_REBUILT','FAILED')),
			identity_count INTEGER NOT NULL CHECK(identity_count>=0),
			core_entity_count INTEGER NOT NULL CHECK(core_entity_count>=0),
			gallery_claim_count INTEGER NOT NULL CHECK(gallery_claim_count>=0),
			item_claim_count INTEGER NOT NULL CHECK(item_claim_count>=0),
			link_claim_count INTEGER NOT NULL CHECK(link_claim_count>=0),
			asset_count INTEGER NOT NULL CHECK(asset_count>=0),
			error_code TEXT NOT NULL DEFAULT '' CHECK(length(error_code)<=100),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL
		);

		CREATE TABLE portable_identity_claims (
			uuid TEXT NOT NULL PRIMARY KEY CHECK(uuid=lower(uuid) AND length(uuid)=36),
			import_id TEXT NOT NULL REFERENCES portable_import_sessions(import_id) ON DELETE RESTRICT,
			entity_kind TEXT NOT NULL CHECK(entity_kind IN ('GALLERY','GALLERY_ITEM','EXTERNAL_LINK')),
			identity_state TEXT NOT NULL CHECK(identity_state IN ('ACTIVE','ALIAS','TOMBSTONE')),
			target_uuid TEXT NOT NULL DEFAULT '' CHECK(length(target_uuid) IN (0,36)),
			created_at_utc TEXT NOT NULL,
			retired_at_utc TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '' CHECK(length(reason)<=2000),
			claim_state TEXT NOT NULL DEFAULT 'PENDING' CHECK(claim_state IN ('PENDING','CLAIMING','CLAIMED')),
			claimed_at_utc TEXT NOT NULL DEFAULT '',
			CHECK(
				(identity_state='ACTIVE' AND target_uuid='' AND retired_at_utc='' AND reason='') OR
				(identity_state='ALIAS' AND target_uuid<>'' AND retired_at_utc<>'' AND reason='') OR
				(identity_state='TOMBSTONE' AND target_uuid='' AND retired_at_utc<>'')
			),
			CHECK((claim_state='CLAIMED' AND claimed_at_utc<>'') OR (claim_state<>'CLAIMED' AND claimed_at_utc=''))
		);
		CREATE INDEX portable_identity_claims_import_state_kind
			ON portable_identity_claims(import_id,claim_state,entity_kind,uuid);

		CREATE TABLE portable_library_mappings (
			import_id TEXT NOT NULL REFERENCES portable_import_sessions(import_id) ON DELETE RESTRICT,
			library_key TEXT NOT NULL CHECK(library_key<>'' AND length(library_key)<=100),
			library_name TEXT NOT NULL CHECK(library_name<>'' AND length(library_name)<=300),
			decision TEXT NOT NULL DEFAULT 'UNMAPPED' CHECK(decision IN ('UNMAPPED','MAPPED','SKIPPED')),
			target_library_id INTEGER REFERENCES media_libraries(id) ON DELETE RESTRICT,
			updated_at_utc TEXT NOT NULL,
			PRIMARY KEY(import_id,library_key),
			CHECK((decision='MAPPED' AND target_library_id IS NOT NULL) OR (decision<>'MAPPED' AND target_library_id IS NULL))
		);

		CREATE TABLE portable_gallery_rebuilds (
			import_id TEXT NOT NULL REFERENCES portable_import_sessions(import_id) ON DELETE RESTRICT,
			set_id TEXT NOT NULL CHECK(set_id=lower(set_id) AND length(set_id)=36),
			library_key TEXT CHECK(library_key IS NULL OR (library_key<>'' AND length(library_key)<=100)),
			source_type TEXT NOT NULL CHECK(source_type IN ('','DIRECTORY','ARCHIVE')),
			relative_source TEXT NOT NULL CHECK(length(relative_source)<=4096),
			locator_status TEXT NOT NULL CHECK(locator_status IN ('MAPPED','LIBRARY_ROOT','UNBOUND','OUTSIDE_LIBRARY')),
			manifest_status TEXT NOT NULL CHECK(manifest_status IN ('CLEAN','MISSING','ERROR','DB_DIRTY','FILE_DIRTY','CONFLICT')),
			manifest_schema INTEGER NOT NULL CHECK(manifest_schema>=0),
			manifest_revision INTEGER NOT NULL CHECK(manifest_revision>=0),
			manifest_hash TEXT NOT NULL DEFAULT '' CHECK(length(manifest_hash)<=200),
			state TEXT NOT NULL DEFAULT 'PENDING' CHECK(state IN ('PENDING','READY','REBUILDING','REBUILT','BLOCKED','SKIPPED')),
			issue_code TEXT NOT NULL DEFAULT '' CHECK(length(issue_code)<=100),
			gallery_id INTEGER REFERENCES galleries(id) ON DELETE RESTRICT,
			source_id INTEGER REFERENCES gallery_sources(id) ON DELETE RESTRICT,
			updated_at_utc TEXT NOT NULL,
			PRIMARY KEY(import_id,set_id),
			FOREIGN KEY(import_id,library_key) REFERENCES portable_library_mappings(import_id,library_key) ON DELETE RESTRICT,
			CHECK((locator_status IN ('MAPPED','LIBRARY_ROOT') AND library_key IS NOT NULL AND source_type<>'') OR locator_status NOT IN ('MAPPED','LIBRARY_ROOT')),
			CHECK((locator_status='MAPPED' AND relative_source<>'') OR (locator_status<>'MAPPED' AND relative_source='')),
			CHECK((state='REBUILT' AND gallery_id IS NOT NULL AND source_id IS NOT NULL) OR state<>'REBUILT')
		);
		CREATE INDEX portable_gallery_rebuilds_import_state ON portable_gallery_rebuilds(import_id,state,set_id);

		CREATE TRIGGER portable_identity_claim_fields_immutable
		BEFORE UPDATE OF uuid,import_id,entity_kind,identity_state,target_uuid,created_at_utc,retired_at_utc,reason
		ON portable_identity_claims BEGIN
			SELECT RAISE(ABORT,'portable identity claim source fields are immutable');
		END;

		CREATE TRIGGER portable_identity_claim_no_delete
		BEFORE DELETE ON portable_identity_claims BEGIN
			SELECT RAISE(ABORT,'portable identity claims are permanent');
		END;

		CREATE TRIGGER portable_identity_claim_transition
		BEFORE UPDATE OF claim_state,claimed_at_utc ON portable_identity_claims BEGIN
			SELECT CASE WHEN NOT (
				(OLD.claim_state='PENDING' AND NEW.claim_state='CLAIMING' AND NEW.claimed_at_utc='') OR
				(OLD.claim_state='CLAIMING' AND NEW.claim_state='CLAIMED' AND NEW.claimed_at_utc<>'')
			) THEN RAISE(ABORT,'invalid portable identity claim transition') END;
		END;

		CREATE TRIGGER portable_uuid_registry_pending_claim_guard
		BEFORE INSERT ON portable_uuid_registry
		WHEN EXISTS(SELECT 1 FROM portable_identity_claims WHERE uuid=NEW.uuid AND claim_state='PENDING')
		BEGIN
			SELECT RAISE(ABORT,'portable UUID is reserved by a pending import claim');
		END;
	`); err != nil {
		return fmt.Errorf("creating portable import schema version 9: %w", err)
	}
	return nil
}

func validatePortableImportSchemaV9(ctx context.Context, db *sql.DB) error {
	for _, item := range []struct {
		kind  string
		names []string
	}{{"table", requiredPortableImportSchemaV9Tables}, {"trigger", requiredPortableImportSchemaV9Triggers}} {
		for _, name := range item.names {
			exists, err := schemaObjectExists(ctx, db, item.kind, name)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("%w: required %s %q is missing", ErrInvalidProductSchema, item.kind, name)
			}
		}
	}
	var maintenanceSQL string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_schema WHERE type='table' AND name='maintenance_state'`).Scan(&maintenanceSQL); err != nil {
		return err
	}
	if !strings.Contains(maintenanceSQL, "'PORTABLE_IMPORTING'") {
		return fmt.Errorf("%w: maintenance_state does not allow PORTABLE_IMPORTING", ErrInvalidProductSchema)
	}
	return nil
}

func validateSchemaV9(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV8(ctx, db); err != nil {
		return err
	}
	return validatePortableImportSchemaV9(ctx, db)
}
