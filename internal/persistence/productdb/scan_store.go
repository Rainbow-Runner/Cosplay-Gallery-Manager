package productdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/coversimilarity"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/media"
	"github.com/stashapp/stash/internal/mediaexclusion"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/sourcescan"
)

var (
	ErrScanNotStaging       = errors.New("Gallery scan is not in STAGING state")
	ErrScanOverLimit        = errors.New("Gallery scan exceeds the 1000 member hard limit")
	ErrSourceScanUnsafe     = errors.New("GallerySource scan was not safe to commit")
	ErrSourceScanInProgress = errors.New("GallerySource scan is already in progress")
	ErrSourceScanConflict   = errors.New("GallerySource changed before scan commit")
)

var activePhysicalScans sync.Map

type physicalScanKey struct {
	db       *sql.DB
	sourceID int64
}

type ScanStore struct {
	db *sql.DB
}

// ScanOptions controls user-facing reconciliation policy without changing
// source discovery. ExcludeNewRootMedia applies only to newly created Items;
// an Item matched by path or rebound by fingerprint keeps its durable choice.
type ScanOptions struct {
	ExcludeNewRootMedia bool
	// ForceContentRead bypasses reusable filesystem evidence. This is the
	// explicit deep-validation policy; ordinary scans still enumerate all paths.
	ForceContentRead bool
	// PortableItemUUIDByPath is used only by the explicit portable rebuild
	// workflow after an exact exported Manifest hash has been verified.
	PortableImportID       string
	PortableItemUUIDByPath map[string]string
	// RegisterManifestItemUUIDs is restricted to an explicitly revalidated
	// unclaimed Manifest import. Portable rebuilds must use claims instead.
	RegisterManifestItemUUIDs bool
	archiveEvidence           *archiveScanEvidence
	expectedScanRevision      *int64
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
	if observation.SourceModifiedStatus == "" {
		observation.SourceModifiedStatus = "NONE"
	}
	if err := validateScanObservation(observation); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_scan_observations (
			scan_run_id, relative_path, media_kind, content_format, image_category, byte_size,
			quick_fingerprint, full_fingerprint, processing_state,
			source_modified_at_utc, source_modified_status, source_modified_origin
		)
		SELECT ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?
		WHERE EXISTS (
			SELECT 1 FROM gallery_scan_runs WHERE id = ? AND status = 'STAGING'
		)
	`, scanRunID, observation.RelativePath, observation.MediaKind,
		observation.ContentFormat, observation.ImageCategory, observation.ByteSize, observation.QuickFingerprint,
		observation.FullFingerprint, observation.ProcessingState, observation.SourceModifiedAtUTC,
		observation.SourceModifiedStatus, observation.SourceModifiedOrigin, scanRunID)
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
		AND reconcile_state = 'SCANNING'
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
	if options.expectedScanRevision != nil {
		var currentRevision int64
		if err := tx.QueryRowContext(ctx, `SELECT scan_revision FROM galleries WHERE id=?`, galleryID).Scan(&currentRevision); err != nil {
			return err
		}
		if currentRevision != *options.expectedScanRevision {
			return ErrSourceScanConflict
		}
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
	claimedPortablePaths := make(map[string]bool, len(options.PortableItemUUIDByPath))
	manifestBusinessChanged := false
	var addedCount, missingCount, changedCount, reboundCount, clearedCount int
	invalidatedCoverItems := make(map[string]bool)
	var similarityCandidates []string
	addedByKind := make(map[gallery.MediaKind]int)
	for _, observation := range observations {
		if item := byPath[observation.RelativePath]; item != nil {
			if desired := options.PortableItemUUIDByPath[observation.RelativePath]; desired != "" {
				if item.UUID != desired {
					return errors.New("portable Manifest Item path is occupied by another UUID")
				}
				claimedPortablePaths[observation.RelativePath] = true
			}
			changed, err := updateObservedItem(ctx, tx, item, observation, scanRunID, now)
			if err != nil {
				return err
			}
			if changed {
				changedCount++
				clearedCount++
				manifestBusinessChanged = true
				invalidatedCoverItems[item.UUID] = true
				if observation.MediaKind == gallery.MediaKindStaticImage {
					similarityCandidates = append(similarityCandidates, item.UUID)
				}
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
				manifestBusinessChanged = true
				reboundCount++
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
			excluded, scanRunID, options.PortableImportID, options.RegisterManifestItemUUIDs,
			options.PortableItemUUIDByPath[observation.RelativePath], now,
		)
		if err != nil {
			return err
		}
		manifestBusinessChanged = true
		addedCount++
		if observation.MediaKind == gallery.MediaKindStaticImage {
			similarityCandidates = append(similarityCandidates, itemUUID)
		}
		addedByKind[observation.MediaKind]++
		if ruleExcluded && !rootExcluded {
			if err := recordAppliedMediaExclusion(ctx, tx, galleryID, itemUUID, ruleExclusion.rule, ruleExclusion.matchedValue, now); err != nil {
				return err
			}
		}
		seenItemIDs[itemID] = struct{}{}
		if options.PortableItemUUIDByPath[observation.RelativePath] != "" {
			claimedPortablePaths[observation.RelativePath] = true
		}
	}
	if len(options.PortableItemUUIDByPath) != len(claimedPortablePaths) {
		return errors.New("portable Manifest Item path did not match exactly one scanned member")
	}

	missingByKind := make(map[gallery.MediaKind]int)
	for index := range existing {
		if _, seen := seenItemIDs[existing[index].ID]; !seen && existing[index].Availability == gallery.AvailabilityAvailable {
			missingByKind[existing[index].MediaKind]++
		}
	}
	for index := range existing {
		if _, seen := seenItemIDs[existing[index].ID]; seen {
			continue
		}
		if existing[index].Availability == gallery.AvailabilityAvailable {
			missingCount++
			invalidatedCoverItems[existing[index].UUID] = true
			// Without a path or fingerprint match we cannot identify pairs. Only
			// treat balanced same-kind churn as replacement; unmatched removals
			// retain their details in case the source temporarily returns.
			if sourceType == gallery.SourceTypeArchive && addedByKind[existing[index].MediaKind] > 0 && addedByKind[existing[index].MediaKind] == missingByKind[existing[index].MediaKind] {
				if err := clearReplacedItemDetails(ctx, tx, &existing[index]); err != nil {
					return err
				}
				clearedCount++
				manifestBusinessChanged = true
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE gallery_items SET availability_state = 'MISSING', updated_at_utc = ?
			WHERE id = ?
		`, formatTime(normalisedTime(now)), existing[index].ID); err != nil {
			return err
		}
	}
	var oldCoverSignature *coversimilarity.Signature
	if len(similarityCandidates) > 0 {
		cover, found, coverErr := findCover(ctx, tx, galleryID)
		if coverErr != nil {
			return coverErr
		}
		if found && (cover.PreferredKind == gallery.CoverItem || cover.PreferredKind == gallery.CoverAutoRandom) {
			for _, item := range existing {
				if item.UUID == cover.PreferredItemUUID {
					sig, hasSignature, sigErr := loadCoverSignature(ctx, tx, item.UUID, item.ContentRevision)
					if sigErr != nil {
						return sigErr
					}
					if hasSignature {
						oldCoverSignature = &sig
					}
					break
				}
			}
		}
	}
	coverReselected, err := reselectScanCover(ctx, tx, galleryID, invalidatedCoverItems, now)
	if err != nil {
		return err
	}
	if coverReselected {
		manifestBusinessChanged = true
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cover_similarity_intents SET status='STALE',completed_at_utc=?
		WHERE gallery_id=? AND status='PENDING'`, formatTime(normalisedTime(now)), galleryID); err != nil {
		return err
	}
	if coverReselected && len(similarityCandidates) > 0 {
		if err := stageCoverSimilarityIntent(ctx, tx, galleryID, scanRunID, oldCoverSignature, similarityCandidates, now); err != nil {
			return err
		}
	}

	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM gallery_scan_observations WHERE scan_run_id = ?;
		UPDATE gallery_scan_runs SET status = 'COMPLETED', completed_at_utc = ?,added_count=?,missing_count=?,changed_count=?,rebound_count=?,cleared_count=?,cover_reselected=? WHERE id = ?;
		UPDATE gallery_sources SET availability_state = 'AVAILABLE',
			reconcile_state = 'IN_SYNC', over_limit = 0, updated_at_utc = ? WHERE id = ?;
		UPDATE galleries SET scan_revision = scan_revision + 1, scrubber_revision = scrubber_revision + 1 WHERE id = ?;
		UPDATE gallery_source_issues SET resolved_at_utc = ?
			WHERE source_id = ? AND code = 'OVER_LIMIT' AND resolved_at_utc IS NULL
	`, scanRunID, timestamp, addedCount, missingCount, changedCount, reboundCount, clearedCount, boolInt(coverReselected), scanRunID, timestamp, sourceID, galleryID, timestamp, sourceID); err != nil {
		return err
	}
	// Archive issues may be suppressed while a member is excluded. Retaining a
	// reusable snapshot would hide that issue if the owner restores the member.
	if sourceType == gallery.SourceTypeArchive && options.archiveEvidence != nil && len(issues) == 0 {
		evidence := options.archiveEvidence
		if _, err := tx.ExecContext(ctx, `INSERT INTO gallery_source_scan_evidence
			(source_id,container_size,container_modified_at_utc,archive_limits_json,scanner_version,checked_at_utc)
			VALUES (?,?,?,?,?,?) ON CONFLICT(source_id) DO UPDATE SET
			container_size=excluded.container_size,container_modified_at_utc=excluded.container_modified_at_utc,
			archive_limits_json=excluded.archive_limits_json,scanner_version=excluded.scanner_version,
			checked_at_utc=excluded.checked_at_utc`, sourceID, evidence.Size, evidence.ModifiedAtUTC,
			evidence.LimitsJSON, archiveScanEvidenceVersion, timestamp); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_source_scan_evidence WHERE source_id=?`, sourceID); err != nil {
		return err
	}
	if err := syncScanItemSuggestions(ctx, tx, galleryID, sourceID, now); err != nil {
		return err
	}
	if manifestBusinessChanged {
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_manifest_sync SET status='DB_DIRTY' WHERE gallery_id=?`, galleryID); err != nil {
			return err
		}
	}
	if err := enqueueScanProcessingJobs(ctx, tx, galleryID, now); err != nil {
		return err
	}
	if err := reconcileGalleryCaptureDate(ctx, tx, galleryID, now); err != nil {
		return err
	}
	if err := reconcileGalleryMediaAdded(ctx, tx, galleryID); err != nil {
		return err
	}
	return tx.Commit()
}

