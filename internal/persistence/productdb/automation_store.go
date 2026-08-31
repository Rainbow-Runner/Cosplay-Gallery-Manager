package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

var ErrAutomationRunNotClaimable = errors.New("automation run is not claimable")

type AutomationMode string

const (
	AutomationManual   AutomationMode = "MANUAL"
	AutomationAssisted AutomationMode = "ASSISTED"
	AutomationTrusted  AutomationMode = "TRUSTED"
)

type LibraryAutomationPolicy struct {
	LibraryID                     int64
	Mode                          AutomationMode
	DefaultContentRating          gallery.ContentRating
	ExcludeNewRootMedia           bool
	AutoImportArchives            bool
	AutoAcceptUniqueEntities      bool
	AutoAcceptMediaClassification bool
	AutoActivate                  bool
	Revision                      int64
}

type AutomationPreview struct {
	CandidateCount, AutoCreateEligible, DraftCount, ActivationReady, NeedsReview int
}

type AutomationRun struct {
	ID                                                int64
	LibraryID                                         int64
	PolicyRevision                                    int64
	Mode                                              AutomationMode
	DefaultContentRating                              gallery.ContentRating
	ExcludeNewRootMedia                               bool
	AutoImportArchives                                bool
	AutoAcceptUniqueEntities                          bool
	AutoAcceptMediaClassification                     bool
	AutoActivate                                      bool
	Status                                            string
	DiscoveryCompleted                                bool
	CursorGalleryID                                   int64
	CancellationRequested                             bool
	CandidatesSeen, DraftsCreated, Scanned, Activated int
	NeedsReview                                       int
	IssueCount                                        int
	ErrorCode                                         string
	StartedAtUTC                                      time.Time
	CompletedAtUTC                                    *time.Time
	LeaseOwner                                        string
	LeaseExpiresAtUTC                                 *time.Time
	LastHeartbeatAtUTC                                *time.Time
}

type AutomationStore struct{ db *sql.DB }

func (db *Database) Automation() *AutomationStore { return &AutomationStore{db: db.DB} }

