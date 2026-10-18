package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

// The legacy read_only flag governed sidecar writes, never source-media reads.
// Preserve each existing library's effective permission during migration.
func createMetadataWritebackSchemaV14(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `ALTER TABLE media_libraries ADD COLUMN metadata_writeback_enabled INTEGER NOT NULL DEFAULT 1 CHECK(metadata_writeback_enabled IN (0,1));
		UPDATE media_libraries SET metadata_writeback_enabled=CASE WHEN read_only=1 THEN 0 ELSE 1 END`); err != nil {
		return fmt.Errorf("creating media-library metadata writeback policy: %w", err)
	}
	return nil
}

func validateSchemaV14(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV13(ctx, db); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(media_libraries)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var position, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&position, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == "metadata_writeback_enabled" && notNull == 1 && defaultValue.Valid && defaultValue.String == "1" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return fmt.Errorf("%w: media_libraries.metadata_writeback_enabled is missing or invalid", ErrInvalidProductSchema)
}
