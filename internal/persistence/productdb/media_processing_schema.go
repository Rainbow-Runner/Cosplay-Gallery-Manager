package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredMediaProcessingSchemaV1Tables = []string{
	"gallery_covers",
	"media_derivatives",
	"processing_jobs",
}

func createMediaProcessingSchemaV1(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE processing_jobs (
			id INTEGER NOT NULL PRIMARY KEY,
			job_key TEXT NOT NULL UNIQUE CHECK (job_key <> '' AND length(job_key) <= 500),
			job_kind TEXT NOT NULL CHECK (job_kind IN ('LIBRARY_SCAN','GALLERY_PROCESSING','ITEM_DERIVATIVE','MANIFEST','CACHE','BACKUP')),
			gallery_id INTEGER REFERENCES galleries(id) ON DELETE CASCADE,
			item_uuid TEXT REFERENCES gallery_items(item_uuid) ON DELETE CASCADE,
			variant TEXT NOT NULL DEFAULT '' CHECK (length(variant) <= 100),
			content_revision INTEGER CHECK (content_revision IS NULL OR content_revision > 0),
			profile_hash TEXT NOT NULL DEFAULT '' CHECK (length(profile_hash) <= 200),
			payload_json BLOB NOT NULL DEFAULT '{}' CHECK (length(payload_json) <= 1048576),
			status TEXT NOT NULL CHECK (status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED','COMPLETED','FAILED','CANCELLED')),
			priority INTEGER NOT NULL,
			attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
			max_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts >= 1 AND max_attempts <= 20),
			not_before_utc TEXT NOT NULL,
			lease_owner TEXT,
			lease_expires_at_utc TEXT,
			last_heartbeat_at_utc TEXT,
			last_error_code TEXT NOT NULL DEFAULT '' CHECK (length(last_error_code) <= 100),
			structural_failure INTEGER NOT NULL DEFAULT 0 CHECK (structural_failure IN (0,1)),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL,
			completed_at_utc TEXT,
			CHECK ((status = 'RUNNING' AND lease_owner IS NOT NULL AND lease_expires_at_utc IS NOT NULL) OR
				(status <> 'RUNNING' AND lease_owner IS NULL AND lease_expires_at_utc IS NULL))
		);
		CREATE INDEX processing_jobs_claim ON processing_jobs(status, not_before_utc, priority DESC, id);
		CREATE INDEX processing_jobs_gallery ON processing_jobs(gallery_id, status, priority DESC);

		CREATE TABLE media_derivatives (
			id INTEGER NOT NULL PRIMARY KEY,
			item_uuid TEXT NOT NULL REFERENCES gallery_items(item_uuid) ON DELETE CASCADE,
			variant TEXT NOT NULL CHECK (variant <> '' AND length(variant) <= 100),
			cache_tier TEXT NOT NULL CHECK (cache_tier IN ('BASE','ENHANCED')),
			content_revision INTEGER NOT NULL CHECK (content_revision > 0),
			profile_hash TEXT NOT NULL CHECK (profile_hash <> '' AND length(profile_hash) <= 200),
			state TEXT NOT NULL CHECK (state IN ('READY','STALE','HARD_INVALID')),
			is_current INTEGER NOT NULL DEFAULT 0 CHECK (is_current IN (0,1)),
			cache_relative_path TEXT NOT NULL CHECK (cache_relative_path <> '' AND length(cache_relative_path) <= 4096),
			mime_type TEXT NOT NULL CHECK (mime_type <> '' AND length(mime_type) <= 100),
			byte_size INTEGER NOT NULL CHECK (byte_size >= 0),
			width INTEGER NOT NULL DEFAULT 0 CHECK (width >= 0),
			height INTEGER NOT NULL DEFAULT 0 CHECK (height >= 0),
			created_at_utc TEXT NOT NULL,
			last_accessed_at_utc TEXT NOT NULL,
			UNIQUE(item_uuid, variant, content_revision, profile_hash)
		);
		CREATE UNIQUE INDEX media_derivatives_current ON media_derivatives(item_uuid, variant) WHERE is_current = 1;
		CREATE INDEX media_derivatives_lru ON media_derivatives(cache_tier, state, is_current, last_accessed_at_utc);

		CREATE TABLE gallery_covers (
			gallery_id INTEGER NOT NULL PRIMARY KEY REFERENCES galleries(id) ON DELETE CASCADE,
			preferred_kind TEXT NOT NULL CHECK (preferred_kind IN ('NONE','AUTO_RANDOM','ITEM','MANAGED')),
			preferred_item_uuid TEXT REFERENCES gallery_items(item_uuid) ON DELETE SET NULL,
			preferred_path TEXT NOT NULL DEFAULT '' CHECK (length(preferred_path) <= 4096),
			fallback_item_uuid TEXT REFERENCES gallery_items(item_uuid) ON DELETE SET NULL,
			effective_kind TEXT NOT NULL CHECK (effective_kind IN ('NONE','ITEM','VIDEO_POSTER','MANAGED')),
			effective_item_uuid TEXT REFERENCES gallery_items(item_uuid) ON DELETE SET NULL,
			effective_path TEXT NOT NULL DEFAULT '' CHECK (length(effective_path) <= 4096),
			warning_code TEXT NOT NULL DEFAULT '' CHECK (length(warning_code) <= 100),
			cover_revision INTEGER NOT NULL DEFAULT 1 CHECK (cover_revision > 0),
			previous_json BLOB CHECK (previous_json IS NULL OR length(previous_json) <= 16384),
			updated_at_utc TEXT NOT NULL,
			CHECK ((preferred_kind IN ('AUTO_RANDOM','ITEM') AND preferred_item_uuid IS NOT NULL AND preferred_path = '') OR
				(preferred_kind = 'MANAGED' AND preferred_item_uuid IS NULL AND preferred_path <> '') OR
				(preferred_kind = 'NONE' AND preferred_item_uuid IS NULL AND preferred_path = '')),
			CHECK ((effective_kind IN ('ITEM','VIDEO_POSTER') AND effective_item_uuid IS NOT NULL AND effective_path = '') OR
				(effective_kind = 'MANAGED' AND effective_item_uuid IS NULL AND effective_path <> '') OR
				(effective_kind = 'NONE' AND effective_item_uuid IS NULL AND effective_path = ''))
		);
	`); err != nil {
		return fmt.Errorf("creating media processing schema version 1: %w", err)
	}
	return nil
}
