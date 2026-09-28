package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrCacheReviewStale = errors.New("CACHE_REVIEW_STALE")

type CacheCleanupEntry struct {
	ID                                                          int64
	Path, ItemUUID, Variant, ProfileHash, Reason, LastErrorCode string
	ContentRevision, ByteSize                                   int64
}

type CacheLifecycleSummary struct {
	ReferencedBytes int64 `json:"referenced_bytes"`
	ObsoleteBytes   int64 `json:"obsolete_bytes"`
	PendingBytes    int64 `json:"pending_bytes"`
	PendingFiles    int64 `json:"pending_files"`
	FailedFiles     int64 `json:"failed_files"`
}

// Missing/excluded/unreadable Items are deliberately not cleanup predicates.
// Retire old artifacts only when the replacement primary resource is ready.
const obsoleteDerivativePredicate = `(d.is_current=0 OR d.state='HARD_INVALID') AND EXISTS (
	SELECT 1 FROM media_derivatives replacement JOIN gallery_items i ON i.item_uuid=replacement.item_uuid
	WHERE replacement.item_uuid=d.item_uuid AND replacement.content_revision=i.content_revision
	AND replacement.cache_tier='BASE' AND replacement.is_current=1 AND replacement.state='READY'
	AND ((i.media_kind='STATIC_IMAGE' AND replacement.variant='CARD_480') OR
	(i.media_kind IN ('ANIMATED_IMAGE','VIDEO') AND replacement.variant='STATIC_POSTER')))
	AND NOT EXISTS (SELECT 1 FROM processing_jobs job WHERE job.item_uuid=d.item_uuid
	AND job.content_revision=d.content_revision AND job.variant=d.variant
	AND job.status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED'))`

func (s *DerivativeStore) LifecycleSummary(ctx context.Context) (CacheLifecycleSummary, error) {
	var result CacheLifecycleSummary
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(byte_size),0),
		COALESCE(SUM(CASE WHEN is_current=0 OR state='HARD_INVALID' THEN byte_size ELSE 0 END),0)
		FROM media_derivatives`).Scan(&result.ReferencedBytes, &result.ObsoleteBytes)
	if err != nil {
		return result, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(byte_size),0),COUNT(*),
		COALESCE(SUM(CASE WHEN last_error_code<>'' THEN 1 ELSE 0 END),0) FROM cache_cleanup_outbox`).Scan(
		&result.PendingBytes, &result.PendingFiles, &result.FailedFiles)
	return result, err
}

func (s *DerivativeStore) CleanupEntries(ctx context.Context, limit int) ([]CacheCleanupEntry, error) {
	return s.cleanupEntries(ctx, limit, "")
}

func (s *DerivativeStore) CleanupEntriesDue(ctx context.Context, limit int, now time.Time) ([]CacheCleanupEntry, error) {
	return s.cleanupEntries(ctx, limit, formatTime(now))
}

