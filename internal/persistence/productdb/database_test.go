package productdb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestOpenMigratesSchemaV1AfterValidatedSnapshot(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "library.sqlite")
	connection, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE cgm_product_identity(singleton_id INTEGER NOT NULL PRIMARY KEY CHECK(singleton_id=1),product_id TEXT NOT NULL,database_schema_version INTEGER NOT NULL,created_at_utc TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := createSchemaV1(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cgm_product_identity VALUES(1,?,1,'2026-08-15T00:00:00Z')`, product.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrating schema v1: %v", err)
	}
	defer db.Close()
	if db.Identity().DatabaseSchemaVersion != product.DatabaseSchemaVersion {
		t.Fatalf("schema version = %d", db.Identity().DatabaseSchemaVersion)
	}
	var tableCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name='video_technical_metadata'`).Scan(&tableCount); err != nil || tableCount != 1 {
		t.Fatalf("video table count=%d err=%v", tableCount, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".pre-schema-v1-") {
			snapshot = filepath.Join(directory, entry.Name())
		}
	}
	if snapshot == "" {
		t.Fatal("pre-migration snapshot was not created")
	}
	backup, err := sql.Open(sqliteDriver, sqliteDSN(snapshot, true))
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	identity, err := readIdentity(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	if identity.DatabaseSchemaVersion != 1 {
		t.Fatalf("snapshot schema version=%d", identity.DatabaseSchemaVersion)
	}
}

func TestOpenMigratesSchemaV2SettingsAfterValidatedSnapshot(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "library.sqlite")
	connection, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE cgm_product_identity(singleton_id INTEGER NOT NULL PRIMARY KEY CHECK(singleton_id=1),product_id TEXT NOT NULL,database_schema_version INTEGER NOT NULL,created_at_utc TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := createSchemaV1(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := createMediaProcessingSchemaV2(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runtime_settings SET home_scope='ALL' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cgm_product_identity VALUES(1,?,2,'2026-08-16T00:00:00Z')`, product.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrating schema v2: %v", err)
	}
	defer db.Close()
	settings, err := db.Settings().Find(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if db.Identity().DatabaseSchemaVersion != product.DatabaseSchemaVersion || settings.HomeScope != "ALL" ||
		settings.GalleryAnimatedPlaybackLimit != 12 || settings.GalleryAnimatedLockIntervalMS != 800 {
		t.Fatalf("migrated identity/settings = %#v / %#v", db.Identity(), settings)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".pre-schema-v2-") {
			snapshot = filepath.Join(directory, entry.Name())
		}
	}
	if snapshot == "" {
		t.Fatal("schema v2 pre-migration snapshot was not created")
	}
	backup, err := sql.Open(sqliteDriver, sqliteDSN(snapshot, true))
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	identity, err := readIdentity(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	if identity.DatabaseSchemaVersion != 2 {
		t.Fatalf("snapshot schema version=%d", identity.DatabaseSchemaVersion)
	}
	if err := validateSettingsSchemaV3(ctx, backup); err == nil {
		t.Fatal("schema v2 snapshot unexpectedly contains schema v3 settings")
	}
}

func TestOpenMigratesSchemaV3MediaClassificationAfterValidatedSnapshot(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "library.sqlite")
	connection, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE cgm_product_identity(singleton_id INTEGER NOT NULL PRIMARY KEY CHECK(singleton_id=1),product_id TEXT NOT NULL,database_schema_version INTEGER NOT NULL,created_at_utc TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := createSchemaV1(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := createMediaProcessingSchemaV2(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := createSettingsSchemaV3(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO portable_uuid_registry(uuid,entity_kind,created_at_utc) VALUES
			('11111111-1111-4111-8111-111111111111','GALLERY','2026-08-17T00:00:00Z'),
			('22222222-2222-4222-8222-222222222222','GALLERY_ITEM','2026-08-17T00:00:00Z');
		INSERT INTO galleries(id,set_id,slug,state,title,created_at_utc,updated_at_utc) VALUES
			(1,'11111111-1111-4111-8111-111111111111','migration-gallery','DRAFT','Migration','2026-08-17T00:00:00Z','2026-08-17T00:00:00Z');
		INSERT INTO gallery_sources(id,gallery_id,source_type,source_path,availability_state,reconcile_state,created_at_utc,updated_at_utc) VALUES
			(1,1,'DIRECTORY',?,'AVAILABLE','IN_SYNC','2026-08-17T00:00:00Z','2026-08-17T00:00:00Z');
		INSERT INTO gallery_items(id,item_uuid,gallery_id,source_id,relative_path,media_kind,content_format,image_category,position,availability_state,processing_state,created_at_utc,updated_at_utc) VALUES
			(1,'22222222-2222-4222-8222-222222222222',1,1,'selfie/a.jpg','STATIC_IMAGE','IMAGE','PHOTO',1024,'AVAILABLE','READY','2026-08-17T00:00:00Z','2026-08-17T00:00:00Z');
		INSERT INTO gallery_item_suggestions(gallery_id,item_uuid,suggestion_kind,value,source_kind,status,created_at_utc,resolved_at_utc) VALUES
			(1,'22222222-2222-4222-8222-222222222222','SELFIE_CATEGORY','SELFIE','DIRECTORY_SEMANTIC','REJECTED','2026-08-17T00:00:00Z','2026-08-17T00:01:00Z')
	`, filepath.Join(directory, "media")); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cgm_product_identity VALUES(1,?,3,'2026-08-17T00:00:00Z')`, product.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrating schema v3: %v", err)
	}
	defer db.Close()
	var defaults int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_classification_rules WHERE system_default=1`).Scan(&defaults); err != nil || defaults != 2 {
		t.Fatalf("default rules=%d err=%v", defaults, err)
	}
	var migratedStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM media_classification_suggestions WHERE item_uuid='22222222-2222-4222-8222-222222222222'`).Scan(&migratedStatus); err != nil || migratedStatus != "REJECTED" {
		t.Fatalf("migrated legacy status=%q err=%v", migratedStatus, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".pre-schema-v3-") {
			snapshot = filepath.Join(directory, entry.Name())
		}
	}
	if snapshot == "" {
		t.Fatal("schema v3 pre-migration snapshot was not created")
	}
	backup, err := sql.Open(sqliteDriver, sqliteDSN(snapshot, true))
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	identity, err := readIdentity(ctx, backup)
	if err != nil || identity.DatabaseSchemaVersion != 3 {
		t.Fatalf("snapshot identity=%#v err=%v", identity, err)
	}
	if err := validateMediaClassificationSchemaV4(ctx, backup); err == nil {
		t.Fatal("schema v3 snapshot unexpectedly contains schema v4 rules")
	}
}

func TestOpenMigratesSchemaV4MediaExclusionWithoutChangingItems(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "library.sqlite")
	connection, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE cgm_product_identity(singleton_id INTEGER NOT NULL PRIMARY KEY CHECK(singleton_id=1),product_id TEXT NOT NULL,database_schema_version INTEGER NOT NULL,created_at_utc TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := createSchemaV1(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := createMediaProcessingSchemaV2(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := createSettingsSchemaV3(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := createMediaClassificationSchemaV4(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO portable_uuid_registry(uuid,entity_kind,created_at_utc) VALUES
			('11111111-1111-4111-8111-111111111111','GALLERY','2026-08-28T00:00:00Z'),
			('22222222-2222-4222-8222-222222222222','GALLERY_ITEM','2026-08-28T00:00:00Z');
		INSERT INTO galleries(id,set_id,slug,state,title,created_at_utc,updated_at_utc) VALUES
			(1,'11111111-1111-4111-8111-111111111111','schema-v4-gallery','DRAFT','Schema v4','2026-08-28T00:00:00Z','2026-08-28T00:00:00Z');
		INSERT INTO gallery_sources(id,gallery_id,source_type,source_path,availability_state,reconcile_state,created_at_utc,updated_at_utc) VALUES
			(1,1,'DIRECTORY',?,'AVAILABLE','IN_SYNC','2026-08-28T00:00:00Z','2026-08-28T00:00:00Z');
		INSERT INTO gallery_items(id,item_uuid,gallery_id,source_id,relative_path,media_kind,content_format,image_category,position,availability_state,processing_state,excluded,created_at_utc,updated_at_utc) VALUES
			(1,'22222222-2222-4222-8222-222222222222',1,1,'excluded/a.jpg','STATIC_IMAGE','IMAGE','PHOTO',1024,'AVAILABLE','READY',1,'2026-08-28T00:00:00Z','2026-08-28T00:00:00Z');
		INSERT INTO cgm_product_identity VALUES(1,?,4,'2026-08-28T00:00:00Z')
	`, filepath.Join(directory, "media"), product.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrating schema v4: %v", err)
	}
	defer db.Close()
	if db.Identity().DatabaseSchemaVersion != product.DatabaseSchemaVersion {
		t.Fatalf("schema version=%d", db.Identity().DatabaseSchemaVersion)
	}
	var excluded, rules, decisions int
	if err := db.QueryRowContext(ctx, `SELECT excluded FROM gallery_items WHERE id=1`).Scan(&excluded); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_exclusion_rules`).Scan(&rules); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_exclusion_decisions`).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if excluded != 1 || rules != 0 || decisions != 0 {
		t.Fatalf("migration changed existing policy state: excluded=%d rules=%d decisions=%d", excluded, rules, decisions)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".pre-schema-v4-") {
			snapshot = filepath.Join(directory, entry.Name())
		}
	}
	if snapshot == "" {
		t.Fatal("schema v4 pre-migration snapshot was not created")
	}
	backup, err := sql.Open(sqliteDriver, sqliteDSN(snapshot, true))
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	identity, err := readIdentity(ctx, backup)
	if err != nil || identity.DatabaseSchemaVersion != 4 {
		t.Fatalf("snapshot identity=%#v err=%v", identity, err)
	}
	if err := validateSchemaV4(ctx, backup); err != nil {
		t.Fatalf("schema v4 snapshot is invalid: %v", err)
	}
	if err := validateMediaExclusionSchemaV5(ctx, backup); err == nil {
		t.Fatal("schema v4 snapshot unexpectedly contains schema v5 rules")
	}
}

