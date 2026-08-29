package productdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/media"
	"github.com/stashapp/stash/internal/mediaexclusion"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/sourcescan"
)

var (
	ErrScanNotStaging   = errors.New("Gallery scan is not in STAGING state")
	ErrScanOverLimit    = errors.New("Gallery scan exceeds the 1000 member hard limit")
	ErrSourceScanUnsafe = errors.New("GallerySource scan was not safe to commit")
)

type ScanStore struct {
	db *sql.DB
}

// ScanOptions controls user-facing reconciliation policy without changing
// source discovery. ExcludeNewRootMedia applies only to newly created Items;
// an Item matched by path or rebound by fingerprint keeps its durable choice.
type ScanOptions struct {
	ExcludeNewRootMedia bool
}

func (db *Database) Scans() *ScanStore {
	return &ScanStore{db: db.DB}
}

type ScanObservation = sourcescan.Observation

func (s *ScanStore) Begin(ctx context.Context, sourceID int64, now time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_scan_runs (source_id, status, started_at_utc)
		SELECT ?, 'STAGING', ? WHERE EXISTS (SELECT 1 FROM gallery_sources WHERE id = ?)
	`, sourceID, formatTime(normalisedTime(now)), sourceID)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rows != 1 {
		return 0, errors.New("GallerySource not found")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE gallery_sources SET reconcile_state = 'SCANNING', updated_at_utc = ? WHERE id = ?
	`, formatTime(normalisedTime(now)), sourceID); err != nil {
		return 0, err
	}
	runID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return runID, nil
}

func (s *ScanStore) Stage(ctx context.Context, scanRunID int64, observation ScanObservation) error {
	observation.ContentFormat = effectiveContentFormat(observation.MediaKind, observation.ContentFormat)
	if err := validateScanObservation(observation); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_scan_observations (
			scan_run_id, relative_path, media_kind, content_format, image_category, byte_size,
			quick_fingerprint, full_fingerprint, processing_state
		)
		SELECT ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?
		WHERE EXISTS (
			SELECT 1 FROM gallery_scan_runs WHERE id = ? AND status = 'STAGING'
		)
	`, scanRunID, observation.RelativePath, observation.MediaKind,
		observation.ContentFormat, observation.ImageCategory, observation.ByteSize, observation.QuickFingerprint,
		observation.FullFingerprint, observation.ProcessingState, scanRunID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrScanNotStaging
	}
	return nil
}

// Abort discards all staged observations and leaves the last committed
// GallerySource and GalleryItem state unchanged.
func (s *ScanStore) Abort(ctx context.Context, scanRunID int64, cancelled bool, errorCode string, now time.Time) error {
	status := "FAILED"
	if cancelled {
		status = "CANCELLED"
	}
	if len([]rune(errorCode)) > 100 {
		return errors.New("scan error code exceeds 100 characters")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_scan_observations WHERE scan_run_id = ?`, scanRunID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE gallery_scan_runs SET status = ?, completed_at_utc = ?, error_code = ?
		WHERE id = ? AND status = 'STAGING'
	`, status, formatTime(normalisedTime(now)), errorCode, scanRunID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrScanNotStaging
	}
	reconcile := gallery.ReconcileError
	if cancelled {
		reconcile = gallery.ReconcileNeedsRescan
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE gallery_sources SET reconcile_state = ?, updated_at_utc = ?
		WHERE id = (SELECT source_id FROM gallery_scan_runs WHERE id = ?)
	`, reconcile, formatTime(normalisedTime(now)), scanRunID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ScanStore) Commit(ctx context.Context, scanRunID int64, now time.Time) error {
	return s.commit(ctx, scanRunID, nil, ScanOptions{}, now)
}

// CommitWithOptions is exposed for policy-focused integration tests. Normal
// physical scans should use RunWithOptions so staging and commit stay atomic.
func (s *ScanStore) CommitWithOptions(ctx context.Context, scanRunID int64, options ScanOptions, now time.Time) error {
	return s.commit(ctx, scanRunID, nil, options, now)
}

