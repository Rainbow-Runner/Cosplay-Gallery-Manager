package productserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/processingworker"
)

const cacheEventDelay = 5 * time.Second
const cacheRunSpacing = 30 * time.Second
const cachePressureFallback = time.Hour
const cacheOrphanContinuation = 5 * time.Minute

type cacheSchedule struct {
	knownAt, pressureAt, orphanAt, lastRun time.Time
	retry                                  time.Duration
	seenRevision                           uint64
}

func (p *cacheSchedule) changed(now time.Time, revision uint64) {
	if revision <= p.seenRevision {
		return
	}
	p.retry = 0
	due := now.Add(cacheEventDelay)
	if earliest := p.lastRun.Add(cacheRunSpacing); earliest.After(due) {
		due = earliest
	}
	if p.knownAt.IsZero() || due.Before(p.knownAt) {
		p.knownAt = due
	}
	if p.pressureAt.IsZero() || due.Before(p.pressureAt) {
		p.pressureAt = due
	}
}
func (p *cacheSchedule) next(now time.Time) time.Duration {
	next := p.pressureAt
	for _, t := range []time.Time{p.knownAt, p.orphanAt} {
		if !t.IsZero() && (next.IsZero() || t.Before(next)) {
			next = t
		}
	}
	if next.IsZero() {
		return cachePressureFallback
	}
	if !next.After(now) {
		return 0
	}
	return next.Sub(now)
}
func (p *cacheSchedule) pending(now, due time.Time, progress bool) {
	if due.IsZero() {
		p.knownAt = time.Time{}
		p.retry = 0
		return
	}
	if due.After(now) {
		p.knownAt = due
		p.retry = 0
		return
	}
	if progress || p.retry == 0 {
		p.retry = time.Minute
	} else {
		p.retry = min(p.retry*2, time.Hour)
	}
	p.knownAt = now.Add(p.retry)
}

// A dedicated event loop prevents slow cache work from delaying scans/backups.
// No cache query, directory walk or cursor write is made on the minute ticker.
func (s *Server) runCacheMaintenanceLoop(ctx context.Context) {
	now := time.Now()
	orphanAt, err := s.Database.Derivatives().CacheOrphanDue(ctx, now)
	if err != nil {
		orphanAt = now.Add(time.Minute)
		slog.Error("CGM_CACHE_ORPHAN_SCHEDULE_FAILED")
	}
	knownAt, err := s.Database.Derivatives().CacheCleanupNextWork(ctx, now)
	if err != nil {
		knownAt = now.Add(time.Minute)
		slog.Error("CGM_CACHE_WORK_RECOVERY_FAILED")
	}
	plan := cacheSchedule{knownAt: knownAt, pressureAt: now, orphanAt: orphanAt}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.Database.CacheMaintenanceEvents():
			plan.changed(time.Now(), s.Database.CacheMaintenanceRevision())
		case <-timer.C:
			now = time.Now()
			revision := s.Database.CacheMaintenanceRevision()
			service := processingworker.CacheLifecycleService{Database: s.Database, Cache: mediaprocessing.CacheWriter{Root: s.Config.CachePath}}
			removed := 0
			knownDue := !plan.knownAt.IsZero() && !plan.knownAt.After(now)
			pressureDue := !plan.pressureAt.After(now)
			if knownDue || pressureDue {
				// Hourly checks also recover changes made outside this process.
				work, err := s.Database.Derivatives().CacheCleanupNextWork(ctx, now)
				if err == nil && !work.IsZero() && !work.After(now) {
					result, runErr := service.MaintainKnown(ctx, now)
					removed += result.Removed
					if runErr != nil {
						err = runErr
					} else {
						work, err = s.Database.Derivatives().CacheCleanupNextWork(ctx, now)
					}
				}
				if err != nil {
					plan.pending(now, now, false)
					if ctx.Err() == nil {
						slog.Error("CGM_CACHE_MAINTENANCE_FAILED")
					}
				} else {
					plan.pending(now, work, removed > 0)
				}
				plan.lastRun = now
			}
			if pressureDue {
				result, err := s.runCachePressureMaintenance(ctx, now)
				removed += result.Removed
				plan.pressureAt = now.Add(cachePressureFallback)
				if err != nil {
					plan.pressureAt = now.Add(15 * time.Minute)
					if ctx.Err() == nil {
						slog.Error("CGM_CACHE_PRESSURE_CHECK_FAILED")
					}
				}
				if err == nil && result.Removed > 0 && result.FreedBytes < result.PlannedBytes {
					plan.pressureAt = now.Add(time.Minute)
				}
			}
			if !plan.orphanAt.After(now) {
				result, complete, err := service.MaintainOrphans(ctx, now)
				removed += result.Removed
				plan.orphanAt = now.Add(cacheOrphanContinuation)
				if err == nil && complete {
					err = s.Database.Derivatives().CompleteCacheOrphanReview(ctx, now)
					if err == nil {
						plan.orphanAt = now.Add(24 * time.Hour)
					}
				}
				if err != nil && ctx.Err() == nil {
					slog.Error("CGM_CACHE_ORPHAN_REVIEW_FAILED")
				}
			}
			plan.seenRevision = revision
			plan.changed(time.Now(), s.Database.CacheMaintenanceRevision())
			if removed > 0 {
				slog.Info("CGM_CACHE_MAINTENANCE_COMPLETED", "removed", removed)
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(plan.next(time.Now()))
	}
}
