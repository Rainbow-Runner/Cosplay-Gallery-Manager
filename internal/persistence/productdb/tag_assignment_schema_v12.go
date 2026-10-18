package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

// Existing Tags remain directly assignable. The database triggers protect
// every writer, including Manifest rebuild and portable import paths.
func createTagAssignmentSchemaV12(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		ALTER TABLE tags ADD COLUMN allow_direct_assignment INTEGER NOT NULL DEFAULT 1 CHECK(allow_direct_assignment IN (0,1));
		CREATE TRIGGER gallery_tags_assignable_insert BEFORE INSERT ON gallery_tags
		WHEN (SELECT allow_direct_assignment FROM tags WHERE uuid=NEW.tag_uuid)=0
		BEGIN SELECT RAISE(ABORT,'TAG_NOT_ASSIGNABLE'); END;
		CREATE TRIGGER gallery_tags_assignable_update BEFORE UPDATE OF tag_uuid ON gallery_tags
		WHEN (SELECT allow_direct_assignment FROM tags WHERE uuid=NEW.tag_uuid)=0
		BEGIN SELECT RAISE(ABORT,'TAG_NOT_ASSIGNABLE'); END;
		CREATE TRIGGER tags_disable_direct_assignment BEFORE UPDATE OF allow_direct_assignment ON tags
		WHEN NEW.allow_direct_assignment=0 AND EXISTS(SELECT 1 FROM gallery_tags WHERE tag_uuid=NEW.uuid)
		BEGIN SELECT RAISE(ABORT,'TAG_HAS_DIRECT_GALLERIES'); END;
	`)
	if err != nil {
		return fmt.Errorf("creating Tag assignment schema version 12: %w", err)
	}
	return nil
}

func validateSchemaV12(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV11(ctx, db); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(tags)`)
	if err != nil {
		return fmt.Errorf("%w: inspecting tags: %v", ErrInvalidProductSchema, err)
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		found = found || name == "allow_direct_assignment" && notNull == 1
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: tags.allow_direct_assignment is missing", ErrInvalidProductSchema)
	}
	for _, trigger := range []string{"gallery_tags_assignable_insert", "gallery_tags_assignable_update", "tags_disable_direct_assignment"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type='trigger' AND name=?`, trigger).Scan(&count); err != nil || count != 1 {
			return fmt.Errorf("%w: trigger %s is missing", ErrInvalidProductSchema, trigger)
		}
	}
	return nil
}