func (s *ScanStore) commit(ctx context.Context, scanRunID int64, issues []sourcescan.Issue, options ScanOptions, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var sourceID, galleryID int64
	var sourceType gallery.SourceType
	var sourceLibraryID sql.NullInt64
	var runStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT run.source_id, source.gallery_id, source.source_type, source.library_id, run.status
		FROM gallery_scan_runs run
		JOIN gallery_sources source ON source.id = run.source_id
		WHERE run.id = ?
	`, scanRunID).Scan(&sourceID, &galleryID, &sourceType, &sourceLibraryID, &runStatus); err != nil {
		return err
	}
	if runStatus != "STAGING" {
		return ErrScanNotStaging
	}

	observations, err := loadScanObservations(ctx, tx, scanRunID)
	if err != nil {
		return err
	}
	existing, err := loadSourceItems(ctx, tx, sourceID)
	if err != nil {
		return err
	}
	ruleExclusions := map[string]scanRuleExclusion{}
	var libraryID *int64
	if sourceLibraryID.Valid {
		libraryID = &sourceLibraryID.Int64
	}
	compiledRules, err := loadCompiledMediaExclusionRules(ctx, tx, libraryID)
	if err != nil {
		return err
	}
	for _, observation := range observations {
		winner, matchedValue := matchMediaExclusionRules(compiledRules, observation.RelativePath, string(observation.MediaKind))
		if winner != nil && winner.Rule().Decision == mediaexclusion.DecisionExclude {
			ruleExclusions[observation.RelativePath] = scanRuleExclusion{rule: winner.Rule(), matchedValue: matchedValue}
		}
	}
	if effectiveScanMemberCount(observations, existing, options, ruleExclusions) > 1000 {
		if err := markScanOverLimit(ctx, tx, scanRunID, sourceID, galleryID, now); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return ErrScanOverLimit
	}

	byPath := make(map[string]*gallery.Item, len(existing))
	maxPosition := int64(0)
	for index := range existing {
		item := &existing[index]
		byPath[item.RelativePath] = item
		if item.Position > maxPosition {
			maxPosition = item.Position
		}
	}
	if err := syncScannerIssues(ctx, tx, sourceID, issues, now); err != nil {
		return err
	}

	observationPaths := make(map[string]struct{}, len(observations))
	fingerprintObservationCount := make(map[string]int)
	for _, observation := range observations {
		observationPaths[observation.RelativePath] = struct{}{}
		if observation.FullFingerprint != "" {
			fingerprintObservationCount[observation.FullFingerprint]++
		}
	}
	rebindCandidates := make(map[string][]*gallery.Item)
	for index := range existing {
		item := &existing[index]
		if _, stillAtPath := observationPaths[item.RelativePath]; stillAtPath || item.FullFingerprint == "" {
			continue
		}
		rebindCandidates[item.FullFingerprint] = append(rebindCandidates[item.FullFingerprint], item)
	}

	sort.SliceStable(observations, func(i int, j int) bool {
		if mediaGroup(observations[i]) != mediaGroup(observations[j]) {
			return mediaGroup(observations[i]) < mediaGroup(observations[j])
		}
		return media.NaturalLess(observations[i].RelativePath, observations[j].RelativePath)
	})
	seenItemIDs := make(map[int64]struct{}, len(observations))
	for _, observation := range observations {
		if item := byPath[observation.RelativePath]; item != nil {
			if err := updateObservedItem(ctx, tx, item, observation, scanRunID, now); err != nil {
				return err
			}
			seenItemIDs[item.ID] = struct{}{}
			continue
		}

		candidates := rebindCandidates[observation.FullFingerprint]
		if observation.FullFingerprint != "" && fingerprintObservationCount[observation.FullFingerprint] == 1 && len(candidates) == 1 {
			item := candidates[0]
			if _, alreadySeen := seenItemIDs[item.ID]; !alreadySeen {
				if err := rebindObservedItem(ctx, tx, item, observation, scanRunID, now); err != nil {
					return err
				}
				seenItemIDs[item.ID] = struct{}{}
				continue
			}
		}

		maxPosition += 1024
		rootExcluded := options.ExcludeNewRootMedia && isRootRelativePath(observation.RelativePath)
		ruleExclusion, ruleExcluded := ruleExclusions[observation.RelativePath]
		excluded := rootExcluded || ruleExcluded
		itemID, itemUUID, err := insertObservedItem(
			ctx, tx, galleryID, sourceID, observation, maxPosition,
			excluded, scanRunID, now,
		)
		if err != nil {
			return err
		}
		if ruleExcluded && !rootExcluded {
			if err := recordAppliedMediaExclusion(ctx, tx, galleryID, itemUUID, ruleExclusion.rule, ruleExclusion.matchedValue, now); err != nil {
				return err
			}
		}
		seenItemIDs[itemID] = struct{}{}
	}

	for index := range existing {
		if _, seen := seenItemIDs[existing[index].ID]; seen {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE gallery_items SET availability_state = 'MISSING', updated_at_utc = ?
			WHERE id = ?
		`, formatTime(normalisedTime(now)), existing[index].ID); err != nil {
			return err
		}
	}

	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM gallery_scan_observations WHERE scan_run_id = ?;
		UPDATE gallery_scan_runs SET status = 'COMPLETED', completed_at_utc = ? WHERE id = ?;
		UPDATE gallery_sources SET availability_state = 'AVAILABLE',
			reconcile_state = 'IN_SYNC', over_limit = 0, updated_at_utc = ? WHERE id = ?;
		UPDATE galleries SET scan_revision = scan_revision + 1, scrubber_revision = scrubber_revision + 1 WHERE id = ?;
		UPDATE gallery_source_issues SET resolved_at_utc = ?
			WHERE source_id = ? AND code = 'OVER_LIMIT' AND resolved_at_utc IS NULL
	`, scanRunID, timestamp, scanRunID, timestamp, sourceID, galleryID, timestamp, sourceID); err != nil {
		return err
	}
	if err := syncScanItemSuggestions(ctx, tx, galleryID, sourceID, now); err != nil {
		return err
	}
	if err := enqueueScanProcessingJobs(ctx, tx, galleryID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func enqueueScanProcessingJobs(ctx context.Context, tx *sql.Tx, galleryID int64, now time.Time) error {
	var state gallery.State
	if err := tx.QueryRowContext(ctx, `SELECT state FROM galleries WHERE id=?`, galleryID).Scan(&state); err != nil {
		return err
	}
	priority := 100
	if state == gallery.StateActive {
		priority = 400
	}
	if state == gallery.StateArchived {
		priority = 10
	}
	rows, err := tx.QueryContext(ctx, `SELECT item.item_uuid,item.media_kind,item.content_format,item.content_revision,source.source_type
		FROM gallery_items item JOIN gallery_sources source ON source.id=item.source_id
		WHERE item.gallery_id=? AND item.excluded=0 AND item.availability_state='AVAILABLE' AND item.processing_state='PENDING'`, galleryID)
	if err != nil {
		return err
	}
	type pending struct {
		uuid     string
		kind     gallery.MediaKind
		format   gallery.ContentFormat
		revision int64
		source   gallery.SourceType
	}
	var items []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.uuid, &item.kind, &item.format, &item.revision, &item.source); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	profileHash := mediaprocessing.DefaultProfileHash()
	timestamp := formatTime(normalisedTime(now))
	for _, item := range items {
		if item.kind == gallery.MediaKindVideo {
			if item.source != gallery.SourceTypeDirectory {
				continue
			}
			key := ItemTechnicalMetadataJobKey(item.uuid, item.revision, profileHash)
			if _, err := tx.ExecContext(ctx, `INSERT INTO video_technical_metadata(item_uuid,content_revision,probe_profile_hash,probe_state)
				VALUES(?,?,?,'PENDING') ON CONFLICT(item_uuid) DO UPDATE SET content_revision=excluded.content_revision,probe_profile_hash=excluded.probe_profile_hash,
				probe_state='PENDING',last_error_code='',completed_at_utc=NULL`, item.uuid, item.revision, profileHash); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO processing_jobs (job_key,job_kind,gallery_id,item_uuid,variant,content_revision,profile_hash,payload_json,status,priority,max_attempts,not_before_utc,created_at_utc,updated_at_utc)
				VALUES (?,'ITEM_TECHNICAL_METADATA',? ,?,'',?,?,'{}','PENDING',?,3,?,?,?) ON CONFLICT(job_key) DO UPDATE SET priority=MAX(priority,excluded.priority),updated_at_utc=excluded.updated_at_utc`,
				key, galleryID, item.uuid, item.revision, profileHash, priority, timestamp, timestamp, timestamp); err != nil {
				return err
			}
			continue
		}
		plans := []struct {
			variant string
			tier    mediaprocessing.CacheTier
		}{
			{mediaprocessing.VariantCard480, mediaprocessing.CacheBase},
		}
		if item.kind == gallery.MediaKindAnimatedImage {
			plans = []struct {
				variant string
				tier    mediaprocessing.CacheTier
			}{{mediaprocessing.VariantStaticPoster, mediaprocessing.CacheBase}}
		}
		for _, plan := range plans {
			key := ItemDerivativeJobKey(item.uuid, plan.variant, item.revision, profileHash)
			payload, _ := json.Marshal(map[string]any{"cache_tier": plan.tier})
			if _, err := tx.ExecContext(ctx, `INSERT INTO processing_jobs (job_key,job_kind,gallery_id,item_uuid,variant,
			content_revision,profile_hash,payload_json,status,priority,max_attempts,not_before_utc,created_at_utc,updated_at_utc)
			VALUES (?,'ITEM_DERIVATIVE',?,?,?,?,?,?,'PENDING',?,3,?,?,?)
			ON CONFLICT(job_key) DO UPDATE SET priority=MAX(priority,excluded.priority),updated_at_utc=excluded.updated_at_utc`,
				key, galleryID, item.uuid, plan.variant, item.revision, profileHash, payload, priority, timestamp, timestamp, timestamp); err != nil {
				return err
			}
		}
	}
	return nil
}

