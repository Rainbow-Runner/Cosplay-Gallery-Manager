package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
)

// ProcessClaimedRunBatch advances a leased run by a bounded number of
// Galleries. Progress is committed after every Gallery, so a process restart
// repeats at most one idempotent target instead of restarting a large library.
func (s *AutomationStore) ProcessClaimedRunBatch(ctx context.Context, runID int64, owner string, batchSize int, lease time.Duration, now time.Time) (AutomationRun, bool, error) {
	if batchSize <= 0 || batchSize > 100 {
		batchSize = 25
	}
	run, err := s.FindRun(ctx, runID)
	if err != nil {
		return AutomationRun{}, false, err
	}
	if run.Status != "RUNNING" || run.LeaseOwner != owner {
		return run, false, ErrAutomationRunNotClaimable
	}
	ctx, stopHeartbeat := s.startAutomationRunHeartbeat(ctx, run.ID, owner, lease)
	defer stopHeartbeat()
	policy := LibraryAutomationPolicy{LibraryID: run.LibraryID, Mode: run.Mode, DefaultContentRating: run.DefaultContentRating,
		ExcludeNewRootMedia: run.ExcludeNewRootMedia, AutoImportArchives: run.AutoImportArchives, AutoAcceptUniqueEntities: run.AutoAcceptUniqueEntities,
		AutoAcceptMediaClassification: run.AutoAcceptMediaClassification, AutoActivate: run.AutoActivate,
		Revision: run.PolicyRevision}
	fail := func(code string, cause error) (AutomationRun, bool, error) {
		finished, finishErr := s.CompleteRun(context.WithoutCancel(ctx), run, owner, "FAILED", code, time.Now())
		if finishErr != nil {
			return run, true, errors.Join(cause, finishErr)
		}
		return finished, true, cause
	}
	if run.CancellationRequested {
		finished, err := s.CompleteRun(ctx, run, owner, "CANCELLED", "", now)
		return finished, true, err
	}
	if err := s.HeartbeatRun(ctx, run.ID, owner, lease, now); err != nil {
		return run, false, err
	}
	if !run.DiscoveryCompleted {
		before, err := countLibraryDrafts(ctx, s.db, run.LibraryID)
		if err != nil {
			return fail("DRAFT_COUNT_FAILED", err)
		}
		snapshot, err := (&CandidateDiscoveryStore{db: s.db}).DiscoverFilesystemWithOptions(ctx, run.LibraryID,
			DiscoveryOptions{AutoCreateArchives: policy.AutoImportArchives}, now)
		if err != nil {
			return fail("DISCOVERY_FAILED", err)
		}
		run.CandidatesSeen = len(snapshot.Candidates)
		after, err := countLibraryDrafts(ctx, s.db, run.LibraryID)
		if err != nil {
			return fail("DRAFT_COUNT_FAILED", err)
		}
		if after > before {
			run.DraftsCreated = after - before
		}
		run.DiscoveryCompleted = true
		run, err = s.SaveRunProgress(ctx, run, owner, time.Now())
		if err != nil {
			return run, false, err
		}
	}
	settings, err := (&SettingsStore{db: s.db}).Find(ctx)
	if err != nil {
		return fail("SETTINGS_READ_FAILED", err)
	}
	limits := archivecheck.Limits{MaxEntries: settings.ArchiveMaxEntries,
		MaxEntryUncompressed: uint64(settings.ArchiveMaxEntryBytes), MaxTotalUncompressed: uint64(settings.ArchiveMaxTotalBytes),
		MaxCompressionRatio: settings.ArchiveMaxCompressionRatio, MaxImagePixels: uint64(settings.ArchiveMaxImagePixels)}
	targets, err := automationDraftTargets(ctx, s.db, run.LibraryID, run.CursorGalleryID, batchSize)
	if err != nil {
		return fail("DRAFT_LIST_FAILED", err)
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return run, false, context.Cause(ctx)
		}
		cancelled, err := s.CancellationRequested(ctx, run.ID, owner)
		if err != nil {
			return run, false, err
		}
		if cancelled {
			run.CancellationRequested = true
			finished, err := s.CompleteRun(ctx, run, owner, "CANCELLED", "", time.Now())
			return finished, true, err
		}
		needsReview := false
		if target.ReconcileState != gallery.ReconcileInSync {
			if scanErr := (&ScanStore{db: s.db}).RunWithOptions(ctx, target.SourceID, limits,
				ScanOptions{ExcludeNewRootMedia: policy.ExcludeNewRootMedia}, time.Now()); scanErr != nil {
				needsReview = true
				code := "SCAN_FAILED"
				if errors.Is(scanErr, ErrSourceScanUnsafe) {
					code = "SOURCE_SCAN_UNSAFE"
				}
				_ = s.RecordRunIssue(context.WithoutCancel(ctx), run.ID, target.GalleryID, target.SourceID, "SCAN", code, time.Now())
			} else {
				run.Scanned++
			}
		}
		if !needsReview {
			if policyErr := s.applyAutomationPolicy(ctx, target.GalleryID, policy, time.Now()); policyErr != nil {
				needsReview = true
				_ = s.RecordRunIssue(context.WithoutCancel(ctx), run.ID, target.GalleryID, target.SourceID, "POLICY", "POLICY_APPLY_FAILED", time.Now())
			}
		}
		if !needsReview && policy.Mode == AutomationTrusted && policy.AutoActivate {
			value, err := (&GalleryStore{db: s.db}).Find(ctx, target.GalleryID)
			if err == nil {
				_, err = (&GalleryStore{db: s.db}).SetState(ctx, value.ID, value.MetadataRevision, gallery.StateActive, time.Now())
			}
			if err != nil {
				needsReview = true
				code := "ACTIVATION_FAILED"
				var blocked *ActivationError
				if errors.As(err, &blocked) && len(blocked.Blockers) > 0 {
					code = "ACTIVATION_" + blocked.Blockers[0].Code
				}
				_ = s.RecordRunIssue(context.WithoutCancel(ctx), run.ID, target.GalleryID, target.SourceID, "ACTIVATION", code, time.Now())
			} else {
				run.Activated++
			}
		}
		if needsReview || policy.Mode != AutomationTrusted || !policy.AutoActivate {
			run.NeedsReview++
		}
		run.CursorGalleryID = target.GalleryID
		run, err = s.SaveRunProgress(ctx, run, owner, time.Now())
		if err != nil {
			return run, false, err
		}
	}
	remaining, err := automationDraftsRemain(ctx, s.db, run.LibraryID, run.CursorGalleryID)
	if err != nil {
		return fail("DRAFT_LIST_FAILED", err)
	}
	if !remaining {
		finished, err := s.CompleteRun(ctx, run, owner, "COMPLETED", "", time.Now())
		return finished, true, err
	}
	requeued, err := s.RequeueRun(ctx, run, owner, time.Now())
	return requeued, false, err
}

