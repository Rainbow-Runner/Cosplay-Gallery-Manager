package productdb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/product"
)

func TestOpenInitialisesEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "library.sqlite")

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("opening empty database: %v", err)
	}
	defer db.Close()

	identity := db.Identity()
	if identity.ProductID != product.ID {
		t.Fatalf("product ID = %q, want %q", identity.ProductID, product.ID)
	}
	if identity.DatabaseSchemaVersion != product.DatabaseSchemaVersion {
		t.Fatalf("schema version = %d, want %d", identity.DatabaseSchemaVersion, product.DatabaseSchemaVersion)
	}
	if identity.CreatedAtUTC.IsZero() {
		t.Fatal("identity creation time must be set")
	}

	var journalMode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("reading journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal mode = %q, want wal", journalMode)
	}

	var foreignKeys int
	if err := db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("reading foreign key mode: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign key mode = %d, want 1", foreignKeys)
	}
}

func TestOpenExistingProductDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "library.sqlite")

	first, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("initial open: %v", err)
	}
	want := first.Identity()
	if err := first.Close(); err != nil {
		t.Fatalf("closing initial database: %v", err)
	}

	second, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopening product database: %v", err)
	}
	defer second.Close()

	if got := second.Identity(); got != want {
		t.Fatalf("identity changed after reopen: got %#v, want %#v", got, want)
	}
}

func TestOpenRejectsLegacyStashDatabaseWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stash.sqlite")
	seedDatabase(t, path, `
		CREATE TABLE schema_migrations (version INTEGER NOT NULL, dirty INTEGER NOT NULL);
		CREATE TABLE scenes (id INTEGER PRIMARY KEY);
		CREATE TABLE performers (id INTEGER PRIMARY KEY);
	`)

	db, err := Open(context.Background(), path)
	if db != nil {
		db.Close()
		t.Fatal("legacy Stash database must not be opened")
	}
	if !errors.Is(err, ErrLegacyStashDatabase) {
		t.Fatalf("error = %v, want ErrLegacyStashDatabase", err)
	}
	assertIdentityTableAbsent(t, path)
}

func TestOpenRejectsUnknownDatabaseWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unknown.sqlite")
	seedDatabase(t, path, `CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT)`)

	db, err := Open(context.Background(), path)
	if db != nil {
		db.Close()
		t.Fatal("unknown database must not be opened")
	}
	if !errors.Is(err, ErrUnknownDatabase) {
		t.Fatalf("error = %v, want ErrUnknownDatabase", err)
	}
	assertIdentityTableAbsent(t, path)
}

func TestOpenRejectsMismatchedProductID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other-product.sqlite")
	seedIdentityDatabase(t, path, "some-other-product", product.DatabaseSchemaVersion)

	db, err := Open(context.Background(), path)
	if db != nil {
		db.Close()
		t.Fatal("foreign product database must not be opened")
	}

	var mismatch *ProductIDMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("error = %v, want ProductIDMismatchError", err)
	}
}

func TestOpenRejectsMismatchedSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.sqlite")
	seedIdentityDatabase(t, path, product.ID, product.DatabaseSchemaVersion+1)

	db, err := Open(context.Background(), path)
	if db != nil {
		db.Close()
		t.Fatal("unsupported product schema must not be opened")
	}

	var mismatch *SchemaVersionMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("error = %v, want SchemaVersionMismatchError", err)
	}
}

func TestOpenRejectsIncompleteProductSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "incomplete-product.sqlite")
	seedIdentityDatabase(t, path, product.ID, product.DatabaseSchemaVersion)

	db, err := Open(context.Background(), path)
	if db != nil {
		db.Close()
		t.Fatal("incomplete product database must not be opened")
	}
	if !errors.Is(err, ErrInvalidProductSchema) {
		t.Fatalf("error = %v, want ErrInvalidProductSchema", err)
	}
}

func TestOpenRejectsProductSchemaWithMissingPermanentHistoryTrigger(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "missing-trigger.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("initialising product database: %v", err)
	}
	if _, err := db.Exec(`DROP TRIGGER portable_uuid_tombstone_immutable_delete`); err != nil {
		t.Fatalf("removing trigger for test: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing product database: %v", err)
	}

	db, err = Open(ctx, path)
	if db != nil {
		db.Close()
		t.Fatal("product database without a required trigger must not be opened")
	}
	if !errors.Is(err, ErrInvalidProductSchema) {
		t.Fatalf("error = %v, want ErrInvalidProductSchema", err)
	}
}

func seedDatabase(t *testing.T, path string, schema string) {
	t.Helper()
	db, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatalf("opening seed database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("seeding database: %v", err)
	}
}

func seedIdentityDatabase(t *testing.T, path string, productID string, schemaVersion uint) {
	t.Helper()
	seedDatabase(t, path, `
		CREATE TABLE cgm_product_identity (
			singleton_id INTEGER NOT NULL PRIMARY KEY,
			product_id TEXT NOT NULL,
			database_schema_version INTEGER NOT NULL,
			created_at_utc TEXT NOT NULL
		)
	`)

	db, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatalf("opening identity seed database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		INSERT INTO cgm_product_identity (
			singleton_id, product_id, database_schema_version, created_at_utc
		) VALUES (1, ?, ?, '2026-07-22T00:00:00Z')
	`, productID, schemaVersion); err != nil {
		t.Fatalf("seeding product identity: %v", err)
	}
}

func assertIdentityTableAbsent(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open(sqliteDriver, sqliteDSN(path, true))
	if err != nil {
		t.Fatalf("opening database for assertion: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_schema
		WHERE type = 'table' AND name = ?
	`, identityTable).Scan(&count); err != nil {
		t.Fatalf("checking product identity table: %v", err)
	}
	if count != 0 {
		t.Fatal("rejected database was mutated with a product identity table")
	}
}
