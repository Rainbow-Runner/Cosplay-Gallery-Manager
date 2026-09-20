package productdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
)

var (
	ErrJobNotClaimable = errors.New("processing job is not claimable")
	ErrJobLeaseLost    = errors.New("processing job lease is no longer owned")
)

type ProcessingJobStore struct{ db *sql.DB }

// ManualGalleryPriorityFloor sits above all ordinary media processing
// priorities (including interactive playback). Later manual scans receive a
// strictly higher priority so the most recently requested Gallery runs first.
const ManualGalleryPriorityFloor = 1000

type ProcessingJobPage struct {
	Items      []mediaprocessing.Job
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}

func (db *Database) ProcessingJobs() *ProcessingJobStore { return &ProcessingJobStore{db: db.DB} }

// PrioritizeManualGalleryScan promotes only runnable, current-revision base
// processing and capture-date jobs. Running leases and retry delays remain
// untouched; terminal failures are never implicitly retried.
func (s *ProcessingJobStore) PrioritizeManualGalleryScan(ctx context.Context, galleryID int64, now time.Time) (int, error) {
	if galleryID <= 0 {
		return 0, errors.New("invalid Gallery")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var maximum int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(priority),0) FROM processing_jobs`).Scan(&maximum); err != nil {
		return 0, err
	}
	priority := int64(ManualGalleryPriorityFloor)
	if maximum >= priority {
		priority = maximum + 1
	}
	if priority > math.MaxInt32 {
		return 0, errors.New("manual Gallery priority limit reached")
	}
	result, err := tx.ExecContext(ctx, `UPDATE processing_jobs SET priority=?,updated_at_utc=?
		WHERE gallery_id=? AND status IN ('PENDING','RETRY_WAIT')
		AND ((job_kind='ITEM_DERIVATIVE' AND variant IN ('CARD_480','STATIC_POSTER'))
			OR (job_kind='ITEM_TECHNICAL_METADATA' AND variant IN ('','CAPTURE_DATE')))
		AND EXISTS(SELECT 1 FROM gallery_items item WHERE item.item_uuid=processing_jobs.item_uuid
			AND item.gallery_id=processing_jobs.gallery_id AND item.content_revision=processing_jobs.content_revision
			AND item.excluded=0 AND item.availability_state='AVAILABLE')`, priority, formatTime(normalisedTime(now)), galleryID)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(count), nil
}

type EnqueueJobInput struct {
	Key             string
	Kind            mediaprocessing.JobKind
	GalleryID       *int64
	ItemUUID        string
	Variant         string
	ContentRevision *int64
	ProfileHash     string
	Payload         any
	Priority        int
	MaxAttempts     int
	NotBefore       time.Time
}

func (s *ProcessingJobStore) Enqueue(ctx context.Context, input EnqueueJobInput, now time.Time) (mediaprocessing.Job, error) {
	if input.Key == "" || len(input.Key) > 500 || !validJobKind(input.Kind) {
		return mediaprocessing.Job{}, errors.New("invalid processing job identity")
	}
	if input.MaxAttempts == 0 {
		input.MaxAttempts = 3
	}
	if input.MaxAttempts < 1 || input.MaxAttempts > 20 || len(input.Variant) > 100 || len(input.ProfileHash) > 200 {
		return mediaprocessing.Job{}, errors.New("invalid processing job limits")
	}
	if input.Kind == mediaprocessing.JobItemDerivative && (input.ItemUUID == "" || input.Variant == "" || input.ContentRevision == nil || *input.ContentRevision <= 0 || input.ProfileHash == "") {
		return mediaprocessing.Job{}, errors.New("Item derivative job requires Item, variant, content revision and profile")
	}
	if input.Kind == mediaprocessing.JobItemTechnicalMetadata && (input.ItemUUID == "" || input.ContentRevision == nil || *input.ContentRevision <= 0 || input.ProfileHash == "") {
		return mediaprocessing.Job{}, errors.New("Item technical metadata job requires Item, content revision and profile")
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return mediaprocessing.Job{}, err
	}
	if len(payload) > 1024*1024 {
		return mediaprocessing.Job{}, errors.New("processing job payload exceeds 1 MiB")
	}
	timestamp := normalisedTime(now)
	notBefore := input.NotBefore
	if notBefore.IsZero() {
		notBefore = timestamp
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO processing_jobs (
		job_key,job_kind,gallery_id,item_uuid,variant,content_revision,profile_hash,payload_json,
		status,priority,max_attempts,not_before_utc,created_at_utc,updated_at_utc
	) VALUES (?,?,?,NULLIF(?,''),?,?,?,?,'PENDING',?,?,?,?,?)
	ON CONFLICT(job_key) DO UPDATE SET priority=MAX(priority,excluded.priority), updated_at_utc=excluded.updated_at_utc,
		status=CASE WHEN processing_jobs.status IN ('PENDING','RETRY_WAIT','PAUSED') THEN processing_jobs.status ELSE processing_jobs.status END`,
		input.Key, input.Kind, input.GalleryID, input.ItemUUID, input.Variant, input.ContentRevision,
		input.ProfileHash, payload, input.Priority, input.MaxAttempts, formatTime(normalisedTime(notBefore)),
		formatTime(timestamp), formatTime(timestamp))
	if err != nil {
		return mediaprocessing.Job{}, err
	}
	return s.FindByKey(ctx, input.Key)
}