func reconcileGalleryMediaAdded(ctx context.Context, tx *sql.Tx, galleryID int64) error {
	var eligible, found, pending int
	var start, end sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN source_modified_status='FOUND' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source_modified_status='PENDING' THEN 1 ELSE 0 END),0),
		MIN(CASE WHEN source_modified_status='FOUND' THEN source_modified_at_utc END),
		MAX(CASE WHEN source_modified_status='FOUND' THEN source_modified_at_utc END)
		FROM gallery_items WHERE gallery_id=? AND media_kind='STATIC_IMAGE'
		AND excluded=0 AND availability_state='AVAILABLE'`, galleryID).Scan(&eligible, &found, &pending, &start, &end); err != nil {
		return err
	}
	status := "NONE"
	if eligible > 0 && found == eligible {
		status = "COMPLETE"
	} else if found > 0 {
		status = "PARTIAL"
	} else if pending > 0 {
		status = "PENDING"
	}
	_, err := tx.ExecContext(ctx, `UPDATE galleries SET media_added_start_at_utc=?,media_added_end_at_utc=?,
		media_added_status=?,media_added_revision=media_added_revision+1 WHERE id=?`,
		start.String, end.String, status, galleryID)
	return err
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
	key := physicalScanKey{db: s.db, sourceID: sourceID}
	if _, loaded := activePhysicalScans.LoadOrStore(key, struct{}{}); loaded {
		return ErrSourceScanInProgress
	}
	defer activePhysicalScans.Delete(key)
	var scanRevision int64
	if err := s.db.QueryRowContext(ctx, `SELECT scan_revision FROM galleries WHERE id=?`, source.GalleryID).Scan(&scanRevision); err != nil {
		return err
	}
	options.expectedScanRevision = &scanRevision
	var prior map[string]sourcescan.Observation
	if source.Type == gallery.SourceTypeDirectory && !options.ForceContentRead {
		prior, err = s.reusableDirectoryEvidence(ctx, sourceID)
		if err != nil {
			return err
		}
	}
	var archiveEvidence archiveScanEvidence
	var archiveInfo os.FileInfo
	var reuseArchive bool
	runID, err := s.Begin(ctx, sourceID, now)
	if err != nil {
		return err
	}

	var result sourcescan.Result
	if source.Type == gallery.SourceTypeArchive {
		archiveEvidence, archiveInfo, err = inspectArchiveEvidence(source.Path, archiveLimits)
		if err == nil && !options.ForceContentRead {
			reuseArchive, prior, err = s.reusableArchiveEvidence(ctx, sourceID, archiveEvidence)
		}
		if err == nil {
			if reuseArchive {
				result.Complete = true
				for _, observation := range prior {
					result.Observations = append(result.Observations, observation)
					result.Reused++
				}
			} else {
				result, err = sourcescan.ScanArchive(ctx, source.Path, archiveLimits)
			}
		}
		if err == nil {
			current, statErr := os.Lstat(source.Path)
			if statErr != nil || !os.SameFile(archiveInfo, current) || current.Size() != archiveEvidence.Size ||
				current.ModTime().UTC().Format(time.RFC3339Nano) != archiveEvidence.ModifiedAtUTC {
				err = errors.New("archive source changed while scanning")
			}
		}
		options.archiveEvidence = &archiveEvidence
	} else {
		result, err = sourcescan.ScanDirectoryWithEvidence(ctx, source.Path, prior)
	}
	if err != nil {
		_ = s.Abort(ctx, runID, false, sourceReadErrorCode(err), now)
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
		if !errors.Is(err, ErrScanOverLimit) {
			_ = s.Abort(ctx, runID, false, "SCAN_COMMIT_FAILED", now)
		}
		return err
	}
	_, err = (&CoverStore{db: s.db, random: rand.Reader}).Initialize(ctx, source.GalleryID, now)
	return err
}

// Active scanner issues must be revalidated before they can be resolved. They
// are currently aggregated per source rather than persisted per member, so a
// source with any such issue takes the conservative full-read path.
func (s *ScanStore) reusableDirectoryEvidence(ctx context.Context, sourceID int64) (map[string]sourcescan.Observation, error) {
	var activeIssues int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_source_issues
		WHERE source_id=? AND code GLOB 'SCAN_*' AND resolved_at_utc IS NULL`, sourceID).Scan(&activeIssues); err != nil {
		return nil, err
	}
	if activeIssues > 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT relative_path,media_kind,content_format,image_category,
		byte_size,quick_fingerprint,full_fingerprint,processing_state,
		source_modified_at_utc,source_modified_status,source_modified_origin
		FROM gallery_items WHERE source_id=? AND availability_state='AVAILABLE' AND scan_evidence_version=?`, sourceID, sourceScanEvidenceVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prior := make(map[string]sourcescan.Observation)
	for rows.Next() {
		var observation sourcescan.Observation
		var category sql.NullString
		if err := rows.Scan(&observation.RelativePath, &observation.MediaKind, &observation.ContentFormat,
			&category, &observation.ByteSize, &observation.QuickFingerprint, &observation.FullFingerprint,
			&observation.ProcessingState, &observation.SourceModifiedAtUTC, &observation.SourceModifiedStatus,
			&observation.SourceModifiedOrigin); err != nil {
			return nil, err
		}
		observation.ImageCategory = gallery.ImageCategory(category.String)
		prior[observation.RelativePath] = observation
	}
	return prior, rows.Err()
}

func sourceReadErrorCode(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "SOURCE_NOT_FOUND"
	case errors.Is(err, os.ErrPermission):
		return "SOURCE_PERMISSION_DENIED"
	default:
		return "SOURCE_READ_FAILED"
	}
}

func loadScanObservations(ctx context.Context, tx *sql.Tx, scanRunID int64) ([]ScanObservation, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT relative_path, media_kind, content_format, image_category, byte_size,
			quick_fingerprint, full_fingerprint, processing_state,
			source_modified_at_utc, source_modified_status, source_modified_origin
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
			&observation.SourceModifiedAtUTC, &observation.SourceModifiedStatus, &observation.SourceModifiedOrigin,
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
) (bool, error) {
	contentChanged := item.FullFingerprint != "" && observation.FullFingerprint != "" &&
		item.FullFingerprint != observation.FullFingerprint
	category := effectiveScannedCategory(*item, observation)
	if contentChanged {
		category = observation.ImageCategory
	}
	// A matching content fingerprint does not invalidate previously generated
	// derivatives. Scanner observations start PENDING because new files need
	// processing, but existing unchanged Items retain their durable state.
	processing := item.ProcessingState
	if contentChanged {
		processing = gallery.ProcessingPending
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE gallery_items SET media_kind = ?, content_format=?, image_category = NULLIF(?, ''),
			availability_state = 'AVAILABLE', processing_state = ?, byte_size = ?,
			quick_fingerprint = ?, full_fingerprint = ?,
			scan_evidence_version = ?,
			source_modified_at_utc=?,source_modified_status=?,source_modified_origin=?,source_modified_checked_at_utc=?,
			content_revision = content_revision + ?, last_seen_scan_run_id = ?,
			updated_at_utc = ?
		WHERE id = ?
	`, observation.MediaKind, observation.ContentFormat, category, processing, observation.ByteSize,
		observation.QuickFingerprint, observation.FullFingerprint, sourceScanEvidenceVersion, observation.SourceModifiedAtUTC,
		observation.SourceModifiedStatus, observation.SourceModifiedOrigin, formatTime(normalisedTime(now)), boolInt(contentChanged),
		scanRunID, formatTime(normalisedTime(now)), item.ID)
	if err != nil || !contentChanged {
		return false, err
	}
	if err := clearReplacedItemDetails(ctx, tx, item); err != nil {
		return false, err
	}
	newRevision := item.ContentRevision + 1
	if _, err := tx.ExecContext(ctx, `UPDATE media_derivatives SET state='HARD_INVALID',is_current=0
		WHERE item_uuid=? AND content_revision<>?`, item.UUID, newRevision); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM video_technical_metadata WHERE item_uuid=? AND content_revision<>?`, item.UUID, newRevision); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE processing_jobs SET status='CANCELLED',lease_owner=NULL,lease_expires_at_utc=NULL,
		last_heartbeat_at_utc=NULL,updated_at_utc=? WHERE item_uuid=? AND content_revision<>?
		AND status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED')`, formatTime(normalisedTime(now)), item.UUID, newRevision)
	return true, err
}