func (s *AutomationStore) FindPolicy(ctx context.Context, libraryID int64) (LibraryAutomationPolicy, error) {
	var result LibraryAutomationPolicy
	var rating sql.NullString
	var excludeRoot, importArchives, acceptEntities, acceptClassification, activate int
	err := s.db.QueryRowContext(ctx, `SELECT library_id,mode,default_content_rating,exclude_new_root_media,
		auto_import_archives,auto_accept_unique_entities,auto_accept_media_classification,auto_activate,revision
		FROM library_automation_policies WHERE library_id=?`, libraryID).Scan(&result.LibraryID, &result.Mode, &rating,
		&excludeRoot, &importArchives, &acceptEntities, &acceptClassification, &activate, &result.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		var exists int
		if findErr := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_libraries WHERE id=?`, libraryID).Scan(&exists); findErr != nil {
			return LibraryAutomationPolicy{}, findErr
		}
		if exists != 1 {
			return LibraryAutomationPolicy{}, ErrMediaLibraryNotFound
		}
		return LibraryAutomationPolicy{LibraryID: libraryID, Mode: AutomationManual, ExcludeNewRootMedia: true, Revision: 0}, nil
	}
	if err != nil {
		return LibraryAutomationPolicy{}, err
	}
	result.DefaultContentRating = gallery.ContentRating(rating.String)
	result.ExcludeNewRootMedia = excludeRoot == 1
	result.AutoImportArchives = importArchives == 1
	result.AutoAcceptUniqueEntities = acceptEntities == 1
	result.AutoAcceptMediaClassification = acceptClassification == 1
	result.AutoActivate = activate == 1
	return result, nil
}

func (s *AutomationStore) SavePolicy(ctx context.Context, input LibraryAutomationPolicy, expectedRevision int64, now time.Time) (LibraryAutomationPolicy, error) {
	if input.LibraryID <= 0 {
		return LibraryAutomationPolicy{}, errors.New("automation policy library is required")
	}
	if input.Mode != AutomationManual && input.Mode != AutomationAssisted && input.Mode != AutomationTrusted {
		return LibraryAutomationPolicy{}, errors.New("unsupported automation mode")
	}
	if input.DefaultContentRating != "" && input.DefaultContentRating != gallery.ContentRatingNonAdult && input.DefaultContentRating != gallery.ContentRatingAdult {
		return LibraryAutomationPolicy{}, errors.New("unsupported automation default content rating")
	}
	if input.Mode != AutomationTrusted && input.AutoActivate {
		return LibraryAutomationPolicy{}, errors.New("automatic activation requires TRUSTED mode")
	}
	if input.AutoActivate && input.DefaultContentRating == "" {
		return LibraryAutomationPolicy{}, errors.New("automatic activation requires a default content rating")
	}
	timestamp := formatTime(normalisedTime(now))
	if expectedRevision == 0 {
		_, err := s.db.ExecContext(ctx, `INSERT INTO library_automation_policies
			(library_id,mode,default_content_rating,exclude_new_root_media,auto_import_archives,auto_accept_unique_entities,
			auto_accept_media_classification,auto_activate,revision,created_at_utc,updated_at_utc)
			VALUES (?,?,NULLIF(?,''),?,?,?,?,?,1,?,?)`, input.LibraryID, input.Mode, input.DefaultContentRating,
			boolInt(input.ExcludeNewRootMedia), boolInt(input.AutoImportArchives), boolInt(input.AutoAcceptUniqueEntities), boolInt(input.AutoAcceptMediaClassification),
			boolInt(input.AutoActivate), timestamp, timestamp)
		if err != nil {
			return LibraryAutomationPolicy{}, err
		}
	} else {
		result, err := s.db.ExecContext(ctx, `UPDATE library_automation_policies SET mode=?,default_content_rating=NULLIF(?,''),
			exclude_new_root_media=?,auto_import_archives=?,auto_accept_unique_entities=?,auto_accept_media_classification=?,auto_activate=?,
			revision=revision+1,updated_at_utc=? WHERE library_id=? AND revision=?`, input.Mode, input.DefaultContentRating,
			boolInt(input.ExcludeNewRootMedia), boolInt(input.AutoImportArchives), boolInt(input.AutoAcceptUniqueEntities), boolInt(input.AutoAcceptMediaClassification),
			boolInt(input.AutoActivate), timestamp, input.LibraryID, expectedRevision)
		if err != nil {
			return LibraryAutomationPolicy{}, err
		}
		if err := requireOneRevisionRow(result); err != nil {
			return LibraryAutomationPolicy{}, err
		}
	}
	return s.FindPolicy(ctx, input.LibraryID)
}

func (s *AutomationStore) Preview(ctx context.Context, libraryID int64) (AutomationPreview, error) {
	policy, err := s.FindPolicy(ctx, libraryID)
	if err != nil {
		return AutomationPreview{}, err
	}
	var result AutomationPreview
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM((auto_create_draft=1 OR (?=1 AND source_type='ARCHIVE')) AND has_conflict=0 AND over_limit=0 AND status='PENDING'),0)
		FROM gallery_candidates WHERE snapshot_id=(SELECT id FROM discovery_snapshots WHERE library_id=? ORDER BY id DESC LIMIT 1)`, boolInt(policy.AutoImportArchives), libraryID).
		Scan(&result.CandidateCount, &result.AutoCreateEligible); err != nil {
		return AutomationPreview{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT gallery.id FROM galleries gallery JOIN gallery_sources source ON source.gallery_id=gallery.id
		WHERE source.library_id=? AND gallery.state='DRAFT' ORDER BY gallery.id`, libraryID)
	if err != nil {
		return AutomationPreview{}, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return AutomationPreview{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return AutomationPreview{}, err
	}
	result.DraftCount = len(ids)
	for _, id := range ids {
		value, err := findGallery(ctx, s.db, id)
		if err != nil {
			return AutomationPreview{}, err
		}
		facts, err := activationFacts(ctx, s.db, id)
		if err != nil {
			return AutomationPreview{}, err
		}
		if len(value.ActivationBlockers(facts)) == 0 {
			result.ActivationReady++
		} else {
			result.NeedsReview++
		}
	}
	return result, nil
}

func (s *AutomationStore) EnqueueRun(ctx context.Context, policy LibraryAutomationPolicy, now time.Time) (AutomationRun, error) {
	if policy.Mode == AutomationManual || policy.Revision <= 0 {
		return AutomationRun{}, errors.New("automation policy must be explicitly saved as ASSISTED or TRUSTED")
	}
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `INSERT INTO library_automation_runs
		(library_id,policy_revision,mode,default_content_rating,exclude_new_root_media,auto_import_archives,auto_accept_unique_entities,
		auto_accept_media_classification,auto_activate,status,started_at_utc) VALUES (?,?,?,NULLIF(?,''),?,?,?,?,?,'QUEUED',?)`,
		policy.LibraryID, policy.Revision, policy.Mode, policy.DefaultContentRating, boolInt(policy.ExcludeNewRootMedia),
		boolInt(policy.AutoImportArchives), boolInt(policy.AutoAcceptUniqueEntities), boolInt(policy.AutoAcceptMediaClassification), boolInt(policy.AutoActivate), timestamp)
	if err != nil {
		return AutomationRun{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return AutomationRun{}, err
	}
	return s.FindRun(ctx, id)
}

func (s *AutomationStore) CompleteRun(ctx context.Context, run AutomationRun, owner, status, errorCode string, now time.Time) (AutomationRun, error) {
	if status != "COMPLETED" && status != "FAILED" && status != "CANCELLED" {
		return AutomationRun{}, errors.New("unsupported automation terminal status")
	}
	if len(errorCode) > 100 {
		return AutomationRun{}, errors.New("automation error code exceeds 100 characters")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE library_automation_runs SET status=?,candidates_seen=?,drafts_created=?,
		scanned=?,activated=?,needs_review=?,error_code=?,completed_at_utc=?,lease_owner=NULL,lease_expires_at_utc=NULL,
		last_heartbeat_at_utc=? WHERE id=? AND status='RUNNING' AND lease_owner=?`,
		status, run.CandidatesSeen, run.DraftsCreated, run.Scanned, run.Activated, run.NeedsReview,
		errorCode, formatTime(normalisedTime(now)), formatTime(normalisedTime(now)), run.ID, owner)
	if err != nil {
		return AutomationRun{}, err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return AutomationRun{}, err
	}
	return s.FindRun(ctx, run.ID)
}

func (s *AutomationStore) ClaimNextRun(ctx context.Context, owner string, lease time.Duration, now time.Time) (AutomationRun, error) {
	if owner == "" || lease <= 0 {
		return AutomationRun{}, errors.New("automation worker owner and lease are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AutomationRun{}, err
	}
	defer func() { _ = tx.Rollback() }()
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `UPDATE library_automation_runs SET status='QUEUED',lease_owner=NULL,
		lease_expires_at_utc=NULL,last_heartbeat_at_utc=NULL,error_code='INTERRUPTED'
		WHERE status='RUNNING' AND lease_expires_at_utc<?`, timestamp); err != nil {
		return AutomationRun{}, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM library_automation_runs WHERE status='QUEUED' ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AutomationRun{}, ErrAutomationRunNotClaimable
		}
		return AutomationRun{}, err
	}
	expires := formatTime(normalisedTime(now).Add(lease))
	result, err := tx.ExecContext(ctx, `UPDATE library_automation_runs SET status='RUNNING',lease_owner=?,
		lease_expires_at_utc=?,last_heartbeat_at_utc=?,error_code='' WHERE id=? AND status='QUEUED'`, owner, expires, timestamp, id)
	if err != nil {
		return AutomationRun{}, err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return AutomationRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationRun{}, err
	}
	return s.FindRun(ctx, id)
}

