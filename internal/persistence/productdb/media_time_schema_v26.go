package productdb

import (
	"context"
	"database/sql"
)

func createMediaTimeSchemaV26(ctx context.Context, tx *sql.Tx) error {
	for _, c := range []struct{ table, name, definition string }{
		{"runtime_settings", "gallery_detail_loading_mode", `gallery_detail_loading_mode TEXT NOT NULL DEFAULT 'MANUAL' CHECK(gallery_detail_loading_mode IN ('MANUAL','AUTO'))`},
		{"runtime_settings", "media_time_backfill_enabled", `media_time_backfill_enabled INTEGER NOT NULL DEFAULT 1 CHECK(media_time_backfill_enabled IN (0,1))`},
		{"galleries", "software_time_utc", `software_time_utc TEXT NOT NULL DEFAULT ''`},
		{"galleries", "software_time_date", `software_time_date TEXT NOT NULL DEFAULT ''`},
		{"galleries", "software_time_status", `software_time_status TEXT NOT NULL DEFAULT 'PENDING' CHECK(software_time_status IN ('PENDING','PARTIAL','COMPLETE','NONE'))`},
		{"galleries", "time_evidence_revision", `time_evidence_revision INTEGER NOT NULL DEFAULT 0 CHECK(time_evidence_revision>=0)`},
	} {
		exists, err := tableColumnExists(ctx, tx, c.table, c.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err = tx.ExecContext(ctx, `ALTER TABLE `+c.table+` ADD COLUMN `+c.definition); err != nil {
				return err
			}
		}
	}
	_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS item_media_times (
  item_uuid TEXT PRIMARY KEY REFERENCES gallery_items(item_uuid) ON DELETE CASCADE,
  content_revision INTEGER NOT NULL CHECK(content_revision>0),profile TEXT NOT NULL,timezone TEXT NOT NULL,
  capture_at_utc TEXT NOT NULL DEFAULT '',capture_precision TEXT NOT NULL DEFAULT '' CHECK(capture_precision IN ('','DAY','SECOND')),capture_zone_source TEXT NOT NULL DEFAULT '' CHECK(capture_zone_source IN ('','EMBEDDED','LIBRARY')),
  software_at_utc TEXT NOT NULL DEFAULT '',software_local_date TEXT NOT NULL DEFAULT '',software_event_index INTEGER NOT NULL DEFAULT 0,software_zone_source TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL CHECK(state IN ('READY','PARTIAL')),checked_at_utc TEXT NOT NULL
 );`)
	if err != nil {
		return err
	}
	for _, rule := range []struct{ name, event, when, ids string }{
		{"item_insert", "AFTER INSERT ON gallery_items", "NEW.media_kind='STATIC_IMAGE'", "NEW.gallery_id"},
		{"item_delete", "AFTER DELETE ON gallery_items", "OLD.media_kind='STATIC_IMAGE'", "OLD.gallery_id"},
		{"item_change", "AFTER UPDATE OF content_revision,excluded,availability_state,media_kind,source_id,gallery_id ON gallery_items", "OLD.content_revision IS NOT NEW.content_revision OR OLD.excluded IS NOT NEW.excluded OR OLD.availability_state IS NOT NEW.availability_state OR OLD.media_kind IS NOT NEW.media_kind OR OLD.source_id IS NOT NEW.source_id OR OLD.gallery_id IS NOT NEW.gallery_id", "OLD.gallery_id,NEW.gallery_id"},
		{"library_zone", "AFTER UPDATE OF capture_timezone ON media_libraries", "OLD.capture_timezone IS NOT NEW.capture_timezone", "SELECT gallery_id FROM gallery_sources WHERE library_id=NEW.id"},
		{"source_change", "AFTER UPDATE OF library_id,availability_state,gallery_id ON gallery_sources", "OLD.library_id IS NOT NEW.library_id OR OLD.availability_state IS NOT NEW.availability_state OR OLD.gallery_id IS NOT NEW.gallery_id", "OLD.gallery_id,NEW.gallery_id"},
	} {
		_, err = tx.ExecContext(ctx, `CREATE TRIGGER IF NOT EXISTS cgm_media_time_`+rule.name+` `+rule.event+` WHEN `+rule.when+` BEGIN UPDATE galleries SET software_time_utc='',software_time_date='',software_time_status='PENDING',time_evidence_revision=time_evidence_revision+1 WHERE id IN (`+rule.ids+`); END;`)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaV26(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV25(ctx, db); err != nil {
		return err
	}
	for table, cols := range map[string][]string{
		"runtime_settings": {"gallery_detail_loading_mode", "media_time_backfill_enabled"},
		"galleries":        {"software_time_utc", "software_time_date", "software_time_status", "time_evidence_revision"},
		"item_media_times": {"item_uuid", "content_revision", "profile", "timezone", "capture_at_utc", "capture_precision", "capture_zone_source", "software_at_utc", "software_local_date", "software_event_index", "software_zone_source", "state", "checked_at_utc"},
	} {
		if err := requireTableColumns(ctx, db, table, cols); err != nil {
			return err
		}
	}
	return nil
}
