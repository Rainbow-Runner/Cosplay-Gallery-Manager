package productserver

import (
	"context"
	"time"

	"github.com/stashapp/stash/internal/persistence/productdb"
)

// Keep the existing bounded work cadence while a backlog exists. Empty
// backfills sleep until a relevant change or the hourly recovery deadline.
type backgroundSchedule struct {
	workAt, scheduledAt, recoveryAt, lastWork time.Time
}

func (p *backgroundSchedule) changed(now time.Time) {
	at := now.Add(time.Second)
	if minimum := p.lastWork.Add(time.Minute); minimum.After(at) {
		at = minimum
	}
	if p.workAt.IsZero() || at.Before(p.workAt) {
		p.workAt = at
	}
}

func earliestBackgroundTime(values ...time.Time) time.Time {
	var earliest time.Time
	for _, at := range values {
		if !at.IsZero() && (earliest.IsZero() || at.Before(earliest)) {
			earliest = at
		}
	}
	return earliest
}

func dueBackground(at, now time.Time) bool { return !at.IsZero() && !at.After(now) }

func (s *Server) runBackgroundScheduler(ctx context.Context) {
	now := time.Now()
	seenScheduleRevision := s.Database.BackgroundScheduleRevision()
	p := backgroundSchedule{workAt: now, scheduledAt: s.nextScheduledBackground(ctx, now), recoveryAt: now.Add(backgroundRecoveryInterval)}
	// An overdue periodic scan is not the explicit startup trigger. Preserve
	// the former first-minute check when scan-on-startup is disabled.
	if dueBackground(p.scheduledAt, now) {
		p.scheduledAt = now.Add(time.Minute)
	}
	for ctx.Err() == nil {
		// Snapshot before reading/processing; SQLite callbacks can arrive before
		// commit, so durable reads remain the authority and block on the writer.
		wake := s.Database.BackgroundEvents()
		now = time.Now()
		if revision := s.Database.BackgroundScheduleRevision(); revision != seenScheduleRevision {
			seenScheduleRevision = revision
			// Ordinary metadata events must not shorten a failed scheduled
			// task's retry delay. Only configuration/maintenance changes do.
			p.scheduledAt = earliestBackgroundTime(p.scheduledAt, now.Add(time.Second))
		}
		if dueBackground(p.recoveryAt, now) {
			p.workAt = now
			p.scheduledAt = now
			p.recoveryAt = now.Add(backgroundRecoveryInterval)
		}
		if dueBackground(p.scheduledAt, now) {
			s.runDailyBackup(ctx, now)
			s.runAutomaticScan(ctx, now)
			p.scheduledAt = s.nextScheduledBackground(ctx, time.Now())
			if dueBackground(p.scheduledAt, time.Now()) {
				p.scheduledAt = time.Now().Add(5 * time.Minute)
			}
		}
		if dueBackground(p.workAt, now) {
			pending := s.runLibraryPathChanges(ctx, now)
			pending = s.runChangedSourceScans(ctx, now) || pending
			pending = s.runVideoProbeBackfill(ctx, now) || pending
			pending = s.runCaptureDateBackfill(ctx, now) || pending
			pending = s.runMediaAddedBackfill(ctx, now) || pending
			p.lastWork = time.Now()
			p.workAt = time.Time{}
			if pending {
				p.workAt = p.lastWork.Add(time.Minute)
			}
		}
		deadline := earliestBackgroundTime(p.workAt, p.scheduledAt, p.recoveryAt)
		timer := time.NewTimer(backgroundWaitDuration(time.Now(), deadline))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-wake:
			timer.Stop()
			p.changed(time.Now())
		case <-timer.C:
		}
	}
}

func (s *Server) nextScheduledBackground(ctx context.Context, now time.Time) time.Time {
	runtime, err := s.Database.Settings().Find(ctx)
	if err != nil {
		return now.Add(5 * time.Minute)
	}
	if runtime.AutomaticSchedulesSuspended {
		return time.Time{}
	}
	maintenance, err := s.Database.Operations().Maintenance(ctx)
	if err != nil {
		return now.Add(5 * time.Minute)
	}
	if maintenance.Mode != productdb.MaintenanceNormal {
		return time.Time{}
	}
	var scan, backup time.Time
	if runtime.AutomaticScanEnabled {
		scan, err = s.Database.ScheduledNextWake(ctx, automaticScanTaskKey, time.Duration(runtime.AutomaticScanIntervalMinutes)*time.Minute, now)
		if err != nil {
			scan = now.Add(5 * time.Minute)
		}
	}
	if runtime.DailyBackupEnabled {
		if _, rootErr := s.Database.Operations().StorageRoots(ctx); rootErr == nil {
			backup, err = s.Database.ScheduledNextWake(ctx, dailyBackupTaskKey, 24*time.Hour, now)
			if err != nil {
				backup = now.Add(5 * time.Minute)
			}
		}
	}
	return earliestBackgroundTime(scan, backup)
}
