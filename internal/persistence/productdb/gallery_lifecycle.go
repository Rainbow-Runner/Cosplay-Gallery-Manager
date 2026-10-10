package productdb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
)

var ErrGalleryDeleteRequiresArchived = errors.New("Gallery must be ARCHIVED before permanent deletion")

type GalleryDeletePreview struct {
	SetID              string
	State              gallery.State
	MetadataRevision   int64
	ItemCount          int64
	ExternalLinkCount  int64
	ExecutableJobCount int64
	HasSource          bool
}

func (preview GalleryDeletePreview) CanDelete() bool {
	return preview.State == gallery.StateArchived
}

// PreviewDelete reports the records affected by a permanent Gallery deletion.
// Delete repeats the state and revision checks in its write transaction.
func (s *GalleryStore) PreviewDelete(ctx context.Context, setID string) (GalleryDeletePreview, error) {
	result := GalleryDeletePreview{SetID: setID}
	var state string
	var hasSource int
	err := s.db.QueryRowContext(ctx, `
		SELECT g.state,g.metadata_revision,
			(SELECT COUNT(*) FROM gallery_items i WHERE i.gallery_id=g.id),
			(SELECT COUNT(*) FROM gallery_external_links l WHERE l.gallery_id=g.id),
			(SELECT COUNT(*) FROM processing_jobs j
				WHERE (j.gallery_id=g.id OR j.item_uuid IN (
					SELECT i.item_uuid FROM gallery_items i WHERE i.gallery_id=g.id
				)) AND j.status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED')),
			EXISTS(SELECT 1 FROM gallery_sources s WHERE s.gallery_id=g.id)
		FROM galleries g WHERE g.set_id=?
	`, setID).Scan(&state, &result.MetadataRevision, &result.ItemCount, &result.ExternalLinkCount,
		&result.ExecutableJobCount, &hasSource)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GalleryDeletePreview{}, ErrGalleryNotFound
		}
		return GalleryDeletePreview{}, err
	}
	result.State = gallery.State(state)
	result.HasSource = hasSource == 1
	return result, nil
}

// Delete permanently removes one ARCHIVED Gallery aggregate from the database.
// It intentionally leaves source media, the Gallery Manifest, managed source
// assets, and derivative cache files untouched.
func (s *GalleryStore) Delete(ctx context.Context, setID string, expectedRevision int64, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var galleryID int64
	var state string
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT id,state,metadata_revision FROM galleries WHERE set_id=?`, setID).
		Scan(&galleryID, &state, &revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGalleryNotFound
		}
		return err
	}
	if gallery.State(state) != gallery.StateArchived {
		return ErrGalleryDeleteRequiresArchived
	}
	if revision != expectedRevision {
		return ErrMetadataRevisionConflict
	}

	itemUUIDs, err := galleryPortableUUIDs(ctx, tx, `SELECT item_uuid FROM gallery_items WHERE gallery_id=?`, galleryID)
	if err != nil {
		return err
	}
	linkUUIDs, err := galleryPortableUUIDs(ctx, tx, `SELECT link_uuid FROM gallery_external_links WHERE gallery_id=?`, galleryID)
	if err != nil {
		return err
	}

	timestamp := formatTime(normalisedTime(now))
	// Preserve task history while preventing any worker from continuing work
	// for an aggregate that is about to disappear.
	if _, err := tx.ExecContext(ctx, `
		UPDATE processing_jobs SET
			status=CASE WHEN status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED') THEN 'CANCELLED' ELSE status END,
			gallery_id=NULL,item_uuid=NULL,lease_owner=NULL,lease_expires_at_utc=NULL,last_heartbeat_at_utc=NULL,
			completed_at_utc=CASE WHEN status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED') THEN ? ELSE completed_at_utc END,
			updated_at_utc=?
		WHERE gallery_id=? OR item_uuid IN (SELECT item_uuid FROM gallery_items WHERE gallery_id=?)
	`, timestamp, timestamp, galleryID, galleryID); err != nil {
		return err
	}

	var libraryID sql.NullInt64
	var sourcePath string
	sourceErr := tx.QueryRowContext(ctx, `SELECT library_id,source_path FROM gallery_sources WHERE gallery_id=?`, galleryID).
		Scan(&libraryID, &sourcePath)
	if sourceErr != nil && !errors.Is(sourceErr, sql.ErrNoRows) {
		return sourceErr
	}
	if sourceErr == nil {
		absolutePath, err := filepath.Abs(sourcePath)
		if err != nil {
			return err
		}
		absolutePath = filepath.Clean(absolutePath)
		if _, err := tx.ExecContext(ctx, `DELETE FROM ignored_gallery_sources WHERE set_id=? OR source_path=?`, setID, absolutePath); err != nil {
			return err
		}
		var libraryValue any
		if libraryID.Valid {
			libraryValue = libraryID.Int64
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ignored_gallery_sources(library_id,set_id,source_path,reason,created_at_utc)
			VALUES(?,?,?,'GALLERY_DELETED',?)
		`, libraryValue, setID, absolutePath, timestamp); err != nil {
			return err
		}
	}

	for _, uuid := range itemUUIDs {
		if err := tombstonePortableUUID(ctx, tx, uuid, portableid.KindGalleryItem, "owning Gallery deleted", now); err != nil {
			return err
		}
	}
	for _, uuid := range linkUUIDs {
		if err := tombstonePortableUUID(ctx, tx, uuid, portableid.KindExternalLink, "owning Gallery deleted", now); err != nil {
			return err
		}
	}
	if err := tombstonePortableUUID(ctx, tx, setID, portableid.KindGallery, "Gallery permanently deleted", now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM galleries WHERE id=?`, galleryID); err != nil {
		return err
	}
	return tx.Commit()
}

func galleryPortableUUIDs(ctx context.Context, tx *sql.Tx, query string, galleryID int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, galleryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, err
		}
		result = append(result, uuid)
	}
	return result, rows.Err()
}
