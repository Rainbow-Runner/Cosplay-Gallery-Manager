package productdb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"time"
)

type LibraryPathChange struct {
	LibraryID int64
	Path      string
	ChangedAt string
}

// File events are hints; durable source scans remain authoritative. Paths are
// checked against the saved root again when the event is processed.
func (db *Database) QueueLibraryPathChange(ctx context.Context, libraryID int64, path string, now time.Time) error {
	library, err := findLibrary(ctx, db.DB, libraryID)
	if err != nil {
		return err
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) || !pathWithin(library.RootPath, path) {
		return errors.New("changed path is outside library")
	}
	// Coalesce an entire known Gallery, not just repeat writes to one photo.
	// New/unbound paths retain their own path and use discovery as fallback.
	var sourcePath string
	err = db.QueryRowContext(ctx, `SELECT source_path FROM gallery_sources WHERE library_id=? AND (source_path=? OR source_path||'.cosplay.json'=? OR source_type='DIRECTORY' AND substr(?,1,length(source_path)+1)=source_path||'/') ORDER BY length(source_path) DESC LIMIT 1`, libraryID, path, path, path).Scan(&sourcePath)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		path = sourcePath
	}
	// Fixed-width timestamps sort chronologically, even at whole-second edges.
	stamp := normalisedTime(now).Format("2006-01-02T15:04:05.000000000Z")
	current := time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")
	_, err = db.ExecContext(ctx, `INSERT INTO library_path_changes(library_id,path,changed_at_utc) VALUES(?,?,?) ON CONFLICT(library_id,path) DO UPDATE SET changed_at_utc=CASE WHEN library_path_changes.changed_at_utc>? AND excluded.changed_at_utc<=? THEN excluded.changed_at_utc ELSE max(library_path_changes.changed_at_utc,excluded.changed_at_utc) END`, libraryID, path, stamp, current, current)
	return err
}

func (db *Database) PendingLibraryPathChanges(ctx context.Context, before time.Time, limit int) ([]LibraryPathChange, error) {
	rows, err := db.QueryContext(ctx, `SELECT change.library_id,change.path,change.changed_at_utc FROM library_path_changes change JOIN media_libraries library ON library.id=change.library_id WHERE library.enabled=1 AND change.changed_at_utc<=? ORDER BY change.changed_at_utc,change.library_id,change.path LIMIT ?`, normalisedTime(before).Format("2006-01-02T15:04:05.000000000Z"), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []LibraryPathChange
	for rows.Next() {
		var value LibraryPathChange
		if err := rows.Scan(&value.LibraryID, &value.Path, &value.ChangedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (db *Database) CompleteLibraryPathChange(ctx context.Context, change LibraryPathChange) error {
	_, err := db.ExecContext(ctx, `DELETE FROM library_path_changes WHERE library_id=? AND path=? AND changed_at_utc=?`, change.LibraryID, change.Path, change.ChangedAt)
	return err
}

// Resolve all paths in a batch before scanning, so 100 changed photos in the
// same Gallery produce one source scan rather than 100 scans.
func (db *Database) ChangedPathSourceIDs(ctx context.Context, change LibraryPathChange, now time.Time) ([]int64, error) {
	library, err := findLibrary(ctx, db.DB, change.LibraryID)
	if err != nil {
		return nil, err
	}
	if !library.Enabled || !pathWithin(library.RootPath, change.Path) {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT id,source_path,source_type FROM gallery_sources WHERE library_id=?`, change.LibraryID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		var path, kind string
		if err := rows.Scan(&id, &path, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		if path == change.Path || change.Path == path+".cosplay.json" || pathWithin(change.Path, path) || kind == "DIRECTORY" && pathWithin(path, change.Path) {
			ids = append(ids, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Even same-size, same-mtime file events invalidate cached discovery.
	if _, err := db.ExecContext(ctx, `DELETE FROM archive_discovery_cache WHERE library_id=? AND (path=? OR substr(path,1,length(?)+1)=?||'/')`, change.LibraryID, change.Path, change.Path, change.Path); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, err := db.ExecContext(ctx, `UPDATE gallery_sources SET reconcile_state='NEEDS_RESCAN',updated_at_utc=? WHERE id=? AND reconcile_state!='SCANNING'`, formatTime(normalisedTime(now)), id); err != nil {
			return nil, err
		}
	}
	return ids, nil
}
