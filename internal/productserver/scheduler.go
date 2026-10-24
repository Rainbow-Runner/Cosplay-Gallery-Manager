package productserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/processingworker"
	"golang.org/x/sys/unix"
)

const dailyBackupTaskKey = "DAILY_BACKUP"
const automaticScanTaskKey = "AUTOMATIC_SCAN"

func (s *Server) runSchedulerLoop(ctx context.Context) {
	// Preserve the startup safety snapshot before any opt-in source scan.
	s.runDailyBackup(ctx, time.Now())
	s.runAutomaticStartupScan(ctx, time.Now())
	s.runBackgroundScheduler(ctx)
}

func (s *Server) runMediaAddedBackfill(ctx context.Context, now time.Time) bool {
	runtime, err := s.Database.Settings().Find(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("CGM_MEDIA_ADDED_BACKFILL_FAILED", "stage", "settings")
		}
		return true
	}
	limits := archivecheck.Limits{
		MaxEntries: runtime.ArchiveMaxEntries, MaxEntryUncompressed: uint64(runtime.ArchiveMaxEntryBytes),
		MaxTotalUncompressed: uint64(runtime.ArchiveMaxTotalBytes), MaxCompressionRatio: runtime.ArchiveMaxCompressionRatio,
		MaxImagePixels: uint64(runtime.ArchiveMaxImagePixels),
	}
	processed, err := s.Database.MediaAdded().BackfillOne(ctx, limits, now)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("CGM_MEDIA_ADDED_BACKFILL_FAILED", "stage", "source_metadata")
		}
		return true
	}
	if processed {
		slog.Info("CGM_MEDIA_ADDED_BACKFILL_COMPLETED")
	}
	return processed
}

func (s *Server) runCaptureDateBackfill(ctx context.Context, now time.Time) bool {
	count, err := s.Database.CaptureDates().EnqueueBackfill(ctx, 25, s.VideoTools.FFprobe.Available, now)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("CGM_CAPTURE_DATE_BACKFILL_FAILED", "error", err)
		}
		return true
	}
	if count > 0 {
		slog.Info("CGM_CAPTURE_DATE_BACKFILL_QUEUED", "item_count", count)
	}
	return count > 0
}

func (s *Server) runMediaTimeBackfill(ctx context.Context, now time.Time) bool {
	reconciled, err := s.Database.MediaTimes().ReconcilePending(ctx, 25)
	if err != nil {
		return true
	}
	count, err := s.Database.MediaTimes().EnqueueBackfill(ctx, 25, s.VideoTools.FFprobe.Available, now)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("CGM_MEDIA_TIME_BACKFILL_FAILED", "error_type", fmt.Sprintf("%T", err))
		}
		return true
	}
	return count > 0 || reconciled > 0
}

func (s *Server) runAutomaticStartupScan(ctx context.Context, now time.Time) {
	if err := s.RunAutomaticStartupScanOnce(ctx, now); err != nil &&
		!errors.Is(err, productdb.ErrScheduledOperationNotDue) &&
		ctx.Err() == nil {
		// The audit stores only technical counts and error codes.
	}
}

func (s *Server) runVideoProbeBackfill(ctx context.Context, now time.Time) bool {
	if !s.VideoTools.FFprobe.Available {
		return false
	}
	count, err := s.Database.VideoMetadata().EnqueueBackfill(ctx, mediaprocessing.VideoProbeProfileHash(s.VideoTools.FFprobe.Version), 25, now)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("CGM_VIDEO_PROBE_BACKFILL_FAILED")
		}
		return true
	}
	if count > 0 {
		slog.Info("CGM_VIDEO_PROBE_BACKFILL_QUEUED", "item_count", count)
	}
	return count > 0
}

