package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredSettingsSchemaV1Tables = []string{"runtime_settings"}

func createSettingsSchemaV1(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE runtime_settings (
			id INTEGER NOT NULL PRIMARY KEY CHECK(id=1),
			settings_revision INTEGER NOT NULL DEFAULT 1 CHECK(settings_revision>0),
			home_scope TEXT NOT NULL DEFAULT 'LIST' CHECK(home_scope IN ('LIST','MAGIC','ALL')),
			gallery_card_scrubber_enabled INTEGER NOT NULL DEFAULT 1 CHECK(gallery_card_scrubber_enabled IN (0,1)),
			gallery_detail_media_filter_enabled INTEGER NOT NULL DEFAULT 0 CHECK(gallery_detail_media_filter_enabled IN (0,1)),
			gallery_card_controls_visible INTEGER NOT NULL DEFAULT 1 CHECK(gallery_card_controls_visible IN (0,1)),
			media_card_controls_visible INTEGER NOT NULL DEFAULT 1 CHECK(media_card_controls_visible IN (0,1)),
			detail_personal_controls_visible INTEGER NOT NULL DEFAULT 1 CHECK(detail_personal_controls_visible IN (0,1)),
			related_limit INTEGER NOT NULL DEFAULT 6 CHECK(related_limit BETWEEN 1 AND 24),
			tag_parent_weight REAL NOT NULL DEFAULT 0.25 CHECK(tag_parent_weight BETWEEN 0 AND 1),
			tag_minimum_score REAL NOT NULL DEFAULT 0.05 CHECK(tag_minimum_score BETWEEN 0 AND 1),
			tag_maximum_depth INTEGER NOT NULL DEFAULT 3 CHECK(tag_maximum_depth BETWEEN 0 AND 10),
			random_limit INTEGER NOT NULL DEFAULT 24 CHECK(random_limit BETWEEN 1 AND 100),
			random_static_quota REAL NOT NULL DEFAULT 0.70 CHECK(random_static_quota BETWEEN 0 AND 1),
			random_gif_quota REAL NOT NULL DEFAULT 0.10 CHECK(random_gif_quota BETWEEN 0 AND 1),
			random_video_quota REAL NOT NULL DEFAULT 0.20 CHECK(random_video_quota BETWEEN 0 AND 1),
			random_gallery_repeat_decay REAL NOT NULL DEFAULT 0.25 CHECK(random_gallery_repeat_decay BETWEEN 0 AND 1),
			enhanced_cache_maximum_bytes INTEGER NOT NULL DEFAULT 53687091200 CHECK(enhanced_cache_maximum_bytes>=0),
			minimum_free_bytes INTEGER NOT NULL DEFAULT 10737418240 CHECK(minimum_free_bytes>=0),
			minimum_free_percent REAL NOT NULL DEFAULT 0.05 CHECK(minimum_free_percent BETWEEN 0 AND 1),
			automatic_scan_enabled INTEGER NOT NULL DEFAULT 0 CHECK(automatic_scan_enabled IN (0,1)),
			automatic_schedules_suspended INTEGER NOT NULL DEFAULT 0 CHECK(automatic_schedules_suspended IN (0,1)),
			daily_backup_enabled INTEGER NOT NULL DEFAULT 1 CHECK(daily_backup_enabled IN (0,1)),
			daily_backup_retention INTEGER NOT NULL DEFAULT 7 CHECK(daily_backup_retention BETWEEN 1 AND 365),
			archive_max_entries INTEGER NOT NULL DEFAULT 20000 CHECK(archive_max_entries>0),
			archive_max_entry_bytes INTEGER NOT NULL DEFAULT 2147483648 CHECK(archive_max_entry_bytes>0),
			archive_max_total_bytes INTEGER NOT NULL DEFAULT 107374182400 CHECK(archive_max_total_bytes>0),
			archive_max_compression_ratio REAL NOT NULL DEFAULT 1000 CHECK(archive_max_compression_ratio>0),
			archive_max_image_pixels INTEGER NOT NULL DEFAULT 200000000 CHECK(archive_max_image_pixels>0),
			updated_at_utc TEXT NOT NULL,
			CHECK(abs((random_static_quota+random_gif_quota+random_video_quota)-1.0)<0.000001)
		);
		INSERT INTO runtime_settings(id,updated_at_utc) VALUES(1,strftime('%Y-%m-%dT%H:%M:%fZ','now'));
	`); err != nil {
		return fmt.Errorf("creating runtime settings schema version 1: %w", err)
	}
	return nil
}
