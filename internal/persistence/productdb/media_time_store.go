package productdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stashapp/stash/internal/imagemetadata"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

const MediaTimeVariant = "MEDIA_TIMES"
const MediaTimeProfile = "media-times-v1"

type MediaTimeStore struct{ db *sql.DB }

func (db *Database) MediaTimes() *MediaTimeStore { return &MediaTimeStore{db.DB} }

func (s *MediaTimeStore) ReconcilePending(ctx context.Context, limit int) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT gallery.id FROM galleries gallery WHERE gallery.software_time_status='PENDING' AND NOT EXISTS(SELECT 1 FROM gallery_items item JOIN gallery_sources src ON src.id=item.source_id LEFT JOIN media_libraries lib ON lib.id=src.library_id LEFT JOIN item_media_times e ON e.item_uuid=item.item_uuid AND e.content_revision=item.content_revision AND e.profile=? AND e.timezone=COALESCE(lib.capture_timezone,'UTC') WHERE item.gallery_id=gallery.id AND item.media_kind='STATIC_IMAGE' AND item.excluded=0 AND item.availability_state='AVAILABLE' AND e.item_uuid IS NULL) ORDER BY gallery.id LIMIT ?`, MediaTimeProfile, limit)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, id := range ids {
		tx, e := s.db.BeginTx(ctx, nil)
		if e != nil {
			return 0, e
		}
		e = reconcileGallerySoftwareTime(ctx, tx, id)
		if e == nil {
			e = tx.Commit()
		} else {
			tx.Rollback()
		}
		if e != nil {
			return 0, e
		}
	}
	return len(ids), nil
}

func (s *MediaTimeStore) HasCurrent(ctx context.Context, uuid string, revision int64, zone string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM item_media_times WHERE item_uuid=? AND content_revision=? AND profile=? AND timezone=?)`, uuid, revision, MediaTimeProfile, zone).Scan(&exists)
	return exists, err
}