func (s *DerivativeStore) cleanupEntries(ctx context.Context, limit int, due string) ([]CacheCleanupEntry, error) {
	if limit < 1 || limit > 10000 {
		return nil, errors.New("invalid cleanup limit")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT d.id,d.cache_relative_path,d.item_uuid,d.variant,d.content_revision,d.profile_hash,d.byte_size,'OBSOLETE',''
		FROM media_derivatives d WHERE `+obsoleteDerivativePredicate+` ORDER BY d.id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	result, err := readCleanupEntries(rows)
	if err != nil {
		return nil, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT 0,cache_relative_path,item_uuid,variant,content_revision,profile_hash,byte_size,'PENDING_DELETE',last_error_code
		FROM cache_cleanup_outbox WHERE (?='' OR not_before_utc<=?)
		AND NOT EXISTS(SELECT 1 FROM media_derivatives d WHERE d.cache_relative_path=cache_cleanup_outbox.cache_relative_path)
		AND NOT EXISTS(SELECT 1 FROM processing_jobs j WHERE j.item_uuid=cache_cleanup_outbox.item_uuid
		AND j.content_revision=cache_cleanup_outbox.content_revision AND j.variant=cache_cleanup_outbox.variant
		AND j.status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED'))
		ORDER BY created_at_utc,cache_relative_path LIMIT ?`, due, due, limit)
	if err != nil {
		return nil, err
	}
	pending, err := readCleanupEntries(rows)
	return append(result, pending...), err
}

func readCleanupEntries(rows *sql.Rows) ([]CacheCleanupEntry, error) {
	defer rows.Close()
	var result []CacheCleanupEntry
	for rows.Next() {
		var entry CacheCleanupEntry
		if err := rows.Scan(&entry.ID, &entry.Path, &entry.ItemUUID, &entry.Variant, &entry.ContentRevision, &entry.ProfileHash, &entry.ByteSize, &entry.Reason, &entry.LastErrorCode); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// CacheFileProtected also protects files between atomic rename and DB Publish.
// Include pending/retry jobs, not only RUNNING: a request may requeue a variant.
func cacheFileProtected(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, entry CacheCleanupEntry) (bool, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM media_derivatives WHERE cache_relative_path=?) +
		(SELECT COUNT(*) FROM processing_jobs WHERE item_uuid=? AND content_revision=? AND variant=?
		 AND status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED'))`, entry.Path, entry.ItemUUID, entry.ContentRevision, entry.Variant).Scan(&count)
	return count > 0, err
}

func (s *DerivativeStore) OrphanEligible(ctx context.Context, entry CacheCleanupEntry) (bool, error) {
	var known int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_uuid_registry WHERE uuid=? AND entity_kind='GALLERY_ITEM'`, entry.ItemUUID).Scan(&known); err != nil {
		return false, err
	}
	protected, err := cacheFileProtected(ctx, s.db, entry)
	if err != nil {
		return false, err
	}
	var pending int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cache_cleanup_outbox WHERE cache_relative_path=?`, entry.Path).Scan(&pending)
	return known == 1 && !protected && pending == 0, err
}

// Retire the served reference durably BEFORE unlinking. A failed commit after
// unlink must never restore a derivative row pointing at a missing file.
func (s *DerivativeStore) retireCacheEntry(ctx context.Context, entry CacheCleanupEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	condition := obsoleteDerivativePredicate
	if entry.Reason == "LRU" {
		condition = `d.cache_tier='ENHANCED'`
	}
	var match int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_derivatives d WHERE d.id=? AND d.cache_relative_path=? AND `+condition, entry.ID, entry.Path).Scan(&match); err != nil {
		return err
	}
	if match != 1 {
		return ErrCacheReviewStale
	}
	// A newly requeued task must not lose its publication target.
	var busy int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE item_uuid=? AND content_revision=? AND variant=? AND status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED')`, entry.ItemUUID, entry.ContentRevision, entry.Variant).Scan(&busy); err != nil {
		return err
	}
	if busy > 0 {
		return ErrCacheReviewStale
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_derivatives WHERE id=?`, entry.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// CleanupCacheEntry holds an immediate writer transaction across the final
// reference/task recheck and a bounded single-file deletion. Retired references
// have already been committed; an interruption leaves only a retryable outbox.
func (s *DerivativeStore) CleanupCacheEntry(ctx context.Context, entry CacheCleanupEntry, automatic bool, now time.Time, remove func() error) (removed, failed bool, err error) {
	if entry.Reason == "OBSOLETE" || entry.Reason == "LRU" {
		if err := s.retireCacheEntry(ctx, entry); err != nil {
			return false, false, err
		}
		entry.Reason = "PENDING_DELETE"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, false, err
	}
	defer tx.Rollback()
	if entry.Reason == "ORPHAN" {
		var known int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_uuid_registry WHERE uuid=? AND entity_kind='GALLERY_ITEM'`, entry.ItemUUID).Scan(&known); err != nil {
			return false, false, err
		}
		if known != 1 {
			return false, false, ErrCacheReviewStale
		}
		var pending int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM cache_cleanup_outbox WHERE cache_relative_path=?`, entry.Path).Scan(&pending); err != nil {
			return false, false, err
		}
		if pending != 0 {
			return false, false, ErrCacheReviewStale
		}
	} else if entry.Reason == "PENDING_DELETE" {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM cache_cleanup_outbox WHERE cache_relative_path=? AND (?=0 OR not_before_utc<=?)`, entry.Path, boolInt(automatic), formatTime(now)).Scan(&count); err != nil {
			return false, false, err
		}
		if count == 0 {
			return false, false, ErrCacheReviewStale
		}
	} else {
		return false, false, errors.New("invalid cleanup reason")
	}
	protected, err := cacheFileProtected(ctx, tx, entry)
	if err != nil {
		return false, false, err
	}
	if protected {
		return false, false, ErrCacheReviewStale
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO cache_cleanup_outbox(cache_relative_path,item_uuid,variant,content_revision,profile_hash,byte_size,created_at_utc) VALUES(?,?,?,?,?,?,?)`, entry.Path, entry.ItemUUID, entry.Variant, entry.ContentRevision, entry.ProfileHash, entry.ByteSize, formatTime(now)); err != nil {
		return false, false, err
	}
	if err := remove(); err != nil {
		if _, saveErr := tx.ExecContext(ctx, `UPDATE cache_cleanup_outbox SET attempts=attempts+1,last_error_code='CACHE_FILE_DELETE_FAILED',not_before_utc=? WHERE cache_relative_path=?`, formatTime(now.Add(15*time.Minute)), entry.Path); saveErr != nil {
			return false, false, saveErr
		}
		return false, true, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cache_cleanup_outbox WHERE cache_relative_path=?`, entry.Path); err != nil {
		return false, false, err
	}
	return true, false, tx.Commit()
}

func (s *DerivativeStore) CacheScanCursor(ctx context.Context) (shard int, cursor string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT shard,cursor FROM cache_cleanup_scan_state WHERE id=1`).Scan(&shard, &cursor)
	return
}
func (s *DerivativeStore) SaveCacheScanCursor(ctx context.Context, shard int, cursor string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE cache_cleanup_scan_state SET shard=?,cursor=? WHERE id=1`, shard, cursor)
	return err
}

func (s *DerivativeStore) MarkCacheCleanupUnsafe(ctx context.Context, path string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE cache_cleanup_outbox SET attempts=attempts+1,last_error_code='CACHE_FILE_UNSAFE_OR_UNREADABLE',not_before_utc=? WHERE cache_relative_path=?`, formatTime(now.Add(15*time.Minute)), path)
	return err
}