// RunCacheMaintenanceOnce first reclaims eligible superseded/deleted/orphaned
// resources, then enforces the ENHANCED quota. This explicit full pass is
// separate from event-driven maintenance. Required BASE/user media are protected.
func (s *Server) RunCacheMaintenanceOnce(ctx context.Context, now time.Time) (processingworker.CacheMaintenanceResult, error) {
	cleaned, err := (processingworker.CacheLifecycleService{
		Database: s.Database, Cache: mediaprocessing.CacheWriter{Root: s.Config.CachePath},
	}).Maintain(ctx, now)
	if err != nil {
		return processingworker.CacheMaintenanceResult{}, err
	}
	if cleaned.Failed > 0 {
		slog.Warn("CGM_CACHE_CLEANUP_RETRY_PENDING", "failed", cleaned.Failed)
	}
	result, err := s.runCachePressureMaintenance(ctx, now)
	result.Removed += cleaned.Removed
	result.FreedBytes += cleaned.FreedBytes
	return result, err
}

func (s *Server) runCachePressureMaintenance(ctx context.Context, now time.Time) (processingworker.CacheMaintenanceResult, error) {
	settings, err := s.Database.Settings().Find(ctx)
	if err != nil {
		return processingworker.CacheMaintenanceResult{}, err
	}
	_, enhanced, err := s.Database.Derivatives().CacheTierBytes(ctx)
	if err != nil {
		return processingworker.CacheMaintenanceResult{}, err
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(s.Config.CachePath, &stat); err != nil {
		return processingworker.CacheMaintenanceResult{}, err
	}
	available := int64(stat.Bavail) * int64(stat.Bsize)
	total := int64(stat.Blocks) * int64(stat.Bsize)
	result, err := processingworker.MaintainEnhancedCache(ctx, s.Database, mediaprocessing.CacheWriter{Root: s.Config.CachePath}, mediaprocessing.CachePressure{
		EnhancedBytes: enhanced, AvailableBytes: available, TotalBytes: total,
		MaximumEnhancedBytes: settings.EnhancedCacheMaximumBytes, MinimumFreeBytes: settings.MinimumFreeBytes,
		MinimumFreePercent: settings.MinimumFreePercent,
	}, 100, now)
	return result, err
}

func (s *Server) runAutomaticScan(ctx context.Context, now time.Time) {
	if err := s.RunAutomaticScanOnce(ctx, now); err != nil &&
		!errors.Is(err, productdb.ErrScheduledOperationNotDue) &&
		ctx.Err() == nil {
		// The audit stores only technical counts and error codes.
	}
}

// Discovery and file events mark changed directory/archive sources NEEDS_RESCAN.
// Reconcile a bounded batch without requiring the owner
// to locate each Gallery and invoke its source scan separately.
func (s *Server) runChangedSourceScans(ctx context.Context, now time.Time) bool {
	settings, err := s.Database.Settings().Find(ctx)
	if err != nil || settings.AutomaticSchedulesSuspended {
		return false
	}
	maintenance, err := s.Database.Operations().Maintenance(ctx)
	if err != nil || maintenance.Mode != productdb.MaintenanceNormal {
		return false
	}
	ids, err := s.Database.ChangedSourceTargets(ctx, 5)
	if err != nil {
		slog.Error("CGM_CHANGED_SOURCE_LIST_FAILED", "error", err)
		return true
	}
	limits := archivecheck.Limits{
		MaxEntries: settings.ArchiveMaxEntries, MaxEntryUncompressed: uint64(settings.ArchiveMaxEntryBytes),
		MaxTotalUncompressed: uint64(settings.ArchiveMaxTotalBytes), MaxCompressionRatio: settings.ArchiveMaxCompressionRatio,
		MaxImagePixels: uint64(settings.ArchiveMaxImagePixels),
	}
	pending := len(ids) >= 5
	for _, id := range ids {
		if err := s.Database.Scans().Run(ctx, id, limits, time.Now()); err != nil {
			pending = true
			slog.Warn("CGM_CHANGED_SOURCE_SCAN_FAILED", "source_id", id, "error", err)
		} else {
			slog.Info("CGM_CHANGED_SOURCE_SCANNED", "source_id", id)
		}
	}
	return pending
}

func (s *Server) runDailyBackup(ctx context.Context, now time.Time) {
	if err := s.RunDailyBackupOnce(ctx, now); err != nil &&
		!errors.Is(err, productdb.ErrScheduledOperationNotDue) &&
		ctx.Err() == nil {
		// Error details intentionally remain in the technical audit and
		// scheduled-operation state rather than being logged with paths.
	}
}

// RunDailyBackupOnce is exposed as a deterministic operation for startup,
// scheduler and regression tests. The persistent lease prevents two process
// instances from producing the same scheduled snapshot concurrently.
func (s *Server) RunDailyBackupOnce(ctx context.Context, now time.Time) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	settings, err := s.Database.Settings().Find(ctx)
	if err != nil {
		return err
	}
	if !settings.DailyBackupEnabled || settings.AutomaticSchedulesSuspended {
		return productdb.ErrScheduledOperationNotDue
	}
	state, err := s.Database.Operations().Maintenance(ctx)
	if err != nil {
		return err
	}
	if state.Mode != productdb.MaintenanceNormal {
		return productdb.ErrScheduledOperationNotDue
	}
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return productdb.ErrScheduledOperationNotDue
	}
	const owner = "local-daily-backup"
	if err := s.Database.Operations().ClaimScheduled(ctx, dailyBackupTaskKey, owner, now.Add(-24*time.Hour), 30*time.Minute, now); err != nil {
		if errors.Is(err, productdb.ErrScheduledOperationNotDue) {
			// Retry retention on startup, settings changes and hourly recovery
			// without creating another snapshot. Respect another backup's lease.
			wake, wakeErr := s.Database.ScheduledNextWake(ctx, dailyBackupTaskKey, 0, now)
			if wakeErr != nil {
				return wakeErr
			}
			if !wake.After(now) {
				if cleanupErr := s.applyDailyBackupRetention(ctx, roots.BackupRoot, settings.DailyBackupRetention, "", now); cleanupErr != nil {
					return cleanupErr
				}
			}
		}
		return err
	}
	version, _, _ := build.Version()
	record, err := s.Database.Backups().CreateDatabaseSnapshot(ctx, roots.BackupRoot, productdb.BackupDailySnapshot, version, now)
	if err != nil {
		_ = s.Database.Operations().FailScheduled(context.Background(), dailyBackupTaskKey, owner, "DAILY_BACKUP_FAILED", now)
		_ = s.Database.Operations().Audit(context.Background(), "SCHEDULED_TASK", "BACKUP", "", "FAILURE", "DAILY_BACKUP_FAILED", map[string]any{"task": dailyBackupTaskKey}, now)
		return err
	}
	if err := s.Database.Operations().CompleteScheduled(ctx, dailyBackupTaskKey, owner, now); err != nil {
		return err
	}
	if err := s.applyDailyBackupRetention(ctx, roots.BackupRoot, settings.DailyBackupRetention, record.ID, now); err != nil {
		return err
	}
	_ = s.Database.Operations().Audit(ctx, "SCHEDULED_TASK", "BACKUP", record.ID, "SUCCESS", "", map[string]any{"task": dailyBackupTaskKey}, now)
	return nil
}

