package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

// Schema v18 separates the Gallery lifecycle timestamp from source-derived
// media-added evidence. Evidence remains local to the current source machine
// and is replaced only by a complete source scan.
func createMediaAddedSchemaV18(ctx context.Context, tx *sql.Tx) error {
	columns := []struct{ table, name, definition string }{
		{"galleries", "first_activated_at_utc", `first_activated_at_utc TEXT`},
		{"galleries", "media_added_start_at_utc", `media_added_start_at_utc TEXT NOT NULL DEFAULT ''`},
		{"galleries", "media_added_end_at_utc", `media_added_end_at_utc TEXT NOT NULL DEFAULT ''`},
		{"galleries", "media_added_status", `media_added_status TEXT NOT NULL DEFAULT 'PENDING' CHECK(media_added_status IN ('PENDING','COMPLETE','PARTIAL','NONE'))`},
		{"galleries", "media_added_revision", `media_added_revision INTEGER NOT NULL DEFAULT 0 CHECK(media_added_revision>=0)`},
		{"galleries", "publish_date", `publish_date TEXT`},
		{"galleries", "publish_date_precision", `publish_date_precision TEXT CHECK(publish_date_precision IN ('MONTH','DAY'))`},
		{"gallery_items", "source_modified_at_utc", `source_modified_at_utc TEXT NOT NULL DEFAULT ''`},
		{"gallery_items", "source_modified_status", `source_modified_status TEXT NOT NULL DEFAULT 'PENDING' CHECK(source_modified_status IN ('PENDING','FOUND','NONE'))`},
		{"gallery_items", "source_modified_origin", `source_modified_origin TEXT NOT NULL DEFAULT '' CHECK(source_modified_origin IN ('','FILESYSTEM','ARCHIVE_ENTRY'))`},
		{"gallery_items", "source_modified_checked_at_utc", `source_modified_checked_at_utc TEXT NOT NULL DEFAULT ''`},
		{"gallery_scan_observations", "source_modified_at_utc", `source_modified_at_utc TEXT NOT NULL DEFAULT ''`},
		{"gallery_scan_observations", "source_modified_status", `source_modified_status TEXT NOT NULL DEFAULT 'NONE' CHECK(source_modified_status IN ('FOUND','NONE'))`},
		{"gallery_scan_observations", "source_modified_origin", `source_modified_origin TEXT NOT NULL DEFAULT '' CHECK(source_modified_origin IN ('','FILESYSTEM','ARCHIVE_ENTRY'))`},
	}
	for _, column := range columns {
		present, err := tableColumnExists(ctx, tx, column.table, column.name)
		if err != nil {
			return err
		}
		if present {
			continue
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE `+column.table+` ADD COLUMN `+column.definition); err != nil {
			return fmt.Errorf("creating media-added schema version 18: %w", err)
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE galleries SET first_activated_at_utc=added_at_utc
		WHERE first_activated_at_utc IS NULL AND added_at_utc IS NOT NULL`)
	return err
}

func validateSchemaV18(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV17(ctx, db); err != nil {
		return err
	}
	for table, columns := range map[string][]string{
		"galleries":                 {"first_activated_at_utc", "media_added_start_at_utc", "media_added_end_at_utc", "media_added_status", "media_added_revision", "publish_date", "publish_date_precision"},
		"gallery_items":             {"source_modified_at_utc", "source_modified_status", "source_modified_origin", "source_modified_checked_at_utc"},
		"gallery_scan_observations": {"source_modified_at_utc", "source_modified_status", "source_modified_origin"},
	} {
		if err := requireTableColumns(ctx, db, table, columns); err != nil {
			return err
		}
	}
	return nil
}
