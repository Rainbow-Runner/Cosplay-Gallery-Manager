package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredOperationsSchemaV1Tables = []string{
	"backup_records",
	"maintenance_state",
	"management_audit_events",
	"scheduled_operations",
}

func createOperationsSchemaV1(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE backup_records (
			backup_id TEXT NOT NULL PRIMARY KEY CHECK(length(backup_id)=36),
			backup_kind TEXT NOT NULL CHECK(backup_kind IN ('DAILY_SNAPSHOT','MANUAL_FULL','SAFETY_SNAPSHOT')),
			file_name TEXT NOT NULL UNIQUE CHECK(file_name<>'' AND length(file_name)<=300 AND instr(file_name,'/')=0 AND instr(file_name,'\')=0),
			status TEXT NOT NULL CHECK(status IN ('CREATING','READY','FAILED')),
			byte_size INTEGER NOT NULL DEFAULT 0 CHECK(byte_size>=0),
			archive_sha256 TEXT NOT NULL DEFAULT '' CHECK(archive_sha256='' OR (length(archive_sha256)=64 AND archive_sha256 NOT GLOB '*[^0-9a-f]*')),
			product_version TEXT NOT NULL CHECK(product_version<>'' AND length(product_version)<=100),
			database_schema_version INTEGER NOT NULL CHECK(database_schema_version>0),
			manifest_schema_version INTEGER NOT NULL CHECK(manifest_schema_version>0),
			media_processing_version INTEGER NOT NULL CHECK(media_processing_version>0),
			created_at_utc TEXT NOT NULL,
			completed_at_utc TEXT,
			last_error_code TEXT NOT NULL DEFAULT '' CHECK(length(last_error_code)<=100)
		);
		CREATE INDEX backup_records_created ON backup_records(status,created_at_utc DESC,backup_id DESC);

		CREATE TABLE maintenance_state (
			id INTEGER NOT NULL PRIMARY KEY CHECK(id=1),
			state TEXT NOT NULL CHECK(state IN ('NORMAL','RESTORING','WAITING_VALIDATION')),
			restore_backup_id TEXT,
			last_error_code TEXT NOT NULL DEFAULT '' CHECK(length(last_error_code)<=100),
			updated_at_utc TEXT NOT NULL
		);
		INSERT INTO maintenance_state(id,state,updated_at_utc) VALUES(1,'NORMAL',strftime('%Y-%m-%dT%H:%M:%fZ','now'));

		CREATE TABLE management_audit_events (
			id INTEGER NOT NULL PRIMARY KEY,
			event_code TEXT NOT NULL CHECK(event_code<>'' AND length(event_code)<=100),
			target_kind TEXT NOT NULL DEFAULT '' CHECK(length(target_kind)<=50),
			target_id TEXT NOT NULL DEFAULT '' CHECK(length(target_id)<=100),
			outcome TEXT NOT NULL CHECK(outcome IN ('SUCCESS','FAILURE')),
			error_code TEXT NOT NULL DEFAULT '' CHECK(length(error_code)<=100),
			summary_json BLOB NOT NULL DEFAULT '{}' CHECK(length(summary_json)<=16384),
			created_at_utc TEXT NOT NULL
		);
		CREATE INDEX management_audit_events_created ON management_audit_events(created_at_utc DESC,id DESC);

		CREATE TABLE scheduled_operations (
			task_key TEXT NOT NULL PRIMARY KEY,
			last_started_at_utc TEXT,
			last_completed_at_utc TEXT,
			lease_owner TEXT,
			lease_expires_at_utc TEXT,
			last_error_code TEXT NOT NULL DEFAULT '' CHECK(length(last_error_code)<=100),
			updated_at_utc TEXT NOT NULL
		);
		INSERT INTO scheduled_operations(task_key,updated_at_utc)
		VALUES('DAILY_BACKUP',strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		      ('AUTOMATIC_SCAN',strftime('%Y-%m-%dT%H:%M:%fZ','now'));
	`); err != nil {
		return fmt.Errorf("creating operations schema version 1: %w", err)
	}
	return nil
}
