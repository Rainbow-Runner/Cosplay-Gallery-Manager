package productserver

import (
	"context"
	"errors"
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
	s.runDailyBackup(ctx, time.Now())
	s.runAutomaticStartupScan(ctx, time.Now())
	s.runCacheMaintenance(ctx, time.Now())
	s.runVideoProbeBackfill(ctx, time.Now())
	s.runCaptureDateBackfill(ctx, time.Now())
	hourly := time.NewTicker(time.Hour)
	scanTicker := time.NewTicker(time.Minute)
	cacheTicker := time.NewTicker(time.Minute)
	defer hourly.Stop()
	defer scanTicker.Stop()
	defer cacheTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-hourly.C:
			s.runDailyBackup(ctx, now)
		case now := <-scanTicker.C:
			s.runAutomaticScan(ctx, now)
		case now := <-cacheTicker.C:
			s.runCacheMaintenance(ctx, now)
			s.runVideoProbeBackfill(ctx, now)
			s.runCaptureDateBackfill(ctx, now)
		}
	}
}

func (s *Server) runCaptureDateBackfill(ctx context.Context, now time.Time) {
	count, err := s.Database.CaptureDates().EnqueueBackfill(ctx, 25, s.VideoTools.FFprobe.Available, now)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("CGM_CAPTURE_DATE_BACKFILL_FAILED", "error", err)
		}
		return
	}
	if count > 0 {
		slog.Info("CGM_CAPTURE_DATE_BACKFILL_QUEUED", "item_count", count)
	}
}

func (s *Server) runAutomaticStartupScan(ctx context.Context, now time.Time) {
	if err := s.RunAutomaticStartupScanOnce(ctx, now); err != nil &&
		!errors.Is(err, productdb.ErrScheduledOperationNotDue) &&
		ctx.Err() == nil {
		// The audit stores only technical counts and error codes.
	}
}

func (s *Server) runVideoProbeBackfill(ctx context.Context, now time.Time) {
	if !s.VideoTools.FFprobe.Available {
		return
	}
	count, err := s.Database.VideoMetadata().EnqueueBackfill(ctx, mediaprocessing.VideoProbeProfileHash(s.VideoTools.FFprobe.Version), 25, now)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("CGM_VIDEO_PROBE_BACKFILL_FAILED")
		}
		return
	}
	if count > 0 {
		slog.Info("CGM_VIDEO_PROBE_BACKFILL_QUEUED", "item_count", count)
	}
}

func (s *Server) runCacheMaintenance(ctx context.Context, now time.Time) {
	result, err := s.RunCacheMaintenanceOnce(ctx, now)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("CGM_CACHE_MAINTENANCE_FAILED")
		}
		return
	}
	if result.Removed > 0 {
		slog.Info("CGM_CACHE_MAINTENANCE_COMPLETED", "removed", result.Removed, "freed_bytes", result.FreedBytes)
	}
}

// RunCacheMaintenanceOnce enforces the configured reclaimable cache limit.
// It can delete only database-confirmed ENHANCED derivatives; user media and
// permanent CARD_480/static-poster resources are outside its authority.
func (s *Server) RunCacheMaintenanceOnce(ctx context.Context, now time.Time) (processingworker.CacheMaintenanceResult, error) {
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
	return processingworker.MaintainEnhancedCache(ctx, s.Database, mediaprocessing.CacheWriter{Root: s.Config.CachePath}, mediaprocessing.CachePressure{
		EnhancedBytes: enhanced, AvailableBytes: available, TotalBytes: total,
		MaximumEnhancedBytes: settings.EnhancedCacheMaximumBytes, MinimumFreeBytes: settings.MinimumFreeBytes,
		MinimumFreePercent: settings.MinimumFreePercent,
	}, 10000, now)
}

func (s *Server) runAutomaticScan(ctx context.Context, now time.Time) {
	if err := s.RunAutomaticScanOnce(ctx, now); err != nil &&
		!errors.Is(err, productdb.ErrScheduledOperationNotDue) &&
		ctx.Err() == nil {
		// The audit stores only technical counts and error codes.
	}
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
	if err := s.Database.Backups().ApplyDailyRetention(ctx, roots.BackupRoot, settings.DailyBackupRetention); err != nil {
		_ = s.Database.Operations().Audit(context.Background(), "SCHEDULED_TASK", "BACKUP", record.ID, "FAILURE", "DAILY_RETENTION_FAILED", map[string]any{"task": dailyBackupTaskKey}, now)
		return err
	}
	_ = s.Database.Operations().Audit(ctx, "SCHEDULED_TASK", "BACKUP", record.ID, "SUCCESS", "", map[string]any{"task": dailyBackupTaskKey}, now)
	return nil
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
	rows, err := s.Database.QueryContext(ctx, `SELECT source.id,source.library_id,gallery.state FROM gallery_sources source
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
		var galleryState string
		if err := rows.Scan(&sourceID, &libraryID, &galleryState); err != nil {
			rows.Close()
			_ = s.Database.Operations().FailScheduled(context.Background(), automaticScanTaskKey, owner, "AUTO_SCAN_SOURCE_LIST_FAILED", now)
			return err
		}
		if automatedLibraries[libraryID] && galleryState == "DRAFT" {
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
