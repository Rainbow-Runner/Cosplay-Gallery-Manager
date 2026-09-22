package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrIntegrityCheckFailed = errors.New("SQLite integrity check failed")
	ErrInvalidProductSchema = errors.New("product database schema is incomplete or invalid")
)

var requiredSchemaV1Tables = []string{
	identityTable,
	"owner_auth",
	"owner_sessions",
	"product_setup",
	"setup_tokens",
	"portable_uuid_aliases",
	"portable_uuid_registry",
	"portable_uuid_tombstones",
}

var requiredSchemaV1Triggers = []string{
	"portable_uuid_alias_immutable_delete",
	"portable_uuid_alias_immutable_update",
	"portable_uuid_alias_validate",
	"portable_uuid_registry_identity_immutable",
	"portable_uuid_registry_no_delete",
	"portable_uuid_tombstone_immutable_delete",
	"portable_uuid_tombstone_immutable_update",
	"portable_uuid_tombstone_validate",
}

func createSchemaV1(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE portable_uuid_registry (
			uuid TEXT NOT NULL PRIMARY KEY,
			entity_kind TEXT NOT NULL CHECK (entity_kind IN (
				'GALLERY', 'GALLERY_ITEM', 'COSER', 'WORK', 'CHARACTER',
				'TAG', 'EXTERNAL_LINK', 'SOCIAL_ACCOUNT'
			)),
			created_at_utc TEXT NOT NULL,
			CHECK (uuid = lower(uuid) AND length(uuid) = 36)
		);

		CREATE TABLE portable_uuid_aliases (
			alias_uuid TEXT NOT NULL PRIMARY KEY
				REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			target_uuid TEXT NOT NULL
				REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			entity_kind TEXT NOT NULL,
			merged_at_utc TEXT NOT NULL,
			CHECK (alias_uuid <> target_uuid)
		);

		CREATE TABLE portable_uuid_tombstones (
			uuid TEXT NOT NULL PRIMARY KEY
				REFERENCES portable_uuid_registry(uuid) ON DELETE RESTRICT,
			entity_kind TEXT NOT NULL,
			deleted_at_utc TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT ''
		);

		CREATE TRIGGER portable_uuid_registry_no_delete
		BEFORE DELETE ON portable_uuid_registry
		BEGIN
			SELECT RAISE(ABORT, 'portable UUID registry records are permanent');
		END;

		CREATE TRIGGER portable_uuid_registry_identity_immutable
		BEFORE UPDATE OF uuid, entity_kind, created_at_utc ON portable_uuid_registry
		BEGIN
			SELECT RAISE(ABORT, 'portable UUID registry identity is immutable');
		END;

		CREATE TRIGGER portable_uuid_alias_validate
		BEFORE INSERT ON portable_uuid_aliases
		BEGIN
			SELECT CASE WHEN EXISTS (
				SELECT 1 FROM portable_uuid_aliases WHERE alias_uuid = NEW.target_uuid
			) OR EXISTS (
				SELECT 1 FROM portable_uuid_tombstones WHERE uuid = NEW.alias_uuid
			) OR EXISTS (
				SELECT 1 FROM portable_uuid_tombstones WHERE uuid = NEW.target_uuid
			) THEN RAISE(ABORT, 'portable UUID alias endpoint is not active') END;
			SELECT CASE WHEN NOT EXISTS (
				SELECT 1
				FROM portable_uuid_registry source
				JOIN portable_uuid_registry target ON target.uuid = NEW.target_uuid
				WHERE source.uuid = NEW.alias_uuid
					AND source.entity_kind = NEW.entity_kind
					AND target.entity_kind = NEW.entity_kind
			) THEN RAISE(ABORT, 'portable UUID alias kind mismatch') END;
		END;

		CREATE TRIGGER portable_uuid_alias_immutable_update
		BEFORE UPDATE ON portable_uuid_aliases
		BEGIN
			SELECT RAISE(ABORT, 'portable UUID aliases are permanent');
		END;

		CREATE TRIGGER portable_uuid_alias_immutable_delete
		BEFORE DELETE ON portable_uuid_aliases
		BEGIN
			SELECT RAISE(ABORT, 'portable UUID aliases are permanent');
		END;

		CREATE TRIGGER portable_uuid_tombstone_validate
		BEFORE INSERT ON portable_uuid_tombstones
		BEGIN
			SELECT CASE WHEN EXISTS (
				SELECT 1 FROM portable_uuid_aliases WHERE alias_uuid = NEW.uuid
			) THEN RAISE(ABORT, 'portable UUID alias cannot become a tombstone') END;
			SELECT CASE WHEN NOT EXISTS (
				SELECT 1 FROM portable_uuid_registry
				WHERE uuid = NEW.uuid AND entity_kind = NEW.entity_kind
			) THEN RAISE(ABORT, 'portable UUID tombstone kind mismatch') END;
		END;

		CREATE TRIGGER portable_uuid_tombstone_immutable_update
		BEFORE UPDATE ON portable_uuid_tombstones
		BEGIN
			SELECT RAISE(ABORT, 'portable UUID tombstones are permanent');
		END;

		CREATE TRIGGER portable_uuid_tombstone_immutable_delete
		BEFORE DELETE ON portable_uuid_tombstones
		BEGIN
			SELECT RAISE(ABORT, 'portable UUID tombstones are permanent');
		END
	`); err != nil {
		return fmt.Errorf("creating database schema version 1: %w", err)
	}
	if err := createCoreEntitySchemaV1(ctx, tx); err != nil {
		return err
	}
	if err := createGallerySchemaV1(ctx, tx); err != nil {
		return err
	}
	if err := createMediaProcessingSchemaV1(ctx, tx); err != nil {
		return err
	}
	if err := createSettingsSchemaV1(ctx, tx); err != nil {
		return err
	}
	if err := createOwnerAuthSchemaV1(ctx, tx); err != nil {
		return err
	}
	if err := createOperationsSchemaV1(ctx, tx); err != nil {
		return err
	}
	return nil
}

func validateSchemaV1(ctx context.Context, db *sql.DB) error {
	for _, table := range requiredSchemaV1Tables {
		exists, err := schemaObjectExists(ctx, db, "table", table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required table %q is missing", ErrInvalidProductSchema, table)
		}
	}
	for _, trigger := range requiredSchemaV1Triggers {
		exists, err := schemaObjectExists(ctx, db, "trigger", trigger)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required trigger %q is missing", ErrInvalidProductSchema, trigger)
		}
	}
	for _, table := range requiredGallerySchemaV1Tables {
		exists, err := schemaObjectExists(ctx, db, "table", table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required table %q is missing", ErrInvalidProductSchema, table)
		}
	}
	for _, trigger := range requiredGallerySchemaV1Triggers {
		exists, err := schemaObjectExists(ctx, db, "trigger", trigger)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required trigger %q is missing", ErrInvalidProductSchema, trigger)
		}
	}
	for _, table := range requiredCoreEntitySchemaV1Tables {
		exists, err := schemaObjectExists(ctx, db, "table", table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required table %q is missing", ErrInvalidProductSchema, table)
		}
	}
	for _, trigger := range requiredCoreEntitySchemaV1Triggers {
		exists, err := schemaObjectExists(ctx, db, "trigger", trigger)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required trigger %q is missing", ErrInvalidProductSchema, trigger)
		}
	}
	for _, table := range requiredMediaProcessingSchemaV1Tables {
		exists, err := schemaObjectExists(ctx, db, "table", table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required table %q is missing", ErrInvalidProductSchema, table)
		}
	}
	for _, table := range requiredSettingsSchemaV1Tables {
		exists, err := schemaObjectExists(ctx, db, "table", table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required table %q is missing", ErrInvalidProductSchema, table)
		}
	}
	for _, table := range requiredOperationsSchemaV1Tables {
		exists, err := schemaObjectExists(ctx, db, "table", table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required table %q is missing", ErrInvalidProductSchema, table)
		}
	}

	return nil
}

func validateSchemaV2(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV1(ctx, db); err != nil {
		return err
	}
	for _, table := range requiredMediaProcessingSchemaV2Tables {
		exists, err := schemaObjectExists(ctx, db, "table", table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required table %q is missing", ErrInvalidProductSchema, table)
		}
	}
	return nil
}

func validateSchemaV3(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV2(ctx, db); err != nil {
		return err
	}
	return validateSettingsSchemaV3(ctx, db)
}

func validateSchemaV4(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV3(ctx, db); err != nil {
		return err
	}
	return validateMediaClassificationSchemaV4(ctx, db)
}

func validateSchemaV5(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV4(ctx, db); err != nil {
		return err
	}
	return validateMediaExclusionSchemaV5(ctx, db)
}

func schemaObjectExists(ctx context.Context, db *sql.DB, objectType string, name string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_schema WHERE type = ? AND name = ?
	`, objectType, name).Scan(&count); err != nil {
		return false, fmt.Errorf("%w: checking %s %q: %v", ErrInvalidProductSchema, objectType, name, err)
	}
	return count == 1, nil
}

func validateIntegrity(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("running SQLite integrity check: %w", err)
	}
	defer rows.Close()

	var messages []string
	for rows.Next() {
		var message string
		if err := rows.Scan(&message); err != nil {
			return fmt.Errorf("reading SQLite integrity result: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading SQLite integrity results: %w", err)
	}
	if len(messages) != 1 || messages[0] != "ok" {
		return fmt.Errorf("%w: %v", ErrIntegrityCheckFailed, messages)
	}

	return nil
}
