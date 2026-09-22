package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stashapp/stash/internal/product"
)

const sqliteDriver = "sqlite3"

// Database is a product-identified SQLite connection. Callers cannot obtain
// one through Open unless the file is empty or already belongs to this product.
type Database struct {
	*sql.DB

	identity Identity
	path     string
}

// Open validates database identity before enabling the writable WAL
// connection. Existing original Stash or unknown non-empty databases are
// inspected read-only and refused without creating product tables.
func Open(ctx context.Context, path string) (*Database, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving database path: %w", err)
	}

	exists, err := regularFileExists(absolutePath)
	if err != nil {
		return nil, err
	}

	if exists {
		inspection, err := inspectFile(ctx, absolutePath)
		if err != nil {
			return nil, err
		}
		if err := validateInspection(inspection); err != nil {
			return nil, err
		}
	}

	connection, err := sql.Open(sqliteDriver, sqliteDSN(absolutePath, false))
	if err != nil {
		return nil, fmt.Errorf("opening writable database: %w", err)
	}
	connection.SetMaxOpenConns(1)
	connection.SetMaxIdleConns(1)

	closeOnError := func(openErr error) (*Database, error) {
		_ = connection.Close()
		return nil, openErr
	}

	if err := connection.PingContext(ctx); err != nil {
		return closeOnError(fmt.Errorf("connecting to writable database: %w", err))
	}
	if err := validateIntegrity(ctx, connection); err != nil {
		return closeOnError(err)
	}

	// Re-inspect on the writable connection so a file changed between the
	// read-only inspection and this point is never silently adopted.
	inspection, err := Inspect(ctx, connection)
	if err != nil {
		return closeOnError(err)
	}
	if err := validateInspection(inspection); err != nil {
		return closeOnError(err)
	}
	if inspection.Kind == KindProduct {
		if err := validateSchemaVersion(ctx, connection, inspection.Identity.DatabaseSchemaVersion); err != nil {
			return closeOnError(err)
		}
	}

	var identity Identity
	if inspection.Kind == KindEmpty {
		identity, err = initialiseIdentity(ctx, connection, time.Now())
		if err != nil {
			return closeOnError(fmt.Errorf("initialising database identity: %w", err))
		}
		if err := validateSchemaV16(ctx, connection); err != nil {
			return closeOnError(err)
		}
		if err := validateIntegrity(ctx, connection); err != nil {
			return closeOnError(err)
		}
	} else {
		identity = *inspection.Identity
		if identity.DatabaseSchemaVersion < product.DatabaseSchemaVersion {
			if err := migrateProductDatabase(ctx, connection, absolutePath, identity.DatabaseSchemaVersion); err != nil {
				return closeOnError(err)
			}
			identity.DatabaseSchemaVersion = product.DatabaseSchemaVersion
		}
	}

	if err := configureConnection(ctx, connection); err != nil {
		return closeOnError(err)
	}

	return &Database{
		DB:       connection,
		identity: identity,
		path:     absolutePath,
	}, nil
}

func inspectFile(ctx context.Context, path string) (Inspection, error) {
	connection, err := sql.Open(sqliteDriver, sqliteDSN(path, true))
	if err != nil {
		return Inspection{}, fmt.Errorf("opening database for identity inspection: %w", err)
	}
	defer connection.Close()

	if err := connection.PingContext(ctx); err != nil {
		return Inspection{}, fmt.Errorf("connecting for database identity inspection: %w", err)
	}
	if err := validateIntegrity(ctx, connection); err != nil {
		return Inspection{}, err
	}

	inspection, err := Inspect(ctx, connection)
	if err != nil {
		return Inspection{}, err
	}
	if err := validateInspection(inspection); err != nil {
		return Inspection{}, err
	}
	if inspection.Kind == KindProduct {
		if err := validateSchemaVersion(ctx, connection, inspection.Identity.DatabaseSchemaVersion); err != nil {
			return Inspection{}, err
		}
	}
	return inspection, nil
}

func validateSchemaVersion(ctx context.Context, db *sql.DB, version uint) error {
	switch version {
	case 1:
		return validateSchemaV1(ctx, db)
	case 2:
		return validateSchemaV2(ctx, db)
	case 3:
		return validateSchemaV3(ctx, db)
	case 4:
		return validateSchemaV4(ctx, db)
	case 5:
		return validateSchemaV5(ctx, db)
	case 6:
		return validateSchemaV6(ctx, db)
	case 7:
		return validateSchemaV7(ctx, db)
	case 8:
		return validateSchemaV8(ctx, db)
	case 9:
		return validateSchemaV9(ctx, db)
	case 10:
		return validateSchemaV10(ctx, db)
	case 11:
		return validateSchemaV11(ctx, db)
	case 12:
		return validateSchemaV12(ctx, db)
	case 13:
		return validateSchemaV13(ctx, db)
	case 14:
		return validateSchemaV14(ctx, db)
	case 15:
		return validateSchemaV15(ctx, db)
	case 16:
		return validateSchemaV16(ctx, db)
	default:
		return &SchemaVersionMismatchError{Found: version, Required: product.DatabaseSchemaVersion}
	}
}