// startAutomationRunHeartbeat keeps a claimed run leased while filesystem or
// media work is in progress. A heartbeat failure cancels the processing
// context with its concrete cause so the caller cannot continue under a lease
// it may no longer own.
func (s *AutomationStore) startAutomationRunHeartbeat(parent context.Context, runID int64, owner string, lease time.Duration) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	done := make(chan struct{})
	interval := lease / 3
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case tick := <-ticker.C:
				if err := s.HeartbeatRun(ctx, runID, owner, lease, tick); err != nil {
					cancel(err)
					return
				}
			}
		}
	}()
	return ctx, func() {
		cancel(context.Canceled)
		<-done
	}
}

type automationDraftTarget struct {
	GalleryID      int64
	SourceID       int64
	ReconcileState gallery.ReconcileState
}

func automationDraftTargets(ctx context.Context, db *sql.DB, libraryID, afterGalleryID int64, limit int) ([]automationDraftTarget, error) {
	rows, err := db.QueryContext(ctx, `SELECT gallery.id,source.id,source.reconcile_state
		FROM galleries gallery JOIN gallery_sources source ON source.gallery_id=gallery.id
		WHERE source.library_id=? AND gallery.state='DRAFT' AND gallery.id>? ORDER BY gallery.id,source.id LIMIT ?`, libraryID, afterGalleryID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []automationDraftTarget
	for rows.Next() {
		var value automationDraftTarget
		if err := rows.Scan(&value.GalleryID, &value.SourceID, &value.ReconcileState); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func automationDraftsRemain(ctx context.Context, db *sql.DB, libraryID, afterGalleryID int64) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM galleries gallery JOIN gallery_sources source
		ON source.gallery_id=gallery.id WHERE source.library_id=? AND gallery.state='DRAFT' AND gallery.id>?)`,
		libraryID, afterGalleryID).Scan(&exists)
	return exists == 1, err
}

func countLibraryDrafts(ctx context.Context, db *sql.DB, libraryID int64) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT gallery.id) FROM galleries gallery
		JOIN gallery_sources source ON source.gallery_id=gallery.id WHERE source.library_id=? AND gallery.state='DRAFT'`, libraryID).Scan(&count)
	return count, err
}

