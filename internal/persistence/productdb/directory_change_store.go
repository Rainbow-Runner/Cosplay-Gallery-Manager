package productdb

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"time"
)

type directoryMemberStamp struct {
	size     int64
	modified string
}
type directoryChangeSource struct {
	id, revision   int64
	prior, current map[string]directoryMemberStamp
}
type directoryChangeDetector map[string]*directoryChangeSource

func loadDirectoryChangeDetector(ctx context.Context, db *sql.DB, libraryID int64) (directoryChangeDetector, error) {
	rows, err := db.QueryContext(ctx, `SELECT source.id,source.source_path,gallery.scan_revision FROM gallery_sources source JOIN galleries gallery ON gallery.id=source.gallery_id WHERE source.library_id=? AND source.source_type='DIRECTORY' AND source.reconcile_state='IN_SYNC'`, libraryID)
	if err != nil {
		return nil, err
	}
	result := directoryChangeDetector{}
	byID := map[int64]*directoryChangeSource{}
	for rows.Next() {
		var id, rev int64
		var path string
		if err := rows.Scan(&id, &path, &rev); err != nil {
			rows.Close()
			return nil, err
		}
		value := &directoryChangeSource{id: id, revision: rev, prior: map[string]directoryMemberStamp{}, current: map[string]directoryMemberStamp{}}
		result[filepath.Clean(path)] = value
		byID[id] = value
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, `SELECT source.id,item.relative_path,item.byte_size,COALESCE(item.source_modified_at_utc,'') FROM gallery_items item JOIN gallery_sources source ON source.id=item.source_id WHERE source.library_id=? AND source.source_type='DIRECTORY' AND source.reconcile_state='IN_SYNC' AND item.availability_state='AVAILABLE'`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var path string
		var stamp directoryMemberStamp
		if err := rows.Scan(&id, &path, &stamp.size, &stamp.modified); err != nil {
			return nil, err
		}
		if value := byID[id]; value != nil {
			value.prior[path] = stamp
		}
	}
	return result, rows.Err()
}

func (d directoryChangeDetector) observe(path string, info os.FileInfo) {
	for root := filepath.Dir(path); ; root = filepath.Dir(root) {
		if source := d[root]; source != nil {
			relative, err := filepath.Rel(root, path)
			if err == nil {
				source.current[filepath.ToSlash(relative)] = directoryMemberStamp{info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano)}
			}
		}
		if parent := filepath.Dir(root); parent == root {
			break
		}
	}
}

func (d directoryChangeDetector) mark(ctx context.Context, db *sql.DB, now time.Time) error {
	for _, source := range d {
		changed := len(source.prior) != len(source.current)
		if !changed {
			for path, stamp := range source.prior {
				if current, ok := source.current[path]; !ok || current != stamp {
					changed = true
					break
				}
			}
		}
		if changed {
			if _, err := db.ExecContext(ctx, `UPDATE gallery_sources SET reconcile_state='NEEDS_RESCAN',updated_at_utc=? WHERE id=? AND reconcile_state='IN_SYNC' AND EXISTS(SELECT 1 FROM galleries WHERE id=gallery_sources.gallery_id AND scan_revision=?)`, formatTime(normalisedTime(now)), source.id, source.revision); err != nil {
				return err
			}
		}
	}
	return nil
}