func (s *ProcessingJobStore) FindByKey(ctx context.Context, key string) (mediaprocessing.Job, error) {
	return scanProcessingJob(s.db.QueryRowContext(ctx, `SELECT id,job_key,job_kind,gallery_id,item_uuid,variant,
		content_revision,profile_hash,payload_json,status,priority,attempt_count,max_attempts,not_before_utc,
		lease_owner,lease_expires_at_utc,last_error_code,structural_failure,created_at_utc,updated_at_utc
		FROM processing_jobs WHERE job_key=?`, key))
}

func (s *ProcessingJobStore) List(ctx context.Context, status string, page int) (ProcessingJobPage, error) {
	if page < 1 || page > 1_000_000 {
		return ProcessingJobPage{}, errors.New("processing job page is out of range")
	}
	where, args := "", []any{}
	if status != "" && status != "ALL" {
		valid := false
		for _, candidate := range []mediaprocessing.JobStatus{mediaprocessing.JobPending, mediaprocessing.JobRunning, mediaprocessing.JobRetryWait, mediaprocessing.JobPaused, mediaprocessing.JobCompleted, mediaprocessing.JobFailed, mediaprocessing.JobCancelled} {
			if status == string(candidate) {
				valid = true
				break
			}
		}
		if !valid {
			return ProcessingJobPage{}, errors.New("invalid processing job status")
		}
		where, args = " WHERE status=?", append(args, status)
	}
	result := ProcessingJobPage{Page: page, PageSize: 50}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs`+where, args...).Scan(&result.TotalItems); err != nil {
		return ProcessingJobPage{}, err
	}
	result.TotalPages = int(math.Ceil(float64(result.TotalItems) / float64(result.PageSize)))
	queryArgs := append(append([]any{}, args...), result.PageSize, (page-1)*result.PageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT id,job_key,job_kind,gallery_id,item_uuid,variant,
		content_revision,profile_hash,payload_json,status,priority,attempt_count,max_attempts,not_before_utc,
		lease_owner,lease_expires_at_utc,last_error_code,structural_failure,created_at_utc,updated_at_utc
		FROM processing_jobs`+where+` ORDER BY CASE status WHEN 'FAILED' THEN 0 WHEN 'RUNNING' THEN 1 WHEN 'PENDING' THEN 2 WHEN 'RETRY_WAIT' THEN 3 ELSE 4 END,updated_at_utc DESC,id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return ProcessingJobPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		value, err := scanProcessingJob(rows)
		if err != nil {
			return ProcessingJobPage{}, err
		}
		result.Items = append(result.Items, value)
	}
	return result, rows.Err()
}

func (s *ProcessingJobStore) ClaimNext(ctx context.Context, owner string, leaseDuration time.Duration, now time.Time) (mediaprocessing.Job, error) {
	if owner == "" || len(owner) > 200 || leaseDuration <= 0 {
		return mediaprocessing.Job{}, errors.New("invalid processing job lease")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return mediaprocessing.Job{}, err
	}
	defer func() { _ = tx.Rollback() }()
	timestamp := normalisedTime(now)
	if _, err := tx.ExecContext(ctx, `UPDATE processing_jobs SET status='PENDING',lease_owner=NULL,lease_expires_at_utc=NULL,
		last_heartbeat_at_utc=NULL,updated_at_utc=? WHERE status='RUNNING' AND lease_expires_at_utc<=?`, formatTime(timestamp), formatTime(timestamp)); err != nil {
		return mediaprocessing.Job{}, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM processing_jobs WHERE status IN ('PENDING','RETRY_WAIT') AND not_before_utc<=?
		ORDER BY priority DESC,id LIMIT 1`, formatTime(timestamp)).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return mediaprocessing.Job{}, ErrJobNotClaimable
	} else if err != nil {
		return mediaprocessing.Job{}, err
	}
	leaseUntil := timestamp.Add(leaseDuration)
	result, err := tx.ExecContext(ctx, `UPDATE processing_jobs SET status='RUNNING',attempt_count=attempt_count+1,
		lease_owner=?,lease_expires_at_utc=?,last_heartbeat_at_utc=?,updated_at_utc=?
		WHERE id=? AND status IN ('PENDING','RETRY_WAIT')`, owner, formatTime(leaseUntil), formatTime(timestamp), formatTime(timestamp), id)
	if err != nil {
		return mediaprocessing.Job{}, err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return mediaprocessing.Job{}, ErrJobNotClaimable
	}
	job, err := scanProcessingJob(tx.QueryRowContext(ctx, `SELECT id,job_key,job_kind,gallery_id,item_uuid,variant,
		content_revision,profile_hash,payload_json,status,priority,attempt_count,max_attempts,not_before_utc,
		lease_owner,lease_expires_at_utc,last_error_code,structural_failure,created_at_utc,updated_at_utc
		FROM processing_jobs WHERE id=?`, id))
	if err != nil {
		return mediaprocessing.Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return mediaprocessing.Job{}, err
	}
	return job, nil
}

