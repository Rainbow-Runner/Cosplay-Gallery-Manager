// Package productdb owns the SQLite identity boundary for Cosplay Gallery
// Manager. It deliberately does not reuse the original Stash migration set.
package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/product"
)

const identityTable = "cgm_product_identity"

var (
	// ErrLegacyStashDatabase means the database has a recognisable original
	// Stash schema and must never be converted in place.
	ErrLegacyStashDatabase = errors.New("original Stash database is not supported")

	// ErrUnknownDatabase means a non-empty SQLite database does not carry this
	// product's valid identity marker.
	ErrUnknownDatabase = errors.New("unknown non-empty database is not supported")
)

// Kind is the result of a read-only database identity inspection.
type Kind string

const (
	KindEmpty       Kind = "EMPTY"
	KindProduct     Kind = "PRODUCT"
	KindLegacyStash Kind = "LEGACY_STASH"
	KindUnknown     Kind = "UNKNOWN"
)

// Identity is the persisted product marker.
type Identity struct {
	ProductID             string
	DatabaseSchemaVersion uint
	CreatedAtUTC          time.Time
}

// Inspection describes a database without mutating it.
type Inspection struct {
	Kind     Kind
	Identity *Identity
	Tables   []string
}

// ProductIDMismatchError prevents a database with a foreign product marker
// from being adopted by this product.
type ProductIDMismatchError struct {
	Found string
}

func (e *ProductIDMismatchError) Error() string {
	return fmt.Sprintf("database product_id %q does not match required product_id %q", e.Found, product.ID)
}

// SchemaVersionMismatchError prevents an unsupported product schema from
// being opened before a controlled migration path exists.
type SchemaVersionMismatchError struct {
	Found    uint
	Required uint
}

func (e *SchemaVersionMismatchError) Error() string {
	return fmt.Sprintf("database schema version %d does not match required version %d", e.Found, e.Required)
}

// Inspect classifies a SQLite database using read-only queries.
func Inspect(ctx context.Context, db *sql.DB) (Inspection, error) {
	tables, err := userTables(ctx, db)
	if err != nil {
		return Inspection{}, fmt.Errorf("listing database tables: %w", err)
	}

	inspection := Inspection{Tables: tables}
	if len(tables) == 0 {
		inspection.Kind = KindEmpty
		return inspection, nil
	}

	if contains(tables, identityTable) {
		identity, err := readIdentity(ctx, db)
		if err != nil {
			return Inspection{}, fmt.Errorf("reading product identity: %w", err)
		}

		inspection.Kind = KindProduct
		inspection.Identity = &identity
		return inspection, nil
	}

	if looksLikeLegacyStash(tables) {
		inspection.Kind = KindLegacyStash
		return inspection, nil
	}

	inspection.Kind = KindUnknown
	return inspection, nil
}

func userTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name
		FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tables, nil
}

func looksLikeLegacyStash(tables []string) bool {
	if !contains(tables, "schema_migrations") {
		return false
	}

	legacyCoreTables := []string{"galleries", "images", "performers", "scenes", "studios"}
	matches := 0
	for _, table := range legacyCoreTables {
		if contains(tables, table) {
			matches++
		}
	}

	// Requiring more than one core table avoids labelling an unrelated schema
	// as Stash. A partial or otherwise unknown database is still rejected.
	return matches >= 2
}

func contains(values []string, target string) bool {
	index := sort.SearchStrings(values, target)
	return index < len(values) && values[index] == target
}

func readIdentity(ctx context.Context, db *sql.DB) (Identity, error) {
	var (
		identity      Identity
		schemaVersion int64
		createdAt     string
		rowCount      int
	)

	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cgm_product_identity`).Scan(&rowCount); err != nil {
		return Identity{}, err
	}
	if rowCount != 1 {
		return Identity{}, fmt.Errorf("identity table must contain exactly one row, found %d", rowCount)
	}

	err := db.QueryRowContext(ctx, `
		SELECT product_id, database_schema_version, created_at_utc
		FROM cgm_product_identity
		WHERE singleton_id = 1
	`).Scan(&identity.ProductID, &schemaVersion, &createdAt)
	if err != nil {
		return Identity{}, err
	}
	if schemaVersion <= 0 {
		return Identity{}, fmt.Errorf("database schema version must be positive, found %d", schemaVersion)
	}

	identity.DatabaseSchemaVersion = uint(schemaVersion)
	identity.CreatedAtUTC, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Identity{}, fmt.Errorf("parsing identity creation time: %w", err)
	}

	return identity, nil
}

func validateInspection(inspection Inspection) error {
	switch inspection.Kind {
	case KindEmpty:
		return nil
	case KindLegacyStash:
		return ErrLegacyStashDatabase
	case KindUnknown:
		return ErrUnknownDatabase
	case KindProduct:
		if inspection.Identity == nil {
			return errors.New("product database inspection has no identity")
		}
		if inspection.Identity.ProductID != product.ID {
			return &ProductIDMismatchError{Found: inspection.Identity.ProductID}
		}
		if inspection.Identity.DatabaseSchemaVersion > product.DatabaseSchemaVersion {
			return &SchemaVersionMismatchError{
				Found:    inspection.Identity.DatabaseSchemaVersion,
				Required: product.DatabaseSchemaVersion,
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported database inspection kind %q", inspection.Kind)
	}
}

func initialiseIdentity(ctx context.Context, db *sql.DB, now time.Time) (Identity, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Identity{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE cgm_product_identity (
			singleton_id INTEGER NOT NULL PRIMARY KEY CHECK (singleton_id = 1),
			product_id TEXT NOT NULL,
			database_schema_version INTEGER NOT NULL CHECK (database_schema_version > 0),
			created_at_utc TEXT NOT NULL
		)
	`); err != nil {
		return Identity{}, fmt.Errorf("creating product identity table: %w", err)
	}
	if err := createSchemaV1(ctx, tx); err != nil {
		return Identity{}, err
	}
	if product.DatabaseSchemaVersion >= 2 {
		if err := createMediaProcessingSchemaV2(ctx, tx); err != nil {
			return Identity{}, err
		}
	}
	if product.DatabaseSchemaVersion >= 3 {
		if err := createSettingsSchemaV3(ctx, tx); err != nil {
			return Identity{}, err
		}
	}
	if product.DatabaseSchemaVersion >= 4 {
		if err := createMediaClassificationSchemaV4(ctx, tx); err != nil {
			return Identity{}, err
		}
	}
	if product.DatabaseSchemaVersion >= 5 {
		if err := createMediaExclusionSchemaV5(ctx, tx); err != nil {
			return Identity{}, err
		}
	}
	if product.DatabaseSchemaVersion >= 6 {
		if err := createAutomationSchemaV6(ctx, tx); err != nil {
			return Identity{}, err
		}
	}
	if product.DatabaseSchemaVersion >= 7 {
		if err := createCoverageSchemaV7(ctx, tx); err != nil {
			return Identity{}, err
		}
	}

	createdAt := now.UTC().Format(time.RFC3339Nano)
	persistedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Identity{}, fmt.Errorf("normalising identity creation time: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cgm_product_identity (
			singleton_id, product_id, database_schema_version, created_at_utc
		) VALUES (1, ?, ?, ?)
	`, product.ID, product.DatabaseSchemaVersion, createdAt); err != nil {
		return Identity{}, fmt.Errorf("writing product identity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Identity{}, err
	}

	return Identity{
		ProductID:             product.ID,
		DatabaseSchemaVersion: product.DatabaseSchemaVersion,
		CreatedAtUTC:          persistedCreatedAt,
	}, nil
}
