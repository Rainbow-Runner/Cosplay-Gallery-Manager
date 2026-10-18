package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Zero means no durable deadline. A periodic recovery check still covers
// external SQLite writers, which do not share connection-local notifications.
func (db *Database) ProcessingNextWake(ctx context.Context, now time.Time) (time.Time, error) {
	deadline, err := nextWorkTime(ctx, db.DB, `SELECT at FROM (
		SELECT not_before_utc AS at FROM processing_jobs WHERE status IN ('PENDING','RETRY_WAIT') AND `+processingJobEnabledSQL+`
		UNION ALL SELECT lease_expires_at_utc FROM processing_jobs WHERE status='RUNNING'
		UNION ALL SELECT next_check_at_utc FROM cover_similarity_intents WHERE status='PENDING'
		UNION ALL SELECT strftime('%Y-%m-%dT%H:%M:%fZ',attempted_at_utc,'+1 hour') FROM cover_signature_backfill_attempts
		WHERE attempted_at_utc>?) ORDER BY julianday(at) LIMIT 1`, formatTime(now.Add(-time.Hour)))
	if err != nil {
		return time.Time{}, err
	}
	_, err = db.CoverSimilarity().NextUnhashed(ctx)
	if err == nil {
		return now.Add(500 * time.Millisecond), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, err
	}
	return deadline, nil
}

func (db *Database) AutomationNextWake(ctx context.Context) (time.Time, error) {
	return nextWorkTime(ctx, db.DB, `SELECT at FROM (
		SELECT started_at_utc AS at FROM library_automation_runs WHERE status='QUEUED'
		UNION ALL SELECT lease_expires_at_utc FROM library_automation_runs WHERE status='RUNNING') ORDER BY julianday(at) LIMIT 1`)
}

func nextWorkTime(ctx context.Context, db *sql.DB, query string, args ...any) (time.Time, error) {
	var raw sql.NullString
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	} else if err != nil {
		return time.Time{}, err
	}
	if !raw.Valid {
		return time.Time{}, nil
	}
	return parseTime(raw.String)
}

func (db *Database) ScheduledNextWake(ctx context.Context, key string, interval time.Duration, now time.Time) (time.Time, error) {
	var completed, lease sql.NullString
	err := db.QueryRowContext(ctx, `SELECT last_completed_at_utc,lease_expires_at_utc FROM scheduled_operations WHERE task_key=?`, key).Scan(&completed, &lease)
	if errors.Is(err, sql.ErrNoRows) {
		return now, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	next := now
	if completed.Valid {
		t, err := parseTime(completed.String)
		if err != nil {
			return time.Time{}, err
		}
		next = t.Add(interval)
	}
	if lease.Valid {
		t, err := parseTime(lease.String)
		if err != nil {
			return time.Time{}, err
		}
		if t.After(next) {
			next = t
		}
	}
	return next, nil
}