func (s *ProcessingJobStore) Heartbeat(ctx context.Context, id int64, owner string, extend time.Duration, now time.Time) error {
	if extend <= 0 {
		return errors.New("lease extension must be positive")
	}
	timestamp := normalisedTime(now)
	result, err := s.db.ExecContext(ctx, `UPDATE processing_jobs SET lease_expires_at_utc=?,last_heartbeat_at_utc=?,updated_at_utc=?
		WHERE id=? AND status='RUNNING' AND lease_owner=? AND lease_expires_at_utc>?`, formatTime(timestamp.Add(extend)), formatTime(timestamp), formatTime(timestamp), id, owner, formatTime(timestamp))
	return requireLeaseRow(result, err)
}

func (s *ProcessingJobStore) Complete(ctx context.Context, id int64, owner string, now time.Time) error {
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `UPDATE processing_jobs SET status='COMPLETED',lease_owner=NULL,
		lease_expires_at_utc=NULL,last_heartbeat_at_utc=NULL,completed_at_utc=?,updated_at_utc=?
		WHERE id=? AND status='RUNNING' AND lease_owner=?`, timestamp, timestamp, id, owner)
	return requireLeaseRow(result, err)
}

func (s *ProcessingJobStore) Fail(ctx context.Context, id int64, owner, errorCode string, structural bool, now time.Time) (mediaprocessing.JobStatus, error) {
	if len(errorCode) > 100 {
		return "", errors.New("processing error code exceeds 100 characters")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var attempts, maximum int
	if err := tx.QueryRowContext(ctx, `SELECT attempt_count,max_attempts FROM processing_jobs WHERE id=? AND status='RUNNING' AND lease_owner=?`, id, owner).Scan(&attempts, &maximum); errors.Is(err, sql.ErrNoRows) {
		return "", ErrJobLeaseLost
	} else if err != nil {
		return "", err
	}
	status := mediaprocessing.JobRetryWait
	if structural || attempts >= maximum {
		status = mediaprocessing.JobFailed
	}
	timestamp := normalisedTime(now)
	notBefore := timestamp
	if status == mediaprocessing.JobRetryWait {
		delay := time.Second * time.Duration(1<<min(attempts, 10))
		notBefore = timestamp.Add(delay)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE processing_jobs SET status=?,lease_owner=NULL,lease_expires_at_utc=NULL,
		last_heartbeat_at_utc=NULL,last_error_code=?,structural_failure=?,not_before_utc=?,updated_at_utc=? WHERE id=?`,
		status, errorCode, structural, formatTime(notBefore), formatTime(timestamp), id); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return status, nil
}

func (s *ProcessingJobStore) Cancel(ctx context.Context, id int64, now time.Time) error {
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `UPDATE processing_jobs SET status='CANCELLED',lease_owner=NULL,
		lease_expires_at_utc=NULL,last_heartbeat_at_utc=NULL,updated_at_utc=?
		WHERE id=? AND status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED')`, timestamp, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("processing job is terminal or missing")
	}
	return nil
}

// Requeue is an explicit technical operation used after an ENHANCED cache
// artifact is evicted or an owner requests retry. Ordinary duplicate Enqueue
// never silently reopens a terminal job.
func (s *ProcessingJobStore) Requeue(ctx context.Context, key string, priority int, now time.Time) (mediaprocessing.Job, error) {
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `UPDATE processing_jobs SET status='PENDING',priority=MAX(priority,?),
		attempt_count=0,not_before_utc=?,lease_owner=NULL,lease_expires_at_utc=NULL,last_heartbeat_at_utc=NULL,
		last_error_code='',structural_failure=0,completed_at_utc=NULL,updated_at_utc=?
		WHERE job_key=? AND status IN ('COMPLETED','FAILED','CANCELLED')`, priority, timestamp, timestamp, key)
	if err != nil {
		return mediaprocessing.Job{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return mediaprocessing.Job{}, err
	}
	if rows != 1 {
		return mediaprocessing.Job{}, errors.New("processing job is not terminal or missing")
	}
	return s.FindByKey(ctx, key)
}

func (s *ProcessingJobStore) RequeueID(ctx context.Context, id int64, priority int, now time.Time) (mediaprocessing.Job, error) {
	var key string
	if err := s.db.QueryRowContext(ctx, `SELECT job_key FROM processing_jobs WHERE id=?`, id).Scan(&key); err != nil {
		return mediaprocessing.Job{}, err
	}
	return s.Requeue(ctx, key, priority, now)
}

func requireLeaseRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrJobLeaseLost
	}
	return nil
}