func migrateProductDatabase(ctx context.Context, connection *sql.DB, databasePath string, from uint) error {
	if from < 1 || from >= product.DatabaseSchemaVersion || product.DatabaseSchemaVersion != 16 {
		return &SchemaVersionMismatchError{Found: from, Required: product.DatabaseSchemaVersion}
	}
	backupPath := fmt.Sprintf("%s.pre-schema-v%d-%d.bak", databasePath, from, time.Now().UTC().UnixNano())
	if err := createMigrationSnapshot(ctx, connection, backupPath, from); err != nil {
		return fmt.Errorf("creating pre-migration database snapshot: %w", err)
	}
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if from < 2 {
		if err := createMediaProcessingSchemaV2(ctx, tx); err != nil {
			return err
		}
	}
	if from < 3 {
		if err := createSettingsSchemaV3(ctx, tx); err != nil {
			return err
		}
	}
	if from < 4 {
		if err := createMediaClassificationSchemaV4(ctx, tx); err != nil {
			return err
		}
	}
	if from < 5 {
		if err := createMediaExclusionSchemaV5(ctx, tx); err != nil {
			return err
		}
	}
	if from < 6 {
		if err := createAutomationSchemaV6(ctx, tx); err != nil {
			return err
		}
	}
	if from < 7 {
		if err := createCoverageSchemaV7(ctx, tx); err != nil {
			return err
		}
	}
	if from < 8 {
		if err := createArchiveDiscoverySchemaV8(ctx, tx); err != nil {
			return err
		}
	}
	if from < 9 {
		if err := createPortableImportSchemaV9(ctx, tx); err != nil {
			return err
		}
	}
	if from < 10 {
		if err := createPortableMergeSchemaV10(ctx, tx); err != nil {
			return err
		}
	}
	if from < 11 {
		if err := createSettingsSchemaV11(ctx, tx); err != nil {
			return err
		}
	}
	if from < 12 {
		if err := createTagAssignmentSchemaV12(ctx, tx); err != nil {
			return err
		}
	}
	if from < 13 {
		if err := createManifestInspectionSchemaV13(ctx, tx); err != nil {
			return err
		}
	}
	if from < 14 {
		if err := createMetadataWritebackSchemaV14(ctx, tx); err != nil {
			return err
		}
	}
	if from < 15 {
		if err := createCaptureDateSchemaV15(ctx, tx); err != nil {
			return err
		}
	}
	if from < 16 {
		if err := createRecoverySchemaV16(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cgm_product_identity SET database_schema_version=16 WHERE singleton_id=1 AND database_schema_version=?`, from); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := validateSchemaV16(ctx, connection); err != nil {
		return fmt.Errorf("validating migrated schema: %w", err)
	}
	return validateIntegrity(ctx, connection)
}

func createMigrationSnapshot(ctx context.Context, source *sql.DB, targetPath string, expectedVersion uint) (returnErr error) {
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(targetPath)
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(targetPath)
		}
	}()
	destination, err := sql.Open(sqliteDriver, sqliteDSN(targetPath, false))
	if err != nil {
		return err
	}
	destination.SetMaxOpenConns(1)
	if err := runOnlineBackup(ctx, source, destination); err != nil {
		_ = destination.Close()
		return err
	}
	if err := destination.Close(); err != nil {
		return err
	}
	check, err := sql.Open(sqliteDriver, sqliteDSN(targetPath, true))
	if err != nil {
		return err
	}
	defer check.Close()
	if err := validateIntegrity(ctx, check); err != nil {
		return err
	}
	identity, err := readIdentity(ctx, check)
	if err != nil || identity.ProductID != product.ID || identity.DatabaseSchemaVersion != expectedVersion {
		return errors.New("migration snapshot identity validation failed")
	}
	if err := validateSchemaVersion(ctx, check, expectedVersion); err != nil {
		return err
	}
	complete = true
	return nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking database path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("database path is not a regular file: %s", path)
	}
	return true, nil
}

func sqliteDSN(path string, readOnly bool) string {
	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	dsn := &url.URL{Scheme: "file", Path: uriPath}
	query := dsn.Query()
	query.Set("_busy_timeout", "5000")
	query.Set("_fk", "true")
	if readOnly {
		query.Set("mode", "ro")
		query.Set("_query_only", "true")
	} else {
		query.Set("mode", "rwc")
		query.Set("_txlock", "immediate")
	}
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

func configureConnection(ctx context.Context, db *sql.DB) error {
	var journalMode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode = WAL`).Scan(&journalMode); err != nil {
		return fmt.Errorf("enabling SQLite WAL: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("enabling SQLite WAL returned mode %q", journalMode)
	}

	if _, err := db.ExecContext(ctx, `PRAGMA synchronous = NORMAL`); err != nil {
		return fmt.Errorf("configuring SQLite synchronous mode: %w", err)
	}

	return nil
}

// Identity returns the immutable product marker loaded during Open.
func (db *Database) Identity() Identity {
	return db.identity
}

// Path returns the absolute local database path.
func (db *Database) Path() string {
	return db.path
}
