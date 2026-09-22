package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredSettingsSchemaV11Columns = []string{
	"automatic_scan_on_startup",
	"automatic_scan_interval_minutes",
}

// Schema v11 completes the automatic scan schedule configuration. Existing
// installations retain the former daily cadence and do not gain a startup
// scan unless the owner explicitly enables it.
func createSettingsSchemaV11(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE runtime_settings ADD COLUMN automatic_scan_on_startup INTEGER NOT NULL DEFAULT 0 CHECK(automatic_scan_on_startup IN (0,1));
		ALTER TABLE runtime_settings ADD COLUMN automatic_scan_interval_minutes INTEGER NOT NULL DEFAULT 1440 CHECK(automatic_scan_interval_minutes BETWEEN 15 AND 10080);
	`); err != nil {
		return fmt.Errorf("creating runtime settings schema version 11: %w", err)
	}
	return nil
}

func validateSettingsSchemaV11(ctx context.Context, db *sql.DB) error {
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
	for _, column := range requiredSettingsSchemaV11Columns {
		if !columns[column] {
			return fmt.Errorf("%w: runtime_settings column %q is missing", ErrInvalidProductSchema, column)
		}
	}
	return nil
}

func validateSchemaV11(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV10(ctx, db); err != nil {
		return err
	}
	return validateSettingsSchemaV11(ctx, db)
}