func (s *Server) applyDailyBackupRetention(ctx context.Context, root string, keep int, backupID string, now time.Time) error {
	err := s.Database.Backups().ApplyDailyRetention(ctx, root, keep)
	if err != nil && ctx.Err() == nil {
		_ = s.Database.Operations().Audit(context.Background(), "SCHEDULED_TASK", "BACKUP", backupID, "FAILURE", "DAILY_RETENTION_FAILED", map[string]any{"task": dailyBackupTaskKey}, now)
	}
	return err
}

// RunAutomaticScanOnce performs deterministic discovery followed by source
// reconciliation for enabled libraries. Automatic scanning is opt-in and uses
// the same atomic source scan implementation as the explicit Manage action.
func (s *Server) RunAutomaticScanOnce(ctx context.Context, now time.Time) error {
	return s.runAutomaticScanOnce(ctx, now, false)
}

// RunAutomaticStartupScanOnce runs the same bounded scan pipeline once during
// process startup when the owner explicitly enabled that trigger. It resets
// the periodic interval from this successful run and never bypasses restore
// suspension or maintenance mode.
func (s *Server) RunAutomaticStartupScanOnce(ctx context.Context, now time.Time) error {
	return s.runAutomaticScanOnce(ctx, now, true)
}

func (s *Server) runAutomaticScanOnce(ctx context.Context, now time.Time, startup bool) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	runtime, err := s.Database.Settings().Find(ctx)
	if err != nil {
		return err
	}
	if !runtime.AutomaticScanEnabled || runtime.AutomaticSchedulesSuspended {
		return productdb.ErrScheduledOperationNotDue
	}
	if startup && !runtime.AutomaticScanOnStartup {
		return productdb.ErrScheduledOperationNotDue
	}
	state, err := s.Database.Operations().Maintenance(ctx)
	if err != nil {
		return err
	}
	if state.Mode != productdb.MaintenanceNormal {
		return productdb.ErrScheduledOperationNotDue
	}
	const owner = "local-automatic-scan"
	dueBefore := now.Add(-time.Duration(runtime.AutomaticScanIntervalMinutes) * time.Minute)
	if startup {
		// A startup trigger is explicitly requested and therefore may run before
		// the periodic interval expires. The shared lease still prevents overlap.
		dueBefore = now
	}
	if err := s.Database.Operations().ClaimScheduled(ctx, automaticScanTaskKey, owner, dueBefore, 12*time.Hour, now); err != nil {
		return err
	}
	libraries, err := s.Database.Libraries().List(ctx)
	if err != nil {
		_ = s.Database.Operations().FailScheduled(context.Background(), automaticScanTaskKey, owner, "AUTO_SCAN_LIST_FAILED", now)
		return err
	}
	discovered, discoveryFailures, automationQueued := 0, 0, 0
	automatedLibraries := map[int64]bool{}
	for _, mediaLibrary := range libraries {
		if !mediaLibrary.Enabled {
			continue
		}
		policy, policyErr := s.Database.Automation().FindPolicy(ctx, mediaLibrary.ID)
		if policyErr != nil {
			discoveryFailures++
			continue
		}
		if policy.Mode != productdb.AutomationManual && policy.Revision > 0 {
			automatedLibraries[mediaLibrary.ID] = true
			active, activeErr := s.Database.Automation().FindActiveRun(ctx, mediaLibrary.ID)
			if activeErr != nil {
				discoveryFailures++
				continue
			}
			if active == nil {
				if _, queueErr := s.Database.Automation().EnqueueRun(ctx, policy, now); queueErr != nil {
					discoveryFailures++
				} else {
					automationQueued++
				}
			}
			continue
		}
		if _, err := s.Database.CandidateDiscovery().DiscoverFilesystem(ctx, mediaLibrary.ID, now); err != nil {
			discoveryFailures++
		} else {
			discovered++
		}
	}
	rows, err := s.Database.QueryContext(ctx, `SELECT source.id,source.library_id,gallery.state,source.source_type,source.reconcile_state FROM gallery_sources source
		JOIN galleries gallery ON gallery.id=source.gallery_id
		JOIN media_libraries library ON library.id=source.library_id
		WHERE library.enabled=1 ORDER BY source.id`)
	if err != nil {
		_ = s.Database.Operations().FailScheduled(context.Background(), automaticScanTaskKey, owner, "AUTO_SCAN_SOURCE_LIST_FAILED", now)
		return err
	}
	var sourceIDs []int64
	for rows.Next() {
		var sourceID, libraryID int64
		var galleryState, sourceType, reconcileState string
		if err := rows.Scan(&sourceID, &libraryID, &galleryState, &sourceType, &reconcileState); err != nil {
			rows.Close()
			_ = s.Database.Operations().FailScheduled(context.Background(), automaticScanTaskKey, owner, "AUTO_SCAN_SOURCE_LIST_FAILED", now)
			return err
		}
		if automatedLibraries[libraryID] && galleryState == "DRAFT" {
			continue
		}
		if sourceType == "ARCHIVE" && reconcileState == "IN_SYNC" {
			continue
		}
		sourceIDs = append(sourceIDs, sourceID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		_ = s.Database.Operations().FailScheduled(context.Background(), automaticScanTaskKey, owner, "AUTO_SCAN_SOURCE_LIST_FAILED", now)
		return err
	}
	if err := rows.Close(); err != nil {
		_ = s.Database.Operations().FailScheduled(context.Background(), automaticScanTaskKey, owner, "AUTO_SCAN_SOURCE_LIST_FAILED", now)
		return err
	}
	limits := archivecheck.Limits{
		MaxEntries: runtime.ArchiveMaxEntries, MaxEntryUncompressed: uint64(runtime.ArchiveMaxEntryBytes),
		MaxTotalUncompressed: uint64(runtime.ArchiveMaxTotalBytes), MaxCompressionRatio: runtime.ArchiveMaxCompressionRatio,
		MaxImagePixels: uint64(runtime.ArchiveMaxImagePixels),
	}
	scanned, scanFailures := 0, 0
	for _, sourceID := range sourceIDs {
		if err := s.Database.Scans().Run(ctx, sourceID, limits, now); err != nil {
			scanFailures++
		} else {
			scanned++
		}
	}
	// This phase only observes Manifest files. It never Pushes, Pulls or
	// resolves a conflict. The persisted cursor bounds work for large libraries.
	manifestChecked, manifestFailures := 0, 0
	targets, inspectionErr := s.Database.Manifests().ScheduledInspectionTargets(ctx, 100)
	if inspectionErr != nil {
		manifestFailures++
	} else {
		deadline := time.Now().Add(30 * time.Second)
		for _, target := range targets {
			if ctx.Err() != nil || time.Now().After(deadline) {
				break
			}
			if !target.Available {
				inspectionErr = s.Database.Manifests().RecordUnavailableGallery(ctx, target.GalleryID, now)
			} else {
				_, inspectionErr = s.Database.Manifests().CheckGallery(ctx, target.GalleryID, now)
			}
			if inspectionErr != nil {
				manifestFailures++
			} else {
				manifestChecked++
			}
			if err := s.Database.Manifests().AdvanceInspectionCursor(ctx, target.GalleryID); err != nil {
				manifestFailures++
				break
			}
		}
	}
	if err := s.Database.Operations().CompleteScheduled(ctx, automaticScanTaskKey, owner, now); err != nil {
		return err
	}
	summary := map[string]any{"libraries_discovered": discovered, "automation_queued": automationQueued, "discovery_failures": discoveryFailures, "sources_scanned": scanned, "scan_failures": scanFailures, "manifests_checked": manifestChecked, "manifest_failures": manifestFailures}
	if discoveryFailures > 0 || scanFailures > 0 || manifestFailures > 0 {
		_ = s.Database.Operations().Audit(ctx, "SCHEDULED_TASK", "SCAN", "", "FAILURE", "AUTO_SCAN_PARTIAL_FAILURE", summary, now)
		return errors.New("automatic scan completed with failures")
	}
	_ = s.Database.Operations().Audit(ctx, "SCHEDULED_TASK", "SCAN", "", "SUCCESS", "", summary, now)
	return nil
}
