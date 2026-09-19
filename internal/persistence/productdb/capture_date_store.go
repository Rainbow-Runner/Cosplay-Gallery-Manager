package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
)

const CaptureDateVariant = "CAPTURE_DATE"
const captureDateProfile = "capture-date-v1"

type CaptureDateRange struct{ Start, End string }
type CaptureDateSummary struct {
	Images, Videos                  CaptureDateRange
	Candidate, Manual, ReviewStatus string
}

type CaptureDateStore struct{ db *sql.DB }

func (db *Database) CaptureDates() *CaptureDateStore { return &CaptureDateStore{db: db.DB} }

func (s *CaptureDateStore) Timezone(ctx context.Context, itemUUID string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(library.capture_timezone,'UTC') FROM gallery_items item
		JOIN gallery_sources source ON source.id=item.source_id LEFT JOIN media_libraries library ON library.id=source.library_id WHERE item.item_uuid=?`, itemUUID).Scan(&value)
	return value, err
}

func (s *CaptureDateStore) EnqueueBackfill(ctx context.Context, limit int, videoAvailable bool, now time.Time) (int, error) {
	return s.enqueueBackfill(ctx, limit, videoAvailable, 0, now)
}

// EnqueueGallery is used by portable rebuild after the destination source has
// been scanned and its Manifest restored. Remaining items are handled by the
// bounded scheduler, never by copying source-machine evidence or paths.
func (s *CaptureDateStore) EnqueueGallery(ctx context.Context, galleryID int64, videoAvailable bool, now time.Time) (int, error) {
	if galleryID <= 0 {
		return 0, errors.New("invalid Gallery")
	}
	return s.enqueueBackfill(ctx, 500, videoAvailable, galleryID, now)
}

func (s *CaptureDateStore) enqueueBackfill(ctx context.Context, limit int, videoAvailable bool, galleryID int64, now time.Time) (int, error) {
	if limit < 1 || limit > 500 {
		return 0, errors.New("invalid capture-date backfill limit")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT item.item_uuid,item.gallery_id,item.content_revision FROM gallery_items item
		JOIN gallery_sources source ON source.id=item.source_id
		LEFT JOIN item_capture_dates capture ON capture.item_uuid=item.item_uuid AND capture.content_revision=item.content_revision
		WHERE item.media_kind IN ('STATIC_IMAGE','VIDEO') AND (?=1 OR item.media_kind='STATIC_IMAGE') AND item.excluded=0 AND item.availability_state='AVAILABLE'
		AND source.availability_state='AVAILABLE' AND capture.item_uuid IS NULL AND (?=0 OR item.gallery_id=?)
		AND NOT EXISTS(SELECT 1 FROM processing_jobs job WHERE job.job_key='capture-date:'||item.item_uuid||':'||item.content_revision||':capture-date-v1')
		ORDER BY item.id LIMIT ?`, boolInt(videoAvailable), galleryID, galleryID, limit)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		uuid                string
		galleryID, revision int64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.uuid, &c.galleryID, &c.revision); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	queued := 0
	for _, c := range candidates {
		key := fmt.Sprintf("capture-date:%s:%d:%s", c.uuid, c.revision, captureDateProfile)
		job, err := (&ProcessingJobStore{db: s.db}).FindByKey(ctx, key)
		if err == nil && job.ID != 0 {
			continue
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return queued, err
		}
		_, err = (&ProcessingJobStore{db: s.db}).Enqueue(ctx, EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemTechnicalMetadata, GalleryID: &c.galleryID, ItemUUID: c.uuid, Variant: CaptureDateVariant, ContentRevision: &c.revision, ProfileHash: captureDateProfile, Payload: map[string]any{}, Priority: 30}, now)
		if err != nil {
			return queued, err
		}
		queued++
	}
	return queued, nil
}