func (s *AutomationStore) RequeueRun(ctx context.Context, run AutomationRun, owner string, now time.Time) (AutomationRun, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE library_automation_runs SET status='QUEUED',discovery_completed=?,
		cursor_gallery_id=?,candidates_seen=?,drafts_created=?,scanned=?,activated=?,needs_review=?,lease_owner=NULL,
		lease_expires_at_utc=NULL,last_heartbeat_at_utc=? WHERE id=? AND status='RUNNING' AND lease_owner=?`,
		boolInt(run.DiscoveryCompleted), run.CursorGalleryID, run.CandidatesSeen, run.DraftsCreated, run.Scanned,
		run.Activated, run.NeedsReview, formatTime(normalisedTime(now)), run.ID, owner)
	if err != nil {
		return AutomationRun{}, err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return AutomationRun{}, err
	}
	return s.FindRun(ctx, run.ID)
}

func (s *AutomationStore) ReleaseRun(ctx context.Context, id int64, owner string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE library_automation_runs SET status='QUEUED',lease_owner=NULL,
		lease_expires_at_utc=NULL,last_heartbeat_at_utc=?,error_code='INTERRUPTED'
		WHERE id=? AND status='RUNNING' AND lease_owner=?`, formatTime(normalisedTime(now)), id, owner)
	if err != nil {
		return err
	}
	return requireOneRevisionRow(result)
}