// Run scans with the product default: newly discovered media directly in the
// Gallery root is retained as a durable excluded Item for explicit review.
func (s *ScanStore) Run(
	ctx context.Context,
	sourceID int64,
	archiveLimits archivecheck.Limits,
	now time.Time,
) error {
	return s.RunWithOptions(ctx, sourceID, archiveLimits, ScanOptions{ExcludeNewRootMedia: true}, now)
}

// RunWithOptions scans the bound physical source and submits one atomic source
// snapshot. It never writes to or deletes from the media source.
func (s *ScanStore) RunWithOptions(
	ctx context.Context,
	sourceID int64,
	archiveLimits archivecheck.Limits,
	options ScanOptions,
	now time.Time,
) error {
	source, err := findSource(ctx, s.db, sourceID)
	if err != nil {
		return err
	}
	runID, err := s.Begin(ctx, sourceID, now)
	if err != nil {
		return err
	}

	var result sourcescan.Result
	if source.Type == gallery.SourceTypeArchive {
		result, err = sourcescan.ScanArchive(ctx, source.Path, archiveLimits)
	} else {
		result, err = sourcescan.ScanDirectory(ctx, source.Path)
	}
	if err != nil {
		_ = s.Abort(ctx, runID, false, "SOURCE_READ_FAILED", now)
		_ = (&GalleryStore{db: s.db}).SetSourceHealth(
			ctx, sourceID, gallery.AvailabilityUnreadable, gallery.ReconcileError, false, now,
		)
		return err
	}
	if !result.Complete {
		if err := s.Abort(ctx, runID, false, "UNSAFE_SOURCE", now); err != nil {
			return err
		}
		if err := replaceScannerIssues(ctx, s.db, sourceID, result.Issues, now); err != nil {
			return err
		}
		return ErrSourceScanUnsafe
	}
	for _, observation := range result.Observations {
		if err := s.Stage(ctx, runID, observation); err != nil {
			_ = s.Abort(ctx, runID, false, "STAGING_FAILED", now)
			return err
		}
	}
	if err := s.commit(ctx, runID, result.Issues, options, now); err != nil {
		return err
	}
	_, err = (&CoverStore{db: s.db, random: rand.Reader}).Initialize(ctx, source.GalleryID, now)
	return err
}

