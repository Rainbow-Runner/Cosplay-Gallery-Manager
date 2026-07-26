package productdb

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
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
		if err := validateSchemaV1(ctx, connection); err != nil {
			return closeOnError(err)
		}
	}

	var identity Identity
	if inspection.Kind == KindEmpty {
		identity, err = initialiseIdentity(ctx, connection, time.Now())
		if err != nil {
			return closeOnError(fmt.Errorf("initialising database identity: %w", err))
		}
		if err := validateSchemaV1(ctx, connection); err != nil {
			return closeOnError(err)
		}
		if err := validateIntegrity(ctx, connection); err != nil {
			return closeOnError(err)
		}
	} else {
		identity = *inspection.Identity
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
		if err := validateSchemaV1(ctx, connection); err != nil {
			return Inspection{}, err
		}
	}
	return inspection, nil
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
