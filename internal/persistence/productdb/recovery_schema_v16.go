package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

func createRecoverySchemaV16(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE TABLE owner_recovery_tokens (
		token_hash BLOB NOT NULL PRIMARY KEY CHECK(length(token_hash)=32),
		created_at_utc TEXT NOT NULL,
		expires_at_utc TEXT NOT NULL,
		used_at_utc TEXT
	);
	CREATE INDEX owner_recovery_tokens_expiry ON owner_recovery_tokens(expires_at_utc,used_at_utc);`)
	if err != nil {
		return fmt.Errorf("creating owner recovery schema v16: %w", err)
	}
	return nil
}

func validateSchemaV16(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV15(ctx, db); err != nil {
		return err
	}
	if exists, err := schemaObjectExists(ctx, db, "table", "owner_recovery_tokens"); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("%w: owner_recovery_tokens is missing", ErrInvalidProductSchema)
	}
	rows, err := db.QueryContext(ctx, `SELECT token_hash,created_at_utc,expires_at_utc,used_at_utc FROM owner_recovery_tokens LIMIT 0`)
	if err != nil {
		return fmt.Errorf("%w: owner_recovery_tokens columns are invalid: %v", ErrInvalidProductSchema, err)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return nil
}
