package productserver

import (
	"context"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

const dailyBackupTaskKey = "DAILY_BACKUP"
const automaticScanTaskKey = "AUTOMATIC_SCAN"

func (s *Server) runSchedulerLoop(ctx context.Context) {
	s.runDailyBackup(ctx, time.Now())
	s.runAutomaticScan(ctx, time.Now())
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.runDailyBackup(ctx, now)
			s.runAutomaticScan(ctx, now)
		}
	}
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
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	runtime, err := s.Database.Settings().Find(ctx)
	if err != nil {
		return err
	}
	if !runtime.AutomaticScanEnabled || runtime.AutomaticSchedulesSuspended {
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
	if err := s.Database.Operations().ClaimScheduled(ctx, automaticScanTaskKey, owner, now.Add(-24*time.Hour), 12*time.Hour, now); err != nil {
		return err
	}
	libraries, err := s.Database.Libraries().List(ctx)
	if err != nil {
		_ = s.Database.Operations().FailScheduled(context.Background(), automaticScanTaskKey, owner, "AUTO_SCAN_LIST_FAILED", now)
		return err
	}
	discovered, discoveryFailures := 0, 0
	for _, mediaLibrary := range libraries {
		if !mediaLibrary.Enabled {
			continue
		}
		if _, err := s.Database.CandidateDiscovery().DiscoverFilesystem(ctx, mediaLibrary.ID, now); err != nil {
			discoveryFailures++
		} else {
			discovered++
		}
	}
	rows, err := s.Database.QueryContext(ctx, `SELECT source.id FROM gallery_sources source
		JOIN media_libraries library ON library.id=source.library_id
		WHERE library.enabled=1 ORDER BY source.id`)
	if err != nil {
		_ = s.Database.Operations().FailScheduled(context.Background(), automaticScanTaskKey, owner, "AUTO_SCAN_SOURCE_LIST_FAILED", now)
		return err
	}
	var sourceIDs []int64
	for rows.Next() {
		var sourceID int64
		if err := rows.Scan(&sourceID); err != nil {
			rows.Close()
			_ = s.Database.Operations().FailScheduled(context.Background(), automaticScanTaskKey, owner, "AUTO_SCAN_SOURCE_LIST_FAILED", now)
			return err
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
	if err := s.Database.Operations().CompleteScheduled(ctx, automaticScanTaskKey, owner, now); err != nil {
		return err
	}
	summary := map[string]any{"libraries_discovered": discovered, "discovery_failures": discoveryFailures, "sources_scanned": scanned, "scan_failures": scanFailures}
	if discoveryFailures > 0 || scanFailures > 0 {
		_ = s.Database.Operations().Audit(ctx, "SCHEDULED_TASK", "SCAN", "", "FAILURE", "AUTO_SCAN_PARTIAL_FAILURE", summary, now)
		return errors.New("automatic scan completed with failures")
	}
	_ = s.Database.Operations().Audit(ctx, "SCHEDULED_TASK", "SCAN", "", "SUCCESS", "", summary, now)
	return nil
}
