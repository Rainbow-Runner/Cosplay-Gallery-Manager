package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
)

// ChangeSlug is the only operation that changes a Gallery presentation URL.
// UUID remains identity and ordinary title edits intentionally preserve Slug.
func (s *GalleryStore) ChangeSlug(ctx context.Context, setID string, expectedRevision int64, requested string, now time.Time) (string, error) {
	newSlug, err := validateCustomSlug(requested)
	if err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var oldSlug string
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT slug,metadata_revision FROM galleries WHERE set_id=?`, setID).Scan(&oldSlug, &revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrGalleryNotFound
		}
		return "", err
	}
	if revision != expectedRevision {
		return "", ErrMetadataRevisionConflict
	}
	if oldSlug == newSlug {
		return oldSlug, tx.Commit()
	}
	timestamp := formatTime(normalisedTime(now))
	result, err := tx.ExecContext(ctx, `UPDATE galleries SET slug=?,metadata_revision=metadata_revision+1,
		updated_at_utc=? WHERE set_id=? AND metadata_revision=?`, newSlug, timestamp, setID, expectedRevision)
	if err != nil {
		return "", err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return "", ErrMetadataRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO slug_redirects(entity_kind,old_slug,target_uuid,created_at_utc)
		VALUES('GALLERY',?,?,?) ON CONFLICT(entity_kind,old_slug) DO UPDATE SET target_uuid=excluded.target_uuid,
		created_at_utc=excluded.created_at_utc`, oldSlug, setID, timestamp); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM slug_redirects WHERE entity_kind='GALLERY' AND old_slug=?`, newSlug); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gallery_manifest_sync SET status=CASE WHEN status='CLEAN' THEN 'DB_DIRTY' ELSE status END
		WHERE gallery_id=(SELECT id FROM galleries WHERE set_id=?)`, setID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return newSlug, nil
}

// ResolveSlug resolves the current route or a lightweight historical route.
// The boolean reports whether the caller should issue a redirect.
func (s *GalleryStore) ResolveSlug(ctx context.Context, value string) (gallery.Gallery, bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM galleries WHERE slug=?`, value).Scan(&id)
	if err == nil {
		result, findErr := findGallery(ctx, s.db, id)
		return result, false, findErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return gallery.Gallery{}, false, err
	}
	var setID string
	if err := s.db.QueryRowContext(ctx, `SELECT target_uuid FROM slug_redirects WHERE entity_kind=? AND old_slug=?`, portableid.KindGallery, value).Scan(&setID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return gallery.Gallery{}, false, ErrGalleryNotFound
		}
		return gallery.Gallery{}, false, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM galleries WHERE set_id=?`, setID).Scan(&id); err != nil {
		return gallery.Gallery{}, false, err
	}
	result, err := findGallery(ctx, s.db, id)
	return result, true, err
}
