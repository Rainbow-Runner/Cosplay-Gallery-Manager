package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const ManifestSourceUnavailable ManifestStatus = "SOURCE_UNAVAILABLE"

type GalleryManifestInspectionTarget struct {
	GalleryID int64
	Available bool
}

func (s *ManifestStore) recordGalleryInspection(ctx context.Context, galleryID int64, state GalleryManifestState, checkErr error, now time.Time) error {
	var revision int64
	var sourcePath string
	if err := s.db.QueryRowContext(ctx, `SELECT gallery.metadata_revision,COALESCE(source.source_path,'')
		FROM galleries gallery LEFT JOIN gallery_sources source ON source.gallery_id=gallery.id WHERE gallery.id=?`, galleryID).Scan(&revision, &sourcePath); err != nil {
		return err
	}
	errorCode := ""
	if state.Status == ManifestSourceUnavailable {
		errorCode = "SOURCE_UNAVAILABLE"
	} else if checkErr != nil {
		errorCode = "MANIFEST_CHECK_FAILED"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO gallery_manifest_inspections
		(gallery_id,status,checked_at_utc,checked_metadata_revision,source_path,file_hash,error_code)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT(gallery_id) DO UPDATE SET
		status=excluded.status,checked_at_utc=excluded.checked_at_utc,
		checked_metadata_revision=excluded.checked_metadata_revision,source_path=excluded.source_path,
		file_hash=excluded.file_hash,error_code=excluded.error_code`, galleryID, state.Status,
		formatTime(normalisedTime(now)), revision, sourcePath, state.FileHash, errorCode)
	return err
}

func (s *ManifestStore) RecordUnavailableGallery(ctx context.Context, galleryID int64, now time.Time) error {
	return s.recordGalleryInspection(ctx, galleryID, GalleryManifestState{GalleryID: galleryID, Status: ManifestSourceUnavailable}, errors.New("source unavailable"), now)
}

// ScheduledInspectionTargets cycles through enabled-library Galleries in ID
// order. The cursor is persisted after each attempt, so a cancelled run resumes
// without repeatedly checking only the first page of a large library.
func (s *ManifestStore) ScheduledInspectionTargets(ctx context.Context, limit int) ([]GalleryManifestInspectionTarget, error) {
	if limit < 1 || limit > 500 {
		return nil, errors.New("Manifest inspection limit must be between 1 and 500")
	}
	var cursor int64
	if err := s.db.QueryRowContext(ctx, `SELECT last_gallery_id FROM gallery_manifest_inspection_progress WHERE singleton_id=1`).Scan(&cursor); err != nil {
		return nil, err
	}
	load := func(after int64) ([]GalleryManifestInspectionTarget, error) {
		rows, err := s.db.QueryContext(ctx, `SELECT gallery.id,source.availability_state='AVAILABLE'
			FROM galleries gallery JOIN gallery_sources source ON source.gallery_id=gallery.id
			JOIN media_libraries library ON library.id=source.library_id
			WHERE library.enabled=1 AND gallery.id>? ORDER BY gallery.id LIMIT ?`, after, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var targets []GalleryManifestInspectionTarget
		for rows.Next() {
			var target GalleryManifestInspectionTarget
			if err := rows.Scan(&target.GalleryID, &target.Available); err != nil {
				return nil, err
			}
			targets = append(targets, target)
		}
		return targets, rows.Err()
	}
	targets, err := load(cursor)
	if err != nil || len(targets) != 0 || cursor == 0 {
		return targets, err
	}
	return load(0)
}

func (s *ManifestStore) AdvanceInspectionCursor(ctx context.Context, galleryID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE gallery_manifest_inspection_progress SET last_gallery_id=? WHERE singleton_id=1`, galleryID)
	return err
}

type GalleryManifestInspection struct {
	Status          string
	CheckedAt       string
	CheckedRevision int64
	SourcePath      string
	FileHash        string
	ErrorCode       string
}

func (s *ManifestStore) Inspection(ctx context.Context, galleryID int64) (GalleryManifestInspection, error) {
	var result GalleryManifestInspection
	err := s.db.QueryRowContext(ctx, `SELECT status,checked_at_utc,checked_metadata_revision,source_path,file_hash,error_code
		FROM gallery_manifest_inspections WHERE gallery_id=?`, galleryID).Scan(&result.Status, &result.CheckedAt,
		&result.CheckedRevision, &result.SourcePath, &result.FileHash, &result.ErrorCode)
	if errors.Is(err, sql.ErrNoRows) {
		return GalleryManifestInspection{Status: "UNCHECKED"}, nil
	}
	return result, err
}