func loadScanObservations(ctx context.Context, tx *sql.Tx, scanRunID int64) ([]ScanObservation, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT relative_path, media_kind, content_format, image_category, byte_size,
			quick_fingerprint, full_fingerprint, processing_state
		FROM gallery_scan_observations WHERE scan_run_id = ?
	`, scanRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var observations []ScanObservation
	for rows.Next() {
		var observation ScanObservation
		var category sql.NullString
		if err := rows.Scan(
			&observation.RelativePath, &observation.MediaKind, &observation.ContentFormat, &category,
			&observation.ByteSize, &observation.QuickFingerprint,
			&observation.FullFingerprint, &observation.ProcessingState,
		); err != nil {
			return nil, err
		}
		observation.ImageCategory = gallery.ImageCategory(category.String)
		observations = append(observations, observation)
	}
	return observations, rows.Err()
}

func loadSourceItems(ctx context.Context, tx *sql.Tx, sourceID int64) ([]gallery.Item, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM gallery_items WHERE source_id = ? ORDER BY id
	`, sourceID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]gallery.Item, 0, len(ids))
	for _, id := range ids {
		item, err := findItem(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func updateObservedItem(
	ctx context.Context,
	tx *sql.Tx,
	item *gallery.Item,
	observation ScanObservation,
	scanRunID int64,
	now time.Time,
) error {
	contentChanged := item.FullFingerprint != "" && observation.FullFingerprint != "" &&
		item.FullFingerprint != observation.FullFingerprint
	category := effectiveScannedCategory(*item, observation)
	processing := observation.ProcessingState
	if contentChanged {
		processing = gallery.ProcessingPending
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE gallery_items SET media_kind = ?, content_format=?, image_category = NULLIF(?, ''),
			availability_state = 'AVAILABLE', processing_state = ?, byte_size = ?,
			quick_fingerprint = ?, full_fingerprint = ?,
			content_revision = content_revision + ?, last_seen_scan_run_id = ?,
			updated_at_utc = ?
		WHERE id = ?
	`, observation.MediaKind, observation.ContentFormat, category, processing, observation.ByteSize,
		observation.QuickFingerprint, observation.FullFingerprint, boolInt(contentChanged),
		scanRunID, formatTime(normalisedTime(now)), item.ID)
	if err != nil || !contentChanged {
		return err
	}
	newRevision := item.ContentRevision + 1
	if _, err := tx.ExecContext(ctx, `UPDATE media_derivatives SET state='HARD_INVALID',is_current=0
		WHERE item_uuid=? AND content_revision<>?`, item.UUID, newRevision); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM video_technical_metadata WHERE item_uuid=? AND content_revision<>?`, item.UUID, newRevision); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE processing_jobs SET status='CANCELLED',lease_owner=NULL,lease_expires_at_utc=NULL,
		last_heartbeat_at_utc=NULL,updated_at_utc=? WHERE item_uuid=? AND content_revision<>?
		AND status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED')`, formatTime(normalisedTime(now)), item.UUID, newRevision)
	return err
}

func rebindObservedItem(
	ctx context.Context,
	tx *sql.Tx,
	item *gallery.Item,
	observation ScanObservation,
	scanRunID int64,
	now time.Time,
) error {
	category := effectiveScannedCategory(*item, observation)
	_, err := tx.ExecContext(ctx, `
		UPDATE gallery_items SET relative_path = ?, media_kind = ?, content_format=?,
			image_category = NULLIF(?, ''), availability_state = 'AVAILABLE',
			processing_state = ?, byte_size = ?, quick_fingerprint = ?,
			full_fingerprint = ?, last_seen_scan_run_id = ?, updated_at_utc = ?
		WHERE id = ?
	`, observation.RelativePath, observation.MediaKind, observation.ContentFormat, category,
		observation.ProcessingState, observation.ByteSize, observation.QuickFingerprint,
		observation.FullFingerprint, scanRunID, formatTime(normalisedTime(now)), item.ID)
	return err
}

func insertObservedItem(
	ctx context.Context,
	tx *sql.Tx,
	galleryID int64,
	sourceID int64,
	observation ScanObservation,
	position int64,
	excluded bool,
	scanRunID int64,
	now time.Time,
) (int64, string, error) {
	timestamp := normalisedTime(now)
	itemUUID := portableid.New()
	if _, err := registerPortableUUID(ctx, tx, itemUUID, portableid.KindGalleryItem, timestamp); err != nil {
		return 0, "", err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_items (
			item_uuid, gallery_id, source_id, relative_path, media_kind,
			content_format, image_category, position, availability_state, processing_state,
			byte_size, quick_fingerprint, full_fingerprint, excluded,
			last_seen_scan_run_id, created_at_utc, updated_at_utc
		) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, 'AVAILABLE', ?, ?, ?, ?, ?, ?, ?, ?)
	`, itemUUID, galleryID, sourceID, observation.RelativePath, observation.MediaKind,
		observation.ContentFormat, observation.ImageCategory, position, observation.ProcessingState,
		observation.ByteSize, observation.QuickFingerprint, observation.FullFingerprint, boolInt(excluded),
		scanRunID, formatTime(timestamp), formatTime(timestamp))
	if err != nil {
		return 0, "", err
	}
	id, err := result.LastInsertId()
	return id, itemUUID, err
}

func markScanOverLimit(
	ctx context.Context,
	tx *sql.Tx,
	scanRunID int64,
	sourceID int64,
	galleryID int64,
	now time.Time,
) error {
	timestamp := formatTime(normalisedTime(now))
	_, err := tx.ExecContext(ctx, `
		DELETE FROM gallery_scan_observations WHERE scan_run_id = ?;
		UPDATE gallery_scan_runs SET status = 'FAILED', completed_at_utc = ?, error_code = 'OVER_LIMIT'
			WHERE id = ?;
		UPDATE gallery_sources SET over_limit = 1, reconcile_state = 'ERROR', updated_at_utc = ?
			WHERE id = ?;
		UPDATE galleries SET scan_revision = scan_revision + 1, scrubber_revision = scrubber_revision + 1 WHERE id = ?;
		INSERT INTO gallery_source_issues (
			source_id, code, severity, message, created_at_utc, resolved_at_utc
		) VALUES (?, 'OVER_LIMIT', 'BLOCKING', 'Gallery source exceeds 1000 members', ?, NULL)
		ON CONFLICT(source_id, code) DO UPDATE SET
			severity = 'BLOCKING', message = excluded.message,
			created_at_utc = excluded.created_at_utc, resolved_at_utc = NULL
	`, scanRunID, timestamp, scanRunID, timestamp, sourceID, galleryID, sourceID, timestamp)
	return err
}

func replaceScannerIssues(
	ctx context.Context,
	db *sql.DB,
	sourceID int64,
	issues []sourcescan.Issue,
	now time.Time,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := syncScannerIssues(ctx, tx, sourceID, issues, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE gallery_sources SET reconcile_state = 'ERROR', updated_at_utc = ? WHERE id = ?;
		UPDATE galleries SET scan_revision = scan_revision + 1, scrubber_revision = scrubber_revision + 1
		WHERE id = (SELECT gallery_id FROM gallery_sources WHERE id = ?)
	`, formatTime(normalisedTime(now)), sourceID, sourceID); err != nil {
		return err
	}
	return tx.Commit()
}

func syncScannerIssues(
	ctx context.Context,
	tx *sql.Tx,
	sourceID int64,
	issues []sourcescan.Issue,
	now time.Time,
) error {
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `
		UPDATE gallery_source_issues SET resolved_at_utc = ?
		WHERE source_id = ? AND code GLOB 'SCAN_*' AND resolved_at_utc IS NULL
	`, timestamp, sourceID); err != nil {
		return err
	}
	type groupedIssue struct {
		blocking bool
		messages []string
	}
	grouped := make(map[string]*groupedIssue)
	for _, issue := range issues {
		if issue.Code == "UNSUPPORTED_ARCHIVE_MEDIA" && issue.RelativePath != "" {
			var excluded int
			err := tx.QueryRowContext(ctx, `
				SELECT excluded FROM gallery_items WHERE source_id = ? AND relative_path = ?
			`, sourceID, issue.RelativePath).Scan(&excluded)
			if err == nil && excluded == 1 {
				continue
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		code := "SCAN_" + issue.Code
		entry := grouped[code]
		if entry == nil {
			entry = &groupedIssue{}
			grouped[code] = entry
		}
		entry.blocking = entry.blocking || issue.Blocking
		message := issue.Message
		if issue.RelativePath != "" {
			message = issue.RelativePath + ": " + message
		}
		if len(entry.messages) < 10 {
			entry.messages = append(entry.messages, message)
		}
	}
	for code, issue := range grouped {
		severity := gallery.IssueSeverityWarning
		if issue.blocking {
			severity = gallery.IssueSeverityBlocking
		}
		message := truncateRunes(strings.Join(issue.messages, "; "), 2000)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_source_issues (
				source_id, code, severity, message, created_at_utc, resolved_at_utc
			) VALUES (?, ?, ?, ?, ?, NULL)
			ON CONFLICT(source_id, code) DO UPDATE SET severity = excluded.severity,
				message = excluded.message, created_at_utc = excluded.created_at_utc,
				resolved_at_utc = NULL
		`, sourceID, code, severity, message, timestamp); err != nil {
			return err
		}
	}
	return nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func validateScanObservation(observation ScanObservation) error {
	observation.ContentFormat = effectiveContentFormat(observation.MediaKind, observation.ContentFormat)
	input := CreateItemInput{
		RelativePath:    observation.RelativePath,
		MediaKind:       observation.MediaKind,
		ImageCategory:   observation.ImageCategory,
		Position:        1,
		Availability:    gallery.AvailabilityAvailable,
		ProcessingState: observation.ProcessingState,
	}
	if err := validateItemInput(input); err != nil {
		return err
	}
	if observation.ProcessingState == gallery.ProcessingProcessing {
		return errors.New("scan observations cannot enter the transient PROCESSING state")
	}
	if observation.ByteSize < 0 {
		return errors.New("scan observation byte size cannot be negative")
	}
	if observation.FullFingerprint != "" && !strings.HasPrefix(observation.FullFingerprint, "blake3-v1:") {
		return fmt.Errorf("unsupported full fingerprint format %q", observation.FullFingerprint)
	}
	if len(observation.QuickFingerprint) > 200 || len(observation.FullFingerprint) > 200 {
		return errors.New("scan observation fingerprint exceeds 200 characters")
	}
	return nil
}

// effectiveScanMemberCount applies durable exclusions before enforcing the
// hard Gallery limit. A successful rescan must never silently restore an
// excluded item merely because the file remains present or moved within the
// same source.
type scanRuleExclusion struct {
	rule         MediaExclusionRule
	matchedValue string
}

func effectiveScanMemberCount(observations []ScanObservation, existing []gallery.Item, options ScanOptions, ruleExclusions map[string]scanRuleExclusion) int {
	byPath := make(map[string]gallery.Item, len(existing))
	observationPaths := make(map[string]struct{}, len(observations))
	observationFingerprintCount := make(map[string]int)
	for _, item := range existing {
		byPath[item.RelativePath] = item
	}
	for _, observation := range observations {
		observationPaths[observation.RelativePath] = struct{}{}
		if observation.FullFingerprint != "" {
			observationFingerprintCount[observation.FullFingerprint]++
		}
	}
	missingByFingerprint := make(map[string][]gallery.Item)
	for _, item := range existing {
		if item.FullFingerprint == "" {
			continue
		}
		if _, remainsAtPath := observationPaths[item.RelativePath]; remainsAtPath {
			continue
		}
		missingByFingerprint[item.FullFingerprint] = append(missingByFingerprint[item.FullFingerprint], item)
	}

	count := 0
	for _, observation := range observations {
		if item, found := byPath[observation.RelativePath]; found && item.Excluded {
			continue
		}
		candidates := missingByFingerprint[observation.FullFingerprint]
		if observation.FullFingerprint != "" &&
			observationFingerprintCount[observation.FullFingerprint] == 1 &&
			len(candidates) == 1 && candidates[0].Excluded {
			continue
		}
		if _, found := byPath[observation.RelativePath]; !found &&
			!(observation.FullFingerprint != "" && observationFingerprintCount[observation.FullFingerprint] == 1 && len(candidates) == 1) &&
			(options.ExcludeNewRootMedia && isRootRelativePath(observation.RelativePath) || ruleExclusions[observation.RelativePath].rule.ID != 0) {
			continue
		}
		count++
	}
	return count
}

func isRootRelativePath(relativePath string) bool {
	return !strings.Contains(relativePath, "/")
}

func effectiveScannedCategory(item gallery.Item, observation ScanObservation) gallery.ImageCategory {
	if observation.MediaKind != gallery.MediaKindStaticImage {
		return ""
	}
	if item.MediaKind == gallery.MediaKindStaticImage && item.ImageCategory != "" {
		return item.ImageCategory
	}
	return gallery.ImageCategoryPhoto
}

func mediaGroup(observation ScanObservation) int {
	if observation.MediaKind == gallery.MediaKindStaticImage {
		if observation.ImageCategory == gallery.ImageCategorySelfie {
			return 1
		}
		return 0
	}
	if observation.MediaKind == gallery.MediaKindAnimatedImage {
		return 2
	}
	return 3
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
