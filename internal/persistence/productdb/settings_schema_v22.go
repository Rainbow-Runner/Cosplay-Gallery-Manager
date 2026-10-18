package productdb

import (
	"context"
	"database/sql"
)

func createSettingsSchemaV22(ctx context.Context, tx *sql.Tx) error {
	columns := []struct{ name, statement string }{
		{"video_hardware_mode", `ALTER TABLE runtime_settings ADD COLUMN video_hardware_mode TEXT NOT NULL DEFAULT 'SOFTWARE' CHECK(video_hardware_mode IN ('SOFTWARE','AUTO','NVENC','VAAPI'))`},
		{"video_hardware_fallback_enabled", `ALTER TABLE runtime_settings ADD COLUMN video_hardware_fallback_enabled INTEGER NOT NULL DEFAULT 1 CHECK(video_hardware_fallback_enabled IN (0,1))`},
		{"video_hardware_device", `ALTER TABLE runtime_settings ADD COLUMN video_hardware_device TEXT NOT NULL DEFAULT '' CHECK(length(video_hardware_device) <= 128)`},
	}
	for _, column := range columns {
		present, err := tableColumnExists(ctx, tx, "runtime_settings", column.name)
		if err != nil {
			return err
		}
		if !present {
			if _, err := tx.ExecContext(ctx, column.statement); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSchemaV22(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV21(ctx, db); err != nil {
		return err
	}
	return requireTableColumns(ctx, db, "runtime_settings", []string{"video_hardware_mode", "video_hardware_fallback_enabled", "video_hardware_device"})
}