func TestOpenMigratesSchemaV5AutomationAsOptIn(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "library.sqlite")
	connection, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE cgm_product_identity(singleton_id INTEGER NOT NULL PRIMARY KEY CHECK(singleton_id=1),product_id TEXT NOT NULL,database_schema_version INTEGER NOT NULL,created_at_utc TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, create := range []func(context.Context, *sql.Tx) error{
		createSchemaV1, createMediaProcessingSchemaV2, createSettingsSchemaV3,
		createMediaClassificationSchemaV4, createMediaExclusionSchemaV5,
	} {
		if err := create(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO media_libraries(id,name,root_path,enabled,read_only,capture_timezone,created_at_utc,updated_at_utc)
		VALUES(1,'Existing',?,1,1,'UTC','2026-08-29T00:00:00Z','2026-08-29T00:00:00Z');
		INSERT INTO cgm_product_identity VALUES(1,?,5,'2026-08-29T00:00:00Z')`, filepath.Join(directory, "media"), product.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrating schema v5: %v", err)
	}
	defer db.Close()
	policy, err := db.Automation().FindPolicy(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode != AutomationManual || policy.Revision != 0 {
		t.Fatalf("migrated policy = %#v, want implicit MANUAL", policy)
	}
	var storedPolicies, storedRuns, storedIssues int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_automation_policies`).Scan(&storedPolicies); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_automation_runs`).Scan(&storedRuns); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_automation_run_issues`).Scan(&storedIssues); err != nil {
		t.Fatal(err)
	}
	if storedPolicies != 0 || storedRuns != 0 || storedIssues != 0 {
		t.Fatalf("migration unexpectedly opted into automation: policies=%d runs=%d issues=%d", storedPolicies, storedRuns, storedIssues)
	}
	var coverageSummaries, coverageDiagnostics int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_coverage_summaries`).Scan(&coverageSummaries); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_coverage_diagnostics`).Scan(&coverageDiagnostics); err != nil {
		t.Fatal(err)
	}
	if coverageSummaries != 0 || coverageDiagnostics != 0 {
		t.Fatalf("migration created coverage data: summaries=%d diagnostics=%d", coverageSummaries, coverageDiagnostics)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".pre-schema-v5-") {
			snapshot = filepath.Join(directory, entry.Name())
		}
	}
	if snapshot == "" {
		t.Fatal("schema v5 pre-migration snapshot was not created")
	}
	backup, err := sql.Open(sqliteDriver, sqliteDSN(snapshot, true))
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	identity, err := readIdentity(ctx, backup)
	if err != nil || identity.DatabaseSchemaVersion != 5 {
		t.Fatalf("snapshot identity=%#v err=%v", identity, err)
	}
	if err := validateAutomationSchemaV6(ctx, backup); err == nil {
		t.Fatal("schema v5 snapshot unexpectedly contains schema v6 automation tables")
	}
}