func (s *AutomationStore) CancellationRequested(ctx context.Context, id int64, owner string) (bool, error) {
	var requested int
	if err := s.db.QueryRowContext(ctx, `SELECT cancellation_requested FROM library_automation_runs
		WHERE id=? AND status='RUNNING' AND lease_owner=?`, id, owner).Scan(&requested); err != nil {
		return false, err
	}
	return requested == 1, nil
}

func (s *AutomationStore) HeartbeatRun(ctx context.Context, id int64, owner string, lease time.Duration, now time.Time) error {
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `UPDATE library_automation_runs SET lease_expires_at_utc=?,
		last_heartbeat_at_utc=? WHERE id=? AND status='RUNNING' AND lease_owner=?`,
		formatTime(normalisedTime(now).Add(lease)), timestamp, id, owner)
	if err != nil {
		return err
	}
	return requireOneRevisionRow(result)
}

func (s *AutomationStore) SaveRunProgress(ctx context.Context, run AutomationRun, owner string, now time.Time) (AutomationRun, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE library_automation_runs SET discovery_completed=?,cursor_gallery_id=?,
		candidates_seen=?,drafts_created=?,scanned=?,activated=?,needs_review=?,last_heartbeat_at_utc=?
		WHERE id=? AND status='RUNNING' AND lease_owner=?`, boolInt(run.DiscoveryCompleted), run.CursorGalleryID,
		run.CandidatesSeen, run.DraftsCreated, run.Scanned, run.Activated, run.NeedsReview,
		formatTime(normalisedTime(now)), run.ID, owner)
	if err != nil {
		return AutomationRun{}, err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return AutomationRun{}, err
	}
	return s.FindRun(ctx, run.ID)
}

func (s *AutomationStore) RequestCancel(ctx context.Context, id int64, now time.Time) (AutomationRun, error) {
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `UPDATE library_automation_runs SET
		status=CASE WHEN status='QUEUED' THEN 'CANCELLED' ELSE status END,
		cancellation_requested=CASE WHEN status='RUNNING' THEN 1 ELSE cancellation_requested END,
		completed_at_utc=CASE WHEN status='QUEUED' THEN ? ELSE completed_at_utc END
		WHERE id=? AND status IN ('QUEUED','RUNNING')`, timestamp, id)
	if err != nil {
		return AutomationRun{}, err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return AutomationRun{}, err
	}
	return s.FindRun(ctx, id)
}

func (s *AutomationStore) FindRun(ctx context.Context, id int64) (AutomationRun, error) {
	return scanAutomationRun(s.db.QueryRowContext(ctx, `SELECT id,library_id,policy_revision,mode,status,candidates_seen,
		drafts_created,scanned,activated,needs_review,error_code,started_at_utc,completed_at_utc,discovery_completed,
		cursor_gallery_id,cancellation_requested,lease_owner,lease_expires_at_utc,last_heartbeat_at_utc,
		default_content_rating,exclude_new_root_media,auto_import_archives,auto_accept_unique_entities,auto_accept_media_classification,auto_activate,
		(SELECT COUNT(*) FROM library_automation_run_issues issue WHERE issue.run_id=library_automation_runs.id)
		FROM library_automation_runs WHERE id=?`, id))
}

func (s *AutomationStore) FindActiveRun(ctx context.Context, libraryID int64) (*AutomationRun, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM library_automation_runs WHERE library_id=?
		AND status IN ('QUEUED','RUNNING') ORDER BY id DESC LIMIT 1`, libraryID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value, err := s.FindRun(ctx, id)
	return &value, err
}

