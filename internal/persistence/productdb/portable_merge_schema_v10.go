package productdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

var requiredPortableMergeSchemaV10Tables = []string{"portable_merge_sessions", "portable_merge_conflicts"}
var requiredPortableMergeSchemaV10Triggers = []string{"portable_merge_session_source_immutable", "portable_merge_conflict_identity_immutable", "portable_merge_conflict_no_delete"}

// Schema v10 stores only package/target-bound merge workflow state. Incoming
// business fields remain in the quarantined package and are not duplicated in
// the product database.
func createPortableMergeSchemaV10(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE maintenance_state RENAME TO maintenance_state_v9;
		CREATE TABLE maintenance_state (
			id INTEGER NOT NULL PRIMARY KEY CHECK(id=1),
			state TEXT NOT NULL CHECK(state IN ('NORMAL','RESTORING','WAITING_VALIDATION','PORTABLE_IMPORTING','PORTABLE_MERGING')),
			restore_backup_id TEXT,
			last_error_code TEXT NOT NULL DEFAULT '' CHECK(length(last_error_code)<=100),
			updated_at_utc TEXT NOT NULL
		);
		INSERT INTO maintenance_state SELECT * FROM maintenance_state_v9;
		DROP TABLE maintenance_state_v9;

		CREATE TABLE portable_merge_sessions (
			merge_id TEXT NOT NULL PRIMARY KEY CHECK(merge_id=lower(merge_id) AND length(merge_id)=36),
			export_id TEXT NOT NULL CHECK(export_id=lower(export_id) AND length(export_id)=36),
			package_sha256 TEXT NOT NULL CHECK(length(package_sha256)=64),
			package_relative_path TEXT NOT NULL UNIQUE CHECK(package_relative_path<>'' AND length(package_relative_path)<=4096),
			target_fingerprint TEXT NOT NULL CHECK(length(target_fingerprint)=64),
			format_version INTEGER NOT NULL CHECK(format_version>0),
			state TEXT NOT NULL CHECK(state IN ('BLOCKED','DECISIONS_PENDING','READY','ABORTED','APPLYING','APPLIED','STALE','FAILED')),
			hard_blocking_count INTEGER NOT NULL CHECK(hard_blocking_count>=0),
			review_count INTEGER NOT NULL CHECK(review_count>=0),
			identity_add_count INTEGER NOT NULL CHECK(identity_add_count>=0),
			identity_reuse_count INTEGER NOT NULL CHECK(identity_reuse_count>=0),
			entity_add_count INTEGER NOT NULL CHECK(entity_add_count>=0),
			entity_reuse_count INTEGER NOT NULL CHECK(entity_reuse_count>=0),
			error_code TEXT NOT NULL DEFAULT '' CHECK(length(error_code)<=100),
			safety_backup_id TEXT NOT NULL DEFAULT '' CHECK(length(safety_backup_id) IN (0,36)),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL
		);

		CREATE TABLE portable_merge_conflicts (
			merge_id TEXT NOT NULL REFERENCES portable_merge_sessions(merge_id) ON DELETE RESTRICT,
			issue_key TEXT NOT NULL CHECK(length(issue_key)=64),
			issue_code TEXT NOT NULL CHECK(issue_code<>'' AND length(issue_code)<=100),
			severity TEXT NOT NULL CHECK(severity IN ('BLOCKING','REVIEW')),
			entity_kind TEXT NOT NULL CHECK(entity_kind IN ('GALLERY','GALLERY_ITEM','COSER','WORK','CHARACTER','TAG','EXTERNAL_LINK','SOCIAL_ACCOUNT')),
			incoming_uuid TEXT NOT NULL CHECK(incoming_uuid=lower(incoming_uuid) AND length(incoming_uuid)=36),
			local_uuid TEXT NOT NULL DEFAULT '' CHECK(local_uuid='' OR (local_uuid=lower(local_uuid) AND length(local_uuid)=36)),
			field_key TEXT NOT NULL DEFAULT '' CHECK(length(field_key)<=100),
			decision TEXT NOT NULL DEFAULT 'UNRESOLVED' CHECK(decision IN ('UNRESOLVED','KEEP_LOCAL','USE_INCOMING','KEEP_SEPARATE','MAP_TO_LOCAL')),
			updated_at_utc TEXT NOT NULL,
			PRIMARY KEY(merge_id,issue_key)
		);
		CREATE INDEX portable_merge_conflicts_state ON portable_merge_conflicts(merge_id,severity,decision,issue_key);

		CREATE TRIGGER portable_merge_session_source_immutable
		BEFORE UPDATE OF merge_id,export_id,package_sha256,package_relative_path,target_fingerprint,format_version,
			hard_blocking_count,review_count,identity_add_count,identity_reuse_count,entity_add_count,entity_reuse_count
		ON portable_merge_sessions BEGIN
			SELECT RAISE(ABORT,'portable merge source and preflight counts are immutable');
		END;

		CREATE TRIGGER portable_merge_conflict_identity_immutable
		BEFORE UPDATE OF merge_id,issue_key,issue_code,severity,entity_kind,incoming_uuid,local_uuid,field_key
		ON portable_merge_conflicts BEGIN
			SELECT RAISE(ABORT,'portable merge conflict identity is immutable');
		END;

		CREATE TRIGGER portable_merge_conflict_no_delete
		BEFORE DELETE ON portable_merge_conflicts BEGIN
			SELECT RAISE(ABORT,'portable merge conflicts are permanent');
		END;

	`); err != nil {
		return fmt.Errorf("creating portable merge schema version 10: %w", err)
	}
	return nil
}

func validatePortableMergeSchemaV10(ctx context.Context, db *sql.DB) error {
	for _, item := range []struct {
		kind  string
		names []string
	}{{"table", requiredPortableMergeSchemaV10Tables}, {"trigger", requiredPortableMergeSchemaV10Triggers}} {
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
	if !strings.Contains(maintenanceSQL, "'PORTABLE_MERGING'") {
		return fmt.Errorf("%w: maintenance_state does not allow PORTABLE_MERGING", ErrInvalidProductSchema)
	}
	return nil
}

func validateSchemaV10(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV9(ctx, db); err != nil {
		return err
	}
	return validatePortableMergeSchemaV10(ctx, db)
}