func (s *AutomationStore) applyAutomationPolicy(ctx context.Context, galleryID int64, policy LibraryAutomationPolicy, now time.Time) error {
	if policy.DefaultContentRating == "" && !policy.AutoAcceptUniqueEntities && !policy.AutoAcceptMediaClassification {
		return nil
	}
	value, err := (&GalleryStore{db: s.db}).Find(ctx, galleryID)
	if err != nil {
		return err
	}
	if value.ContentRating == "" && policy.DefaultContentRating != "" {
		value, err = (&GalleryStore{db: s.db}).UpdateMetadata(ctx, galleryID, value.MetadataRevision, UpdateGalleryMetadataInput{
			Title: value.Title, Aliases: value.Aliases, Description: value.Description,
			ShootDate: value.ShootDate, ShootDatePrecision: value.ShootDatePrecision,
			ContentRating: policy.DefaultContentRating, PhotographerName: value.PhotographerName,
			StudioName: value.StudioName,
		}, now)
		if err != nil {
			return err
		}
	}
	if policy.AutoAcceptUniqueEntities {
		if err := s.acceptUniqueIdentityPair(ctx, value, now); err != nil {
			return err
		}
	}
	if policy.AutoAcceptMediaClassification {
		if err := s.acceptClassificationSuggestions(ctx, galleryID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *AutomationStore) acceptClassificationSuggestions(ctx context.Context, galleryID int64, now time.Time) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM media_classification_suggestions
		WHERE gallery_id=? AND status='PENDING' ORDER BY id`, galleryID)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		current, err := (&GalleryStore{db: s.db}).Find(ctx, galleryID)
		if err != nil {
			return err
		}
		if _, err := (&MediaClassificationRuleStore{db: s.db}).ResolveSuggestion(ctx, id, true, current.MetadataRevision, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *AutomationStore) acceptUniqueIdentityPair(ctx context.Context, value gallery.Gallery, now time.Time) error {
	var existing int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_credits WHERE gallery_id=?`, value.ID).Scan(&existing); err != nil || existing > 0 {
		return err
	}
	suggestions, err := loadPendingIdentitySuggestions(ctx, s.db, value.ID)
	if err != nil {
		return err
	}
	coserValues, characterValues, workValues := suggestions["COSER"], suggestions["CHARACTER"], suggestions["WORK"]
	if len(coserValues) != 1 || len(characterValues) != 1 || len(workValues) > 1 {
		return nil
	}
	coserUUIDs, err := exactEntityUUIDs(ctx, s.db, "cosers", "coser_aliases", "coser_uuid", coserValues[0], "")
	if err != nil || len(coserUUIDs) != 1 {
		return err
	}
	workUUID := ""
	if len(workValues) == 1 {
		matches, err := exactEntityUUIDs(ctx, s.db, "works", "work_aliases", "work_uuid", workValues[0], "")
		if err != nil || len(matches) != 1 {
			return err
		}
		workUUID = matches[0]
	}
	characterUUIDs, err := exactEntityUUIDs(ctx, s.db, "characters", "character_aliases", "character_uuid", characterValues[0], workUUID)
	if err != nil || len(characterUUIDs) != 1 {
		return err
	}
	if err := (&GalleryStore{db: s.db}).ReplaceRelations(ctx, value.ID, value.MetadataRevision, ReplaceGalleryRelationsInput{Credits: []ReplaceGalleryCreditInput{{
		CoserUUID: coserUUIDs[0], Position: 1024,
		Cast: []ReplaceGalleryCastInput{{CharacterUUID: characterUUIDs[0], Position: 1024}},
	}}}, now); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE gallery_identity_suggestions SET status='ACCEPTED',resolved_at_utc=?
		WHERE gallery_id=? AND status='PENDING' AND suggestion_kind IN ('COSER','CHARACTER','WORK')`, formatTime(normalisedTime(now)), value.ID)
	return err
}

func loadPendingIdentitySuggestions(ctx context.Context, db *sql.DB, galleryID int64) (map[string][]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT suggestion_kind,value FROM gallery_identity_suggestions
		WHERE gallery_id=? AND status='PENDING' ORDER BY id`, galleryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][]string{}
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return nil, err
		}
		result[kind] = append(result[kind], value)
	}
	return result, rows.Err()
}

// exactEntityUUIDs intentionally performs normalized exact/alias matching only.
// No fuzzy score is allowed to cross the automatic acceptance boundary.
func exactEntityUUIDs(ctx context.Context, db *sql.DB, table, aliasTable, ownerColumn, value, workUUID string) ([]string, error) {
	query := `SELECT uuid,name FROM ` + table
	args := []any{}
	if table == "characters" && workUUID != "" {
		query += ` WHERE work_uuid=?`
		args = append(args, workUUID)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	wanted := normalizedKey(value)
	matches := map[string]struct{}{}
	for rows.Next() {
		var uuid, name string
		if err := rows.Scan(&uuid, &name); err != nil {
			rows.Close()
			return nil, err
		}
		if normalizedKey(name) == wanted {
			matches[uuid] = struct{}{}
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	aliasQuery := `SELECT alias.` + ownerColumn + `,alias.normalized_alias FROM ` + aliasTable + ` alias`
	aliasArgs := []any{}
	if table == "characters" && workUUID != "" {
		aliasQuery += ` JOIN characters entity ON entity.uuid=alias.character_uuid WHERE entity.work_uuid=?`
		aliasArgs = append(aliasArgs, workUUID)
	}
	aliasRows, err := db.QueryContext(ctx, aliasQuery, aliasArgs...)
	if err != nil {
		return nil, err
	}
	for aliasRows.Next() {
		var uuid, alias string
		if err := aliasRows.Scan(&uuid, &alias); err != nil {
			aliasRows.Close()
			return nil, err
		}
		if alias == wanted {
			matches[uuid] = struct{}{}
		}
	}
	if err := aliasRows.Close(); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(matches))
	for uuid := range matches {
		result = append(result, uuid)
	}
	return result, nil
}