func (s *AutomationStore) RecentRuns(ctx context.Context, libraryID int64, limit int) ([]AutomationRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,library_id,policy_revision,mode,status,candidates_seen,
		drafts_created,scanned,activated,needs_review,error_code,started_at_utc,completed_at_utc,discovery_completed,
		cursor_gallery_id,cancellation_requested,lease_owner,lease_expires_at_utc,last_heartbeat_at_utc,
		default_content_rating,exclude_new_root_media,auto_import_archives,auto_accept_unique_entities,auto_accept_media_classification,auto_activate,
		(SELECT COUNT(*) FROM library_automation_run_issues issue WHERE issue.run_id=library_automation_runs.id)
		FROM library_automation_runs WHERE library_id=? ORDER BY id DESC LIMIT ?`, libraryID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AutomationRun
	for rows.Next() {
		value, err := scanAutomationRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func scanAutomationRun(row rowScanner) (AutomationRun, error) {
	var result AutomationRun
	var started string
	var completed, leaseOwner, leaseExpires, heartbeat sql.NullString
	var rating sql.NullString
	var discoveryCompleted, cancellationRequested, excludeRoot, importArchives, acceptEntities, acceptClassification, activate int
	if err := row.Scan(&result.ID, &result.LibraryID, &result.PolicyRevision, &result.Mode, &result.Status,
		&result.CandidatesSeen, &result.DraftsCreated, &result.Scanned, &result.Activated,
		&result.NeedsReview, &result.ErrorCode, &started, &completed, &discoveryCompleted,
		&result.CursorGalleryID, &cancellationRequested, &leaseOwner, &leaseExpires, &heartbeat,
		&rating, &excludeRoot, &importArchives, &acceptEntities, &acceptClassification, &activate, &result.IssueCount); err != nil {
		return AutomationRun{}, err
	}
	result.DiscoveryCompleted = discoveryCompleted == 1
	result.CancellationRequested = cancellationRequested == 1
	result.LeaseOwner = leaseOwner.String
	result.DefaultContentRating = gallery.ContentRating(rating.String)
	result.ExcludeNewRootMedia = excludeRoot == 1
	result.AutoImportArchives = importArchives == 1
	result.AutoAcceptUniqueEntities = acceptEntities == 1
	result.AutoAcceptMediaClassification = acceptClassification == 1
	result.AutoActivate = activate == 1
	var err error
	result.StartedAtUTC, err = parseTime(started)
	if err != nil {
		return AutomationRun{}, err
	}
	if completed.Valid {
		value, err := parseTime(completed.String)
		if err != nil {
			return AutomationRun{}, err
		}
		result.CompletedAtUTC = &value
	}
	if leaseExpires.Valid {
		value, err := parseTime(leaseExpires.String)
		if err != nil {
			return AutomationRun{}, err
		}
		result.LeaseExpiresAtUTC = &value
	}
	if heartbeat.Valid {
		value, err := parseTime(heartbeat.String)
		if err != nil {
			return AutomationRun{}, err
		}
		result.LastHeartbeatAtUTC = &value
	}
	return result, nil
}

func (s *AutomationStore) RecordRunIssue(ctx context.Context, runID, galleryID, sourceID int64, stage, errorCode string, now time.Time) error {
	if stage != "SCAN" && stage != "POLICY" && stage != "ACTIVATION" {
		return errors.New("unsupported automation issue stage")
	}
	if errorCode == "" || len(errorCode) > 100 {
		return errors.New("invalid automation issue error code")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO library_automation_run_issues
		(run_id,gallery_id,source_id,stage,error_code,created_at_utc) VALUES(?,?,?,?,?,?)
		ON CONFLICT(run_id,gallery_id,stage) DO UPDATE SET source_id=excluded.source_id,error_code=excluded.error_code,
		created_at_utc=excluded.created_at_utc`, runID, galleryID, sourceID, stage, errorCode, formatTime(normalisedTime(now)))
	return err
}