func validJobKind(kind mediaprocessing.JobKind) bool {
	switch kind {
	case mediaprocessing.JobLibraryScan, mediaprocessing.JobGalleryProcessing, mediaprocessing.JobItemDerivative,
		mediaprocessing.JobItemTechnicalMetadata, mediaprocessing.JobManifest, mediaprocessing.JobCache, mediaprocessing.JobBackup:
		return true
	default:
		return false
	}
}

func ItemTechnicalMetadataJobKey(itemUUID string, contentRevision int64, profileHash string) string {
	return fmt.Sprintf("item:%s:video-probe:%d:%s", itemUUID, contentRevision, profileHash)
}

type rowScanner interface{ Scan(...any) error }

func scanProcessingJob(row rowScanner) (mediaprocessing.Job, error) {
	var job mediaprocessing.Job
	var galleryID, contentRevision sql.NullInt64
	var itemUUID, leaseOwner, leaseUntil sql.NullString
	var structural int
	var notBefore, created, updated string
	if err := row.Scan(&job.ID, &job.Key, &job.Kind, &galleryID, &itemUUID, &job.Variant,
		&contentRevision, &job.ProfileHash, &job.PayloadJSON, &job.Status, &job.Priority,
		&job.AttemptCount, &job.MaxAttempts, &notBefore, &leaseOwner, &leaseUntil,
		&job.LastErrorCode, &structural, &created, &updated); err != nil {
		return mediaprocessing.Job{}, err
	}
	if galleryID.Valid {
		value := galleryID.Int64
		job.GalleryID = &value
	}
	if contentRevision.Valid {
		value := contentRevision.Int64
		job.ContentRevision = &value
	}
	job.ItemUUID, job.LeaseOwner, job.StructuralFailure = itemUUID.String, leaseOwner.String, structural == 1
	var err error
	if job.NotBeforeUTC, err = parseTime(notBefore); err != nil {
		return mediaprocessing.Job{}, err
	}
	if job.CreatedAtUTC, err = parseTime(created); err != nil {
		return mediaprocessing.Job{}, err
	}
	if job.UpdatedAtUTC, err = parseTime(updated); err != nil {
		return mediaprocessing.Job{}, err
	}
	if leaseUntil.Valid {
		value, err := parseTime(leaseUntil.String)
		if err != nil {
			return mediaprocessing.Job{}, err
		}
		job.LeaseExpiresUTC = &value
	}
	return job, nil
}

func ItemDerivativeJobKey(itemUUID, variant string, contentRevision int64, profileHash string) string {
	return fmt.Sprintf("item:%s:%s:%d:%s", itemUUID, variant, contentRevision, profileHash)
}
