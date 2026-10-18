package productdb

import (
	"context"
	"database/sql"
)

func createTitleSyncSchemaV24(ctx context.Context, tx *sql.Tx) error {
	for _, column := range []struct{ name, ddl string }{
		{"title_origin", `ALTER TABLE galleries ADD COLUMN title_origin TEXT NOT NULL DEFAULT 'MANUAL' CHECK(title_origin IN ('AUTO','MANUAL'))`},
		{"source_title_baseline", `ALTER TABLE galleries ADD COLUMN source_title_baseline TEXT NOT NULL DEFAULT ''`},
		{"title_source_rule", `ALTER TABLE galleries ADD COLUMN title_source_rule TEXT NOT NULL DEFAULT 'ROOT' CHECK(title_source_rule IN ('ROOT','MARKER','ARCHIVE'))`},
	} {
		present, err := tableColumnExists(ctx, tx, "galleries", column.name)
		if err != nil {
			return err
		}
		if !present {
			if _, err := tx.ExecContext(ctx, column.ddl); err != nil {
				return err
			}
		}
	}
	// Historical title ownership is unknown. Never infer it from matching text.
	return nil
}

func validateSchemaV24(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV23(ctx, db); err != nil {
		return err
	}
	return requireTableColumns(ctx, db, "galleries", []string{"title_origin", "source_title_baseline", "title_source_rule"})
}