func (s *CaptureDateStore) Publish(ctx context.Context, itemUUID string, revision int64, date, tag string, now time.Time) error {
	if date != "" {
		if parsed, err := time.Parse("2006-01-02", date); err != nil || parsed.Format("2006-01-02") != date || tag == "" {
			return errors.New("invalid capture date evidence")
		}
	} else {
		tag = ""
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var galleryID int64
	var actualRevision int64
	if err = tx.QueryRowContext(ctx, `SELECT gallery_id,content_revision FROM gallery_items WHERE item_uuid=?`, itemUUID).Scan(&galleryID, &actualRevision); err != nil {
		return err
	}
	if actualRevision != revision {
		return errors.New("capture date content revision is stale")
	}
	status := "NONE"
	if date != "" {
		status = "FOUND"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO item_capture_dates(item_uuid,content_revision,capture_date,source_tag,status,checked_at_utc) VALUES(?,?,NULLIF(?,''),?,?,?) ON CONFLICT(item_uuid) DO UPDATE SET content_revision=excluded.content_revision,capture_date=excluded.capture_date,source_tag=excluded.source_tag,status=excluded.status,checked_at_utc=excluded.checked_at_utc`, itemUUID, revision, date, tag, status, formatTime(normalisedTime(now)))
	if err != nil {
		return err
	}
	if err = reconcileGalleryCaptureDate(ctx, tx, galleryID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func reconcileGalleryCaptureDate(ctx context.Context, tx *sql.Tx, galleryID int64, now time.Time) error {
	var total, done int
	var candidate sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(capture.item_uuid),MIN(capture.capture_date) FROM gallery_items item
		LEFT JOIN item_capture_dates capture ON capture.item_uuid=item.item_uuid AND capture.content_revision=item.content_revision
		WHERE item.gallery_id=? AND item.excluded=0 AND item.availability_state='AVAILABLE' AND item.media_kind IN ('STATIC_IMAGE','VIDEO')`, galleryID).Scan(&total, &done, &candidate)
	if err != nil {
		return err
	}
	if total == 0 || total != done {
		updated, updateErr := tx.ExecContext(ctx, `UPDATE galleries SET shoot_date=NULL,shoot_date_precision=NULL,metadata_revision=metadata_revision+1,updated_at_utc=?
			WHERE id=? AND shoot_date_origin='AUTO' AND shoot_date IS NOT NULL`, formatTime(normalisedTime(now)), galleryID)
		if updateErr != nil {
			return updateErr
		}
		changed, updateErr := updated.RowsAffected()
		if updateErr != nil {
			return updateErr
		}
		if changed > 0 {
			if updateErr = markGalleryManifestDBDirty(ctx, tx, galleryID); updateErr != nil {
				return updateErr
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM gallery_capture_date_reviews WHERE gallery_id=?`, galleryID)
		return err
	}
	var current sql.NullString
	var origin string
	if err = tx.QueryRowContext(ctx, `SELECT shoot_date,shoot_date_origin FROM galleries WHERE id=?`, galleryID).Scan(&current, &origin); err != nil {
		return err
	}
	if !candidate.Valid {
		if origin == "AUTO" && current.Valid {
			_, err = tx.ExecContext(ctx, `UPDATE galleries SET shoot_date=NULL,shoot_date_precision=NULL,metadata_revision=metadata_revision+1,updated_at_utc=? WHERE id=?`, formatTime(normalisedTime(now)), galleryID)
			if err != nil {
				return err
			}
			if err = markGalleryManifestDBDirty(ctx, tx, galleryID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM gallery_capture_date_reviews WHERE gallery_id=?`, galleryID)
		return err
	}
	if !current.Valid || origin == "AUTO" {
		if current.String != candidate.String || origin != "AUTO" {
			_, err = tx.ExecContext(ctx, `UPDATE galleries SET shoot_date=?,shoot_date_precision='DAY',shoot_date_origin='AUTO',metadata_revision=metadata_revision+1,updated_at_utc=? WHERE id=?`, candidate.String, formatTime(normalisedTime(now)), galleryID)
			if err != nil {
				return err
			}
			if err = markGalleryManifestDBDirty(ctx, tx, galleryID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM gallery_capture_date_reviews WHERE gallery_id=?`, galleryID)
		return err
	}
	if current.String == candidate.String {
		_, err = tx.ExecContext(ctx, `DELETE FROM gallery_capture_date_reviews WHERE gallery_id=?`, galleryID)
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO gallery_capture_date_reviews(gallery_id,candidate_date,manual_date,status,updated_at_utc) VALUES(?,?,?,'PENDING',?)
		ON CONFLICT(gallery_id) DO UPDATE SET candidate_date=excluded.candidate_date,manual_date=excluded.manual_date,
		status=CASE WHEN gallery_capture_date_reviews.candidate_date=excluded.candidate_date AND gallery_capture_date_reviews.manual_date=excluded.manual_date THEN gallery_capture_date_reviews.status ELSE 'PENDING' END,updated_at_utc=excluded.updated_at_utc`, galleryID, candidate.String, current.String, formatTime(normalisedTime(now)))
	return err
}

func (s *CaptureDateStore) Summary(ctx context.Context, galleryID int64) (CaptureDateSummary, error) {
	var result CaptureDateSummary
	rows, err := s.db.QueryContext(ctx, `SELECT item.media_kind,MIN(capture.capture_date),MAX(capture.capture_date),COUNT(*),COUNT(capture.item_uuid)
		FROM gallery_items item LEFT JOIN item_capture_dates capture ON capture.item_uuid=item.item_uuid AND capture.content_revision=item.content_revision
		WHERE item.gallery_id=? AND item.excluded=0 AND item.availability_state='AVAILABLE' AND item.media_kind IN ('STATIC_IMAGE','VIDEO') GROUP BY item.media_kind`, galleryID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var min, max sql.NullString
		var total, done int
		if err := rows.Scan(&kind, &min, &max, &total, &done); err != nil {
			return result, err
		}
		if total != done {
			continue
		}
		value := CaptureDateRange{Start: min.String, End: max.String}
		if kind == "VIDEO" {
			result.Videos = value
		} else {
			result.Images = value
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT candidate_date,manual_date,status FROM gallery_capture_date_reviews WHERE gallery_id=?`, galleryID).Scan(&result.Candidate, &result.Manual, &result.ReviewStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	return result, err
}

func (s *CaptureDateStore) Resolve(ctx context.Context, galleryID, expectedRevision int64, candidate, decision string, now time.Time) error {
	if decision != "KEEP_MANUAL" && decision != "USE_EXTRACTED" {
		return errors.New("invalid capture date decision")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current string
	var revision int64
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT review.candidate_date,review.status,gallery.metadata_revision FROM gallery_capture_date_reviews review JOIN galleries gallery ON gallery.id=review.gallery_id WHERE review.gallery_id=?`, galleryID).Scan(&current, &status, &revision); err != nil {
		return err
	}
	if revision != expectedRevision || candidate != current || status != "PENDING" {
		return errors.New("capture date review is stale")
	}
	if decision == "KEEP_MANUAL" {
		_, err = tx.ExecContext(ctx, `UPDATE gallery_capture_date_reviews SET status='KEEP_MANUAL',updated_at_utc=? WHERE gallery_id=?`, formatTime(normalisedTime(now)), galleryID)
		if err != nil {
			return err
		}
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE galleries SET shoot_date=?,shoot_date_precision='DAY',shoot_date_origin='AUTO',metadata_revision=metadata_revision+1,updated_at_utc=? WHERE id=?`, candidate, formatTime(normalisedTime(now)), galleryID)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM gallery_capture_date_reviews WHERE gallery_id=?`, galleryID); err != nil {
			return err
		}
		if err = markGalleryManifestDBDirty(ctx, tx, galleryID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
