package productdb

import (
	"context"
	"database/sql"
)

// This local outbox intentionally has no Item FK: it must survive aggregate
// deletion and cache files are not part of portable metadata packages.
func createCacheLifecycleSchemaV20(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS cache_cleanup_outbox (
			cache_relative_path TEXT PRIMARY KEY,
			item_uuid TEXT NOT NULL,
			variant TEXT NOT NULL,
			content_revision INTEGER NOT NULL,
			profile_hash TEXT NOT NULL,
			byte_size INTEGER NOT NULL CHECK(byte_size>=0),
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error_code TEXT NOT NULL DEFAULT '',
			not_before_utc TEXT NOT NULL DEFAULT '',
			created_at_utc TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS cache_cleanup_scan_state (
			id INTEGER PRIMARY KEY CHECK(id=1),
			shard INTEGER NOT NULL DEFAULT 0 CHECK(shard BETWEEN 0 AND 255),
			cursor TEXT NOT NULL DEFAULT ''
		);
		INSERT OR IGNORE INTO cache_cleanup_scan_state(id) VALUES(1);
		CREATE TRIGGER IF NOT EXISTS media_derivative_cleanup_outbox AFTER DELETE ON media_derivatives BEGIN
			INSERT OR IGNORE INTO cache_cleanup_outbox(cache_relative_path,item_uuid,variant,content_revision,profile_hash,byte_size,created_at_utc)
			VALUES(OLD.cache_relative_path,OLD.item_uuid,OLD.variant,OLD.content_revision,OLD.profile_hash,OLD.byte_size,strftime('%Y-%m-%dT%H:%M:%fZ','now'));
		END;
		CREATE TRIGGER IF NOT EXISTS media_derivative_repath_outbox AFTER UPDATE OF cache_relative_path ON media_derivatives
		WHEN OLD.cache_relative_path<>NEW.cache_relative_path BEGIN
			INSERT OR IGNORE INTO cache_cleanup_outbox(cache_relative_path,item_uuid,variant,content_revision,profile_hash,byte_size,created_at_utc)
			VALUES(OLD.cache_relative_path,OLD.item_uuid,OLD.variant,OLD.content_revision,OLD.profile_hash,OLD.byte_size,strftime('%Y-%m-%dT%H:%M:%fZ','now'));
		END;
		CREATE INDEX IF NOT EXISTS media_derivatives_cache_path ON media_derivatives(cache_relative_path);
		CREATE INDEX IF NOT EXISTS processing_jobs_cache_identity ON processing_jobs(item_uuid,content_revision,variant,status);
	`)
	return err
}

func validateSchemaV20(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV19(ctx, db); err != nil {
		return err
	}
	if err := requireTableColumns(ctx, db, "cache_cleanup_outbox", []string{
		"cache_relative_path", "item_uuid", "variant", "content_revision", "profile_hash", "byte_size", "attempts", "last_error_code", "not_before_utc", "created_at_utc",
	}); err != nil {
		return err
	}
	if err := requireTableColumns(ctx, db, "cache_cleanup_scan_state", []string{"id", "shard", "cursor"}); err != nil {
		return err
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='trigger' AND name IN ('media_derivative_cleanup_outbox','media_derivative_repath_outbox')`).Scan(&count); err != nil {
		return err
	}
	if count != 2 {
		return ErrInvalidProductSchema
	}
	return nil
}
