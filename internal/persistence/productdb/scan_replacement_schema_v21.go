package productdb

import (
	"context"
	"database/sql"
)

// Scan results are durable so asynchronous source scans remain visible after
// the automation run that discovered the source has completed.
func createScanReplacementSchemaV21(ctx context.Context, tx *sql.Tx) error {
	for _, name := range []string{"added_count", "missing_count", "changed_count", "rebound_count", "cleared_count", "cover_reselected"} {
		present, err := tableColumnExists(ctx, tx, "gallery_scan_runs", name)
		if err != nil {
			return err
		}
		if !present {
			if _, err := tx.ExecContext(ctx, `ALTER TABLE gallery_scan_runs ADD COLUMN `+name+` INTEGER NOT NULL DEFAULT 0`); err != nil {
				return err
			}
		}
	}
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS cover_image_signatures (
			item_uuid TEXT NOT NULL REFERENCES gallery_items(item_uuid) ON DELETE CASCADE,
			content_revision INTEGER NOT NULL CHECK(content_revision > 0),
			profile_hash TEXT NOT NULL,
			signature_version INTEGER NOT NULL,
			dhash_h TEXT NOT NULL CHECK(length(dhash_h)=16),
			dhash_v TEXT NOT NULL CHECK(length(dhash_v)=16),
			phash TEXT NOT NULL CHECK(length(phash)=16),
			width INTEGER NOT NULL CHECK(width > 0),
			height INTEGER NOT NULL CHECK(height > 0),
			created_at_utc TEXT NOT NULL,
			PRIMARY KEY(item_uuid,content_revision,profile_hash,signature_version)
		);
		CREATE TABLE IF NOT EXISTS cover_similarity_intents (
			gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
			scan_run_id INTEGER NOT NULL PRIMARY KEY REFERENCES gallery_scan_runs(id) ON DELETE CASCADE,
			old_signature_json BLOB NOT NULL CHECK(length(old_signature_json) <= 4096),
			candidates_json BLOB NOT NULL CHECK(length(candidates_json) <= 131072),
			expected_cover_revision INTEGER NOT NULL,
			expected_metadata_revision INTEGER NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('PENDING','MATCHED','MULTIPLE_MATCHED','NO_MATCH','NO_SIGNATURE','STALE','FALLBACK')),
			qualified_count INTEGER NOT NULL DEFAULT 0,
			selected_item_uuid TEXT,
			created_at_utc TEXT NOT NULL,
			next_check_at_utc TEXT NOT NULL,
			completed_at_utc TEXT
		);
		CREATE TABLE IF NOT EXISTS cover_signature_backfill_attempts (
			item_uuid TEXT NOT NULL REFERENCES gallery_items(item_uuid) ON DELETE CASCADE,
			content_revision INTEGER NOT NULL,
			profile_hash TEXT NOT NULL,
			attempted_at_utc TEXT NOT NULL,
			PRIMARY KEY(item_uuid,content_revision,profile_hash)
		);
		CREATE INDEX IF NOT EXISTS cover_similarity_intents_pending ON cover_similarity_intents(status,next_check_at_utc,scan_run_id);
	`)
	if err != nil {
		return err
	}
	return nil
}

func validateSchemaV21(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV20(ctx, db); err != nil {
		return err
	}
	if err := requireTableColumns(ctx, db, "gallery_scan_runs", []string{
		"added_count", "missing_count", "changed_count", "rebound_count", "cleared_count", "cover_reselected",
	}); err != nil {
		return err
	}
	if err := requireTableColumns(ctx, db, "cover_image_signatures", []string{"item_uuid", "content_revision", "profile_hash", "signature_version", "dhash_h", "dhash_v", "phash", "width", "height"}); err != nil {
		return err
	}
	if err := requireTableColumns(ctx, db, "cover_signature_backfill_attempts", []string{"item_uuid", "content_revision", "profile_hash", "attempted_at_utc"}); err != nil {
		return err
	}
	return requireTableColumns(ctx, db, "cover_similarity_intents", []string{"gallery_id", "scan_run_id", "old_signature_json", "candidates_json", "expected_cover_revision", "expected_metadata_revision", "status", "next_check_at_utc"})
}
