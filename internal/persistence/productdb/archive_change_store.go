package productdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
)

// markChangedArchiveSources compares registered archives with the evidence
// captured by their last successful source scan. It does not inspect archive
// members or change Gallery metadata; the source scanner does that later.
func markChangedArchiveSources(ctx context.Context, db *sql.DB, libraryID int64, limits archivecheck.Limits, now time.Time) error {
	encodedLimits, err := json.Marshal(limits)
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `SELECT source.id,source.source_path,
		evidence.container_size,evidence.container_modified_at_utc,evidence.checked_at_utc,
		evidence.archive_limits_json,evidence.scanner_version
		FROM gallery_sources source LEFT JOIN gallery_source_scan_evidence evidence ON evidence.source_id=source.id
		WHERE source.library_id=? AND source.source_type='ARCHIVE' AND source.reconcile_state='IN_SYNC'
		AND (evidence.source_id IS NOT NULL OR NOT EXISTS (
			SELECT 1 FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.resolved_at_utc IS NULL
		))`, libraryID)
	if err != nil {
		return err
	}
	type archiveState struct {
		id                        int64
		path                      string
		size                      sql.NullInt64
		modified, checked, limits sql.NullString
		version                   sql.NullInt64
	}
	var sources []archiveState
	for rows.Next() {
		var source archiveState
		if err := rows.Scan(&source.id, &source.path, &source.size, &source.modified, &source.checked, &source.limits, &source.version); err != nil {
			rows.Close()
			return err
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Lstat(source.path)
		if errors.Is(err, os.ErrNotExist) {
			// Missing-source reconciliation has its own lifecycle. Do not turn
			// an unavailable archive into a content-replacement scan.
			continue
		}
		if err == nil {
			if info.Mode().IsRegular() {
				modified := ""
				if !info.ModTime().IsZero() {
					modified = info.ModTime().UTC().Format(time.RFC3339Nano)
				}
				if source.size.Valid && source.modified.Valid && source.size.Int64 == info.Size() && source.modified.String == modified &&
					source.version.Valid && source.version.Int64 == archiveScanEvidenceVersion &&
					source.limits.Valid && source.limits.String == string(encodedLimits) {
					continue
				}
			}
			// Non-regular sources are handed to the source scanner, which
			// records the concrete error without altering the last Item set.
		}
		// A concurrent source scan may have replaced the evidence since our
		// SELECT. Only mark this source if that exact evidence is still live.
		result, err := db.ExecContext(ctx, `UPDATE gallery_sources SET reconcile_state='NEEDS_RESCAN',updated_at_utc=?
			WHERE id=? AND reconcile_state='IN_SYNC' AND
			((? IS NULL AND NOT EXISTS (SELECT 1 FROM gallery_source_scan_evidence WHERE source_id=?)) OR
			 EXISTS (SELECT 1 FROM gallery_source_scan_evidence WHERE source_id=?
				AND container_size=? AND container_modified_at_utc=? AND checked_at_utc=?))`,
			formatTime(normalisedTime(now)), source.id, source.checked, source.id,
			source.id, source.size, source.modified, source.checked)
		if err != nil {
			return err
		}
		if changed, err := result.RowsAffected(); err == nil && changed == 1 {
			slog.Info("CGM_ARCHIVE_SOURCE_CHANGE_DETECTED", "library_id", libraryID, "source_id", source.id)
		}
	}
	return nil
}

// ChangedArchiveSourceTargets supplies the bounded background reconciler.
// A failed scan moves the source to ERROR rather than retrying forever.
func (db *Database) ChangedArchiveSourceTargets(ctx context.Context, limit int) ([]int64, error) {
	if limit < 1 || limit > 25 {
		limit = 1
	}
	rows, err := db.QueryContext(ctx, `SELECT source.id FROM gallery_sources source
		JOIN media_libraries library ON library.id=source.library_id
		WHERE library.enabled=1 AND source.source_type='ARCHIVE'
		AND source.availability_state='AVAILABLE' AND source.reconcile_state='NEEDS_RESCAN'
		ORDER BY source.updated_at_utc,source.id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
