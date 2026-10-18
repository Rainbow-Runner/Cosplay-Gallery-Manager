package productdb

import (
	"context"
	"database/sql"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/sourcescan"
)

type MediaAddedStore struct{ db *sql.DB }

func (db *Database) MediaAdded() *MediaAddedStore { return &MediaAddedStore{db: db.DB} }

// BackfillOne performs one bounded Gallery-level metadata pass. DIRECTORY
// sources use stat only; archives are enumerated once without opening members.
func (s *MediaAddedStore) BackfillOne(ctx context.Context, limits archivecheck.Limits, now time.Time) (bool, error) {
	var sourceID, galleryID int64
	var sourceType gallery.SourceType
	var sourcePath string
	err := s.db.QueryRowContext(ctx, `SELECT source.id,source.gallery_id,source.source_type,source.source_path
		FROM gallery_sources source JOIN galleries gallery ON gallery.id=source.gallery_id
		WHERE source.availability_state='AVAILABLE' AND source.reconcile_state<>'SCANNING'
		AND EXISTS(SELECT 1 FROM gallery_items item WHERE item.source_id=source.id
			AND item.media_kind='STATIC_IMAGE' AND item.availability_state='AVAILABLE'
			AND item.source_modified_status='PENDING')
		ORDER BY CASE gallery.state WHEN 'ACTIVE' THEN 0 WHEN 'DRAFT' THEN 1 ELSE 2 END,source.id LIMIT 1`).Scan(&sourceID, &galleryID, &sourceType, &sourcePath)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var evidence map[string]sourcescan.ModificationEvidence
	if sourceType == gallery.SourceTypeArchive {
		evidence, err = sourcescan.ScanArchiveModificationTimes(ctx, sourcePath, limits)
	} else {
		evidence, err = sourcescan.ScanDirectoryModificationTimes(ctx, sourcePath)
	}
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id,relative_path FROM gallery_items WHERE source_id=?
		AND media_kind='STATIC_IMAGE' AND availability_state='AVAILABLE' AND source_modified_status='PENDING'`, sourceID)
	if err != nil {
		return false, err
	}
	type pending struct {
		id   int64
		path string
	}
	var items []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.id, &item.path); err != nil {
			rows.Close()
			return false, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	checked := formatTime(normalisedTime(now))
	for _, item := range items {
		value, ok := evidence[item.path]
		if !ok {
			value.Status = "NONE"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_items SET source_modified_at_utc=?,source_modified_status=?,
			source_modified_origin=?,source_modified_checked_at_utc=? WHERE id=? AND source_modified_status='PENDING'`,
			value.AtUTC, value.Status, value.Origin, checked, item.id); err != nil {
			return false, err
		}
	}
	if err := reconcileGalleryMediaAdded(ctx, tx, galleryID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