// Per-item preferences describe the old bytes, not the new media. Path-based
// exclusion stays intact because it is an explicit safety/visibility decision.
func clearReplacedItemDetails(ctx context.Context, tx *sql.Tx, item *gallery.Item) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_item_personal_states WHERE gallery_item_id=?`, item.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_item_suggestions WHERE item_uuid=?`, item.UUID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE gallery_items SET caption='' WHERE id=?`, item.ID)
	return err
}

func reselectScanCover(ctx context.Context, tx *sql.Tx, galleryID int64, invalidated map[string]bool, now time.Time) (bool, error) {
	state, found, err := findCover(ctx, tx, galleryID)
	if err != nil || !found {
		return false, err
	}
	if state.PreferredKind != gallery.CoverItem && state.PreferredKind != gallery.CoverAutoRandom {
		return false, nil
	}
	if !invalidated[state.PreferredItemUUID] {
		available, err := isStaticMemberAvailable(ctx, tx, galleryID, state.PreferredItemUUID)
		if err != nil {
			return false, err
		}
		if available {
			return false, nil
		}
	}
	selected, err := chooseStaticMember(ctx, tx, galleryID, "", rand.Reader)
	if err != nil {
		return false, err
	}
	state.PreferredKind, state.PreferredItemUUID, state.PreferredPath = gallery.CoverNone, "", ""
	if selected != "" {
		state.PreferredKind, state.PreferredItemUUID = gallery.CoverAutoRandom, selected
	}
	state.Revision++
	state.CanUndo = false
	if err := recomputeEffectiveCover(ctx, tx, &state, false, rand.Reader, now); err != nil {
		return false, err
	}
	if err := persistCoverClearingPrevious(ctx, tx, state); err != nil {
		return false, err
	}
	if err := touchGalleryCoverMetadata(ctx, tx, galleryID, nil, now); err != nil {
		return false, err
	}
	return true, nil
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
			full_fingerprint = ?,scan_evidence_version=?,source_modified_at_utc=?,source_modified_status=?,source_modified_origin=?,
			source_modified_checked_at_utc=?,last_seen_scan_run_id = ?, updated_at_utc = ?
		WHERE id = ?
	`, observation.RelativePath, observation.MediaKind, observation.ContentFormat, category,
		item.ProcessingState, observation.ByteSize, observation.QuickFingerprint,
		observation.FullFingerprint, sourceScanEvidenceVersion, observation.SourceModifiedAtUTC, observation.SourceModifiedStatus,
		observation.SourceModifiedOrigin, formatTime(normalisedTime(now)), scanRunID, formatTime(normalisedTime(now)), item.ID)
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
	portableImportID string,
	registerManifestUUIDs bool,
	portableItemUUID string,
	now time.Time,
) (int64, string, error) {
	timestamp := normalisedTime(now)
	itemUUID := portableItemUUID
	if itemUUID == "" {
		itemUUID = portableid.New()
		if _, err := registerPortableUUID(ctx, tx, itemUUID, portableid.KindGalleryItem, timestamp); err != nil {
			return 0, "", err
		}
	} else if portableImportID == "" {
		if !registerManifestUUIDs {
			return 0, "", errors.New("Manifest GalleryItem UUID requires an explicit import authorization")
		}
		if _, err := registerPortableUUID(ctx, tx, itemUUID, portableid.KindGalleryItem, timestamp); err != nil {
			return 0, "", err
		}
	} else if err := claimPortableUUID(ctx, tx, portableImportID, itemUUID, portableid.KindGalleryItem, timestamp); err != nil {
		return 0, "", err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_items (
			item_uuid, gallery_id, source_id, relative_path, media_kind,
			content_format, image_category, position, availability_state, processing_state,
			byte_size, quick_fingerprint, full_fingerprint, excluded,
			scan_evidence_version,
			source_modified_at_utc,source_modified_status,source_modified_origin,source_modified_checked_at_utc,
			last_seen_scan_run_id, created_at_utc, updated_at_utc
		) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, 'AVAILABLE', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, itemUUID, galleryID, sourceID, observation.RelativePath, observation.MediaKind,
		observation.ContentFormat, observation.ImageCategory, position, observation.ProcessingState,
		observation.ByteSize, observation.QuickFingerprint, observation.FullFingerprint, boolInt(excluded), sourceScanEvidenceVersion,
		observation.SourceModifiedAtUTC, observation.SourceModifiedStatus, observation.SourceModifiedOrigin, formatTime(timestamp),
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
	if observation.SourceModifiedStatus != "FOUND" && observation.SourceModifiedStatus != "NONE" {
		return errors.New("scan observation has invalid source modification status")
	}
	if observation.SourceModifiedStatus == "FOUND" {
		if observation.SourceModifiedAtUTC == "" || (observation.SourceModifiedOrigin != "FILESYSTEM" && observation.SourceModifiedOrigin != "ARCHIVE_ENTRY") {
			return errors.New("scan observation has incomplete source modification evidence")
		}
		if _, err := time.Parse(time.RFC3339Nano, observation.SourceModifiedAtUTC); err != nil {
			return errors.New("scan observation has invalid source modification time")
		}
	} else if observation.SourceModifiedAtUTC != "" || observation.SourceModifiedOrigin != "" {
		return errors.New("scan observation without source modification time has evidence metadata")
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
