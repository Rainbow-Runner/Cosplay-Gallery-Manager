package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

func createOwnerAuthSchemaV1(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE owner_auth (
			id INTEGER NOT NULL PRIMARY KEY CHECK (id=1),
			password_hash TEXT,
			trusted_mode INTEGER NOT NULL DEFAULT 0 CHECK (trusted_mode IN (0,1)),
			auth_revision INTEGER NOT NULL DEFAULT 1 CHECK (auth_revision>0),
			configured_at_utc TEXT,
			updated_at_utc TEXT NOT NULL
		);
		INSERT INTO owner_auth(id,updated_at_utc) VALUES(1,strftime('%Y-%m-%dT%H:%M:%fZ','now'));

		CREATE TABLE owner_sessions (
			token_hash BLOB NOT NULL PRIMARY KEY CHECK (length(token_hash)=32),
			auth_revision INTEGER NOT NULL,
			created_at_utc TEXT NOT NULL,
			last_seen_at_utc TEXT NOT NULL,
			expires_at_utc TEXT NOT NULL,
			revoked_at_utc TEXT
		);
		CREATE INDEX owner_sessions_active ON owner_sessions(expires_at_utc,revoked_at_utc);

		CREATE TABLE product_setup (
			id INTEGER NOT NULL PRIMARY KEY CHECK (id=1),
			complete INTEGER NOT NULL DEFAULT 0 CHECK (complete IN (0,1)),
			runtime_environment TEXT NOT NULL DEFAULT 'NATIVE' CHECK (runtime_environment IN ('NATIVE','DOCKER')),
			locale TEXT NOT NULL DEFAULT 'zh-CN',
			timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
			coser_metadata_root TEXT NOT NULL DEFAULT '',
			backup_root TEXT NOT NULL DEFAULT '',
			completed_at_utc TEXT
		);
		INSERT INTO product_setup(id) VALUES(1);

		CREATE TABLE setup_tokens (
			token_hash BLOB NOT NULL PRIMARY KEY CHECK (length(token_hash)=32),
			token_kind TEXT NOT NULL CHECK (token_kind IN ('TICKET','SESSION')),
			created_at_utc TEXT NOT NULL,
			expires_at_utc TEXT NOT NULL,
			used_at_utc TEXT
		);
		CREATE INDEX setup_tokens_expiry ON setup_tokens(token_kind,expires_at_utc,used_at_utc);
	`); err != nil {
		return fmt.Errorf("creating owner authentication schema: %w", err)
	}
	return nil
}
