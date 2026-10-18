package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

// Only dates used by the Gallery business workflow are persisted. Full EXIF,
// video tags, physical paths and derived thumbnails remain outside this table.
func createCaptureDateSchemaV15(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		ALTER TABLE galleries ADD COLUMN shoot_date_origin TEXT NOT NULL DEFAULT 'MANUAL'
			CHECK(shoot_date_origin IN ('MANUAL','AUTO'));
		CREATE TABLE item_capture_dates (
			item_uuid TEXT PRIMARY KEY REFERENCES gallery_items(item_uuid) ON DELETE CASCADE,
			content_revision INTEGER NOT NULL CHECK(content_revision > 0),
			capture_date TEXT CHECK(capture_date IS NULL OR length(capture_date)=10),
			source_tag TEXT NOT NULL DEFAULT '' CHECK(length(source_tag)<=100),
			status TEXT NOT NULL CHECK(status IN ('FOUND','NONE')),
			checked_at_utc TEXT NOT NULL,
			CHECK((status='FOUND' AND capture_date IS NOT NULL AND source_tag<>'') OR
				(status='NONE' AND capture_date IS NULL AND source_tag=''))
		);
		CREATE TABLE gallery_capture_date_reviews (
			gallery_id INTEGER PRIMARY KEY REFERENCES galleries(id) ON DELETE CASCADE,
			candidate_date TEXT NOT NULL CHECK(length(candidate_date)=10),
			manual_date TEXT NOT NULL CHECK(length(manual_date) BETWEEN 7 AND 10),
			status TEXT NOT NULL CHECK(status IN ('PENDING','KEEP_MANUAL')),
			updated_at_utc TEXT NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("creating capture-date schema v15: %w", err)
	}
	return nil
}

func validateSchemaV15(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV14(ctx, db); err != nil {
		return err
	}
	for _, table := range []string{"item_capture_dates", "gallery_capture_date_reviews"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			return fmt.Errorf("%w: table %s is missing", ErrInvalidProductSchema, table)
		}
	}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(galleries)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&id, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == "shoot_date_origin" && notNull == 1 {
			return nil
		}
	}
	return fmt.Errorf("%w: galleries.shoot_date_origin is missing", ErrInvalidProductSchema)
}
