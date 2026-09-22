package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredSettingsSchemaV3Columns = []string{
	"gallery_animated_playback_limit",
	"gallery_animated_lock_interval_ms",
}

func createSettingsSchemaV3(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE runtime_settings ADD COLUMN gallery_animated_playback_limit INTEGER NOT NULL DEFAULT 12 CHECK(gallery_animated_playback_limit BETWEEN 1 AND 16);
		ALTER TABLE runtime_settings ADD COLUMN gallery_animated_lock_interval_ms INTEGER NOT NULL DEFAULT 800 CHECK(gallery_animated_lock_interval_ms BETWEEN 700 AND 1000);
	`); err != nil {
		return fmt.Errorf("creating runtime settings schema version 3: %w", err)
	}
	return nil
}

func validateSettingsSchemaV3(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(runtime_settings)`)
	if err != nil {
		return fmt.Errorf("%w: reading runtime settings columns: %v", ErrInvalidProductSchema, err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("%w: reading runtime settings column: %v", ErrInvalidProductSchema, err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: reading runtime settings columns: %v", ErrInvalidProductSchema, err)
	}
	for _, column := range requiredSettingsSchemaV3Columns {
		if !columns[column] {
			return fmt.Errorf("%w: runtime_settings column %q is missing", ErrInvalidProductSchema, column)
		}
	}
	return nil
}