// Durable missing evidence drives this existing event-based scheduler. A
// checked file without dates is still complete; it is not polled again.
func (s *MediaTimeStore) EnqueueBackfill(ctx context.Context, limit int, video bool, now time.Time) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, errors.New("invalid media-time batch")
	}
	runtime, err := (&SettingsStore{db: s.db}).Find(ctx)
	if err != nil {
		return 0, err
	}
	if !runtime.MediaTimeBackfillEnabled || runtime.AutomaticSchedulesSuspended {
		return 0, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT item.item_uuid,item.gallery_id,item.content_revision,COALESCE(lib.capture_timezone,'UTC') FROM gallery_items item
 JOIN gallery_sources source ON source.id=item.source_id LEFT JOIN media_libraries lib ON lib.id=source.library_id
 LEFT JOIN item_media_times evidence ON evidence.item_uuid=item.item_uuid AND evidence.content_revision=item.content_revision AND evidence.profile=? AND evidence.timezone=COALESCE(lib.capture_timezone,'UTC')
 WHERE item.excluded=0 AND item.availability_state='AVAILABLE' AND source.availability_state='AVAILABLE' AND item.media_kind IN ('STATIC_IMAGE','VIDEO') AND (?=1 OR item.media_kind='STATIC_IMAGE') AND evidence.item_uuid IS NULL
 AND NOT EXISTS(SELECT 1 FROM processing_jobs job WHERE job.item_uuid=item.item_uuid AND job.content_revision=item.content_revision AND job.variant=? AND job.profile_hash=? AND json_extract(job.payload_json,'$.timezone')=COALESCE(lib.capture_timezone,'UTC'))
 AND NOT EXISTS(SELECT 1 FROM processing_jobs job WHERE job.job_key>='item:'||item.item_uuid||':' AND job.job_key<'item:'||item.item_uuid||';' AND job.status IN ('PENDING','RUNNING','RETRY_WAIT'))
 ORDER BY item.id LIMIT ?`, MediaTimeProfile, video, MediaTimeVariant, MediaTimeProfile, limit)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		uuid, zone        string
		gallery, revision int64
	}
	var items []candidate
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.uuid, &c.gallery, &c.revision, &c.zone); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, c)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	count := 0
	for _, c := range items {
		key := fmt.Sprintf("media-times:%s:%d:%s:%x", c.uuid, c.revision, MediaTimeProfile, sha256.Sum256([]byte(c.zone)))
		if _, err = (&ProcessingJobStore{db: s.db}).FindByKey(ctx, key); err == nil {
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			return count, err
		}
		_, err = (&ProcessingJobStore{db: s.db}).Enqueue(ctx, EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemTechnicalMetadata, GalleryID: &c.gallery, ItemUUID: c.uuid, Variant: MediaTimeVariant, ContentRevision: &c.revision, ProfileHash: MediaTimeProfile, Priority: 10, Payload: map[string]any{"timezone": c.zone}}, now)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *MediaTimeStore) Publish(ctx context.Context, uuid string, revision int64, zone string, value imagemetadata.TimeSummary, now time.Time, jobs ...mediaprocessing.Job) error {
	for _, stamp := range []string{value.CaptureAtUTC, value.SoftwareAtUTC} {
		if stamp != "" {
			if _, err := time.Parse(imagemetadata.EvidenceTimeLayout, stamp); err != nil {
				return errors.New("invalid media time evidence")
			}
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if len(jobs) > 0 {
		job := jobs[0]
		var until string
		if e := tx.QueryRowContext(ctx, `SELECT lease_expires_at_utc FROM processing_jobs WHERE id=? AND status='RUNNING' AND lease_owner=?`, job.ID, job.LeaseOwner).Scan(&until); e != nil {
			return ErrJobLeaseLost
		}
		deadline, e := time.Parse(time.RFC3339Nano, until)
		if e != nil || !now.Before(deadline) {
			return ErrJobLeaseLost
		}
	}
	var gallery, actual int64
	var currentZone string
	err = tx.QueryRowContext(ctx, `SELECT item.gallery_id,item.content_revision,COALESCE(lib.capture_timezone,'UTC') FROM gallery_items item JOIN gallery_sources src ON src.id=item.source_id LEFT JOIN media_libraries lib ON lib.id=src.library_id WHERE item.item_uuid=?`, uuid).Scan(&gallery, &actual, &currentZone)
	if err != nil {
		return err
	}
	if actual != revision || currentZone != zone {
		return errors.New("stale media time evidence")
	}
	state := "READY"
	if value.Partial {
		state = "PARTIAL"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO item_media_times(item_uuid,content_revision,profile,timezone,capture_at_utc,capture_precision,capture_zone_source,software_at_utc,software_local_date,software_event_index,software_zone_source,state,checked_at_utc)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(item_uuid) DO UPDATE SET content_revision=excluded.content_revision,profile=excluded.profile,timezone=excluded.timezone,capture_at_utc=excluded.capture_at_utc,capture_precision=excluded.capture_precision,capture_zone_source=excluded.capture_zone_source,software_at_utc=excluded.software_at_utc,software_local_date=excluded.software_local_date,software_event_index=excluded.software_event_index,software_zone_source=excluded.software_zone_source,state=excluded.state,checked_at_utc=excluded.checked_at_utc`, uuid, revision, MediaTimeProfile, zone, value.CaptureAtUTC, value.CapturePrecision, value.CaptureZoneSource, value.SoftwareAtUTC, value.SoftwareLocalDate, value.SoftwareEventIndex, value.SoftwareZoneSource, state, formatTime(now))
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE galleries SET time_evidence_revision=time_evidence_revision+1 WHERE id=?`, gallery); err != nil {
		return err
	}
	if err = reconcileGallerySoftwareTime(ctx, tx, gallery); err != nil {
		return err
	}
	return tx.Commit()
}

func reconcileGallerySoftwareTime(ctx context.Context, tx *sql.Tx, gallery int64) error {
	var total, done, partial int
	var stamp, date sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(e.item_uuid),COALESCE(SUM(e.state='PARTIAL'),0),MIN(NULLIF(e.software_at_utc,'')) FROM gallery_items item
 JOIN gallery_sources src ON src.id=item.source_id LEFT JOIN media_libraries lib ON lib.id=src.library_id
 LEFT JOIN item_media_times e ON e.item_uuid=item.item_uuid AND e.content_revision=item.content_revision AND e.profile=? AND e.timezone=COALESCE(lib.capture_timezone,'UTC')
 WHERE item.gallery_id=? AND item.media_kind='STATIC_IMAGE' AND item.excluded=0 AND item.availability_state='AVAILABLE'`, MediaTimeProfile, gallery).Scan(&total, &done, &partial, &stamp)
	if err != nil {
		return err
	}
	state := "NONE"
	if total != done {
		state = "PENDING"
	} else if partial > 0 {
		state = "PARTIAL"
	} else if stamp.Valid {
		state = "COMPLETE"
	}
	if state == "COMPLETE" {
		if err = tx.QueryRowContext(ctx, `SELECT e.software_local_date FROM item_media_times e JOIN gallery_items item ON item.item_uuid=e.item_uuid JOIN gallery_sources src ON src.id=item.source_id LEFT JOIN media_libraries lib ON lib.id=src.library_id WHERE item.gallery_id=? AND item.media_kind='STATIC_IMAGE' AND item.excluded=0 AND item.availability_state='AVAILABLE' AND e.content_revision=item.content_revision AND e.profile=? AND e.timezone=COALESCE(lib.capture_timezone,'UTC') AND e.software_at_utc=? ORDER BY item.id LIMIT 1`, gallery, MediaTimeProfile, stamp.String).Scan(&date); err != nil {
			return err
		}
	} else {
		stamp = sql.NullString{}
	}
	_, err = tx.ExecContext(ctx, `UPDATE galleries SET software_time_utc=?,software_time_date=?,software_time_status=?,time_evidence_revision=time_evidence_revision+1 WHERE id=? AND (software_time_utc<>? OR software_time_date<>? OR software_time_status<>?)`, stamp.String, date.String, state, gallery, stamp.String, date.String, state)
	return err
}

type MediaTimeProgress struct{ Total, Checked, Partial, Pending, Failed int }

func (s *MediaTimeStore) Progress(ctx context.Context) (MediaTimeProgress, error) {
	var p MediaTimeProgress
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(e.item_uuid),COALESCE(SUM(e.state='PARTIAL'),0) FROM gallery_items item JOIN gallery_sources src ON src.id=item.source_id LEFT JOIN media_libraries lib ON lib.id=src.library_id LEFT JOIN item_media_times e ON e.item_uuid=item.item_uuid AND e.content_revision=item.content_revision AND e.profile=? AND e.timezone=COALESCE(lib.capture_timezone,'UTC') WHERE item.media_kind IN ('STATIC_IMAGE','VIDEO') AND item.excluded=0 AND item.availability_state='AVAILABLE' AND src.availability_state='AVAILABLE'`, MediaTimeProfile).Scan(&p.Total, &p.Checked, &p.Partial)
	if err != nil {
		return p, err
	}
	p.Pending = p.Total - p.Checked
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE variant=? AND status='FAILED'`, MediaTimeVariant).Scan(&p.Failed)
	return p, err
}
