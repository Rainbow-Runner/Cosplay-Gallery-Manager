package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
)

var ErrBackupTargetExists = errors.New("backup target already exists")

// Backup creates a consistent SQLite online-backup snapshot. It never
// overwrites an existing path and does not depend on copying WAL sidecar files.
func (db *Database) Backup(ctx context.Context, targetPath string) (returnErr error) {
	absoluteTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolving backup target: %w", err)
	}
	if absoluteTarget == db.path {
		return errors.New("backup target must differ from the active database")
	}

	targetFile, err := os.OpenFile(absoluteTarget, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%w: %s", ErrBackupTargetExists, absoluteTarget)
	}
	if err != nil {
		return fmt.Errorf("reserving backup target: %w", err)
	}
	if err := targetFile.Close(); err != nil {
		_ = os.Remove(absoluteTarget)
		return fmt.Errorf("closing reserved backup target: %w", err)
	}

	completed := false
	defer func() {
		if completed {
			return
		}
		for _, path := range []string{
			absoluteTarget,
			absoluteTarget + "-journal",
			absoluteTarget + "-shm",
			absoluteTarget + "-wal",
		} {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && returnErr == nil {
				returnErr = fmt.Errorf("cleaning incomplete backup %s: %w", path, err)
			}
		}
	}()

	destination, err := sql.Open(sqliteDriver, sqliteDSN(absoluteTarget, false))
	if err != nil {
		return fmt.Errorf("opening backup target: %w", err)
	}
	destination.SetMaxOpenConns(1)
	destination.SetMaxIdleConns(1)

	if err := runOnlineBackup(ctx, db.DB, destination); err != nil {
		_ = destination.Close()
		return err
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("closing backup target: %w", err)
	}

	inspection, err := inspectFile(ctx, absoluteTarget)
	if err != nil {
		return fmt.Errorf("validating completed backup: %w", err)
	}
	if inspection.Kind != KindProduct {
		return fmt.Errorf("validating completed backup: unexpected database kind %s", inspection.Kind)
	}

	completed = true
	return nil
}

func runOnlineBackup(ctx context.Context, source *sql.DB, destination *sql.DB) error {
	sourceConnection, err := source.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquiring backup source connection: %w", err)
	}
	defer sourceConnection.Close()

	destinationConnection, err := destination.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquiring backup destination connection: %w", err)
	}
	defer destinationConnection.Close()

	return destinationConnection.Raw(func(destinationDriver interface{}) error {
		destinationSQLite, ok := destinationDriver.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("unexpected backup destination driver %T", destinationDriver)
		}

		return sourceConnection.Raw(func(sourceDriver interface{}) error {
			sourceSQLite, ok := sourceDriver.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("unexpected backup source driver %T", sourceDriver)
			}

			backup, err := destinationSQLite.Backup("main", sourceSQLite, "main")
			if err != nil {
				return fmt.Errorf("starting SQLite online backup: %w", err)
			}

			for {
				done, stepErr := backup.Step(256)
				if stepErr != nil {
					_ = backup.Finish()
					return fmt.Errorf("copying SQLite backup pages: %w", stepErr)
				}
				if done {
					if err := backup.Finish(); err != nil {
						return fmt.Errorf("finishing SQLite online backup: %w", err)
					}
					return nil
				}

				timer := time.NewTimer(10 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					_ = backup.Finish()
					return ctx.Err()
				case <-timer.C:
				}
			}
		})
	})
}