func TestOpenMigratesSchemaV6CoverageAsEmpty(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "library.sqlite")
	connection, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil { t.Fatal(err) }
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil { t.Fatal(err) }
	if _, err := tx.ExecContext(ctx, `CREATE TABLE cgm_product_identity(singleton_id INTEGER NOT NULL PRIMARY KEY CHECK(singleton_id=1),product_id TEXT NOT NULL,database_schema_version INTEGER NOT NULL,created_at_utc TEXT NOT NULL)`); err != nil { t.Fatal(err) }
	for _, create := range []func(context.Context, *sql.Tx) error{
		createSchemaV1, createMediaProcessingSchemaV2, createSettingsSchemaV3,
		createMediaClassificationSchemaV4, createMediaExclusionSchemaV5, createAutomationSchemaV6,
	} {
		if err := create(ctx, tx); err != nil { t.Fatal(err) }
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cgm_product_identity VALUES(1,?,6,'2026-08-30T00:00:00Z')`, product.ID); err != nil { t.Fatal(err) }
	if err := tx.Commit(); err != nil { t.Fatal(err) }
	if err := connection.Close(); err != nil { t.Fatal(err) }
	db, err := Open(ctx, path)
	if err != nil { t.Fatalf("migrating schema v6: %v", err) }
	defer db.Close()
	if db.Identity().DatabaseSchemaVersion != 7 { t.Fatalf("schema version = %d", db.Identity().DatabaseSchemaVersion) }
	for _, table := range requiredCoverageSchemaV7Tables {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != 0 { t.Fatalf("%s count=%d err=%v", table, count, err) }
	}
	entries, err := os.ReadDir(directory)
	if err != nil { t.Fatal(err) }
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".pre-schema-v6-") {
			backup, err := sql.Open(sqliteDriver, sqliteDSN(filepath.Join(directory, entry.Name()), true))
			if err != nil { t.Fatal(err) }
			defer backup.Close()
			identity, err := readIdentity(ctx, backup)
			if err != nil || identity.DatabaseSchemaVersion != 6 { t.Fatalf("snapshot identity=%#v err=%v", identity, err) }
			if err := validateCoverageSchemaV7(ctx, backup); err == nil { t.Fatal("schema v6 snapshot unexpectedly contains coverage tables") }
			return
		}
	}
	t.Fatal("schema v6 pre-migration snapshot was not created")
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
