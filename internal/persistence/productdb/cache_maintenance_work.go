package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CacheCleanupNextWork is called at startup, after events/attempts, or the
// hourly fallback -- never by an unconditional minute poll.
func (s *DerivativeStore) CacheCleanupNextWork(ctx context.Context, now time.Time) (time.Time, error) {
	var obsolete bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM media_derivatives d WHERE `+obsoleteDerivativePredicate+`)`).Scan(&obsolete); err != nil {
		return time.Time{}, err
	}
	if obsolete {
		return now, nil
	}
	var due sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT MIN(not_before_utc) FROM cache_cleanup_outbox o
		WHERE NOT EXISTS(SELECT 1 FROM media_derivatives d WHERE d.cache_relative_path=o.cache_relative_path)
		AND NOT EXISTS(SELECT 1 FROM processing_jobs j WHERE j.item_uuid=o.item_uuid AND j.content_revision=o.content_revision AND j.variant=o.variant AND j.status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED'))`).Scan(&due)
	if err != nil || !due.Valid {
		return time.Time{}, err
	}
	if due.String == "" {
		return now, nil
	}
	return parseTime(due.String)
}

const cacheOrphanTaskKey = "CACHE_ORPHAN_REVIEW"

func (s *DerivativeStore) CacheOrphanDue(ctx context.Context, now time.Time) (time.Time, error) {
	var completed sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT last_completed_at_utc FROM scheduled_operations WHERE task_key=?`, cacheOrphanTaskKey).Scan(&completed)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !completed.Valid {
		return now, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	t, err := parseTime(completed.String)
	return t.Add(24 * time.Hour), err
}

func (s *DerivativeStore) CompleteCacheOrphanReview(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO scheduled_operations(task_key,last_completed_at_utc,updated_at_utc) VALUES(?,?,?)
		ON CONFLICT(task_key) DO UPDATE SET last_completed_at_utc=excluded.last_completed_at_utc,updated_at_utc=excluded.updated_at_utc`, cacheOrphanTaskKey, formatTime(now), formatTime(now))
	return err
}
