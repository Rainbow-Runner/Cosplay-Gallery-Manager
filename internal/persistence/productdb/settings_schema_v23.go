package productdb

import (
	"context"
	"database/sql"
)

func createSettingsSchemaV23(ctx context.Context, tx *sql.Tx) error {
	present, err := tableColumnExists(ctx, tx, "runtime_settings", "gallery_cover_1600_enabled")
	if err != nil || present {
		return err
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE runtime_settings ADD COLUMN gallery_cover_1600_enabled INTEGER NOT NULL DEFAULT 0 CHECK(gallery_cover_1600_enabled IN (0,1))`)
	return err
}
func validateSchemaV23(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV22(ctx, db); err != nil {
		return err
	}
	return requireTableColumns(ctx, db, "runtime_settings", []string{"gallery_cover_1600_enabled"})
}
