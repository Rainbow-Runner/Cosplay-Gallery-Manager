package processingworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

const CacheOrphanGrace = 24 * time.Hour
const cacheCleanupBatch = 100

type CacheCleanupCandidate struct {
	ID            string `json:"id"`
	Reason        string `json:"reason"`
	ByteSize      int64  `json:"byte_size"`
	LastErrorCode string `json:"last_error_code,omitempty"`
	entry         productdb.CacheCleanupEntry
	info          fs.FileInfo
}
type CacheCleanupReview struct {
	productdb.CacheLifecycleSummary
	Candidates       []CacheCleanupCandidate `json:"candidates"`
	ReclaimableBytes int64                   `json:"reclaimable_bytes"`
	OrphanBytes      int64                   `json:"orphan_bytes"`
	IgnoredFiles     int                     `json:"ignored_files"`
	Partial          bool                    `json:"partial"`
	GraceHours       int                     `json:"grace_hours"`
}
type CacheCleanupResult struct {
	Removed    int   `json:"removed"`
	FreedBytes int64 `json:"freed_bytes"`
	Failed     int   `json:"failed"`
	Skipped    int   `json:"skipped"`
}
type CacheLifecycleService struct {
	Database *productdb.Database
	Cache    mediaprocessing.CacheWriter
}

func (s CacheLifecycleService) candidate(entry productdb.CacheCleanupEntry) (CacheCleanupCandidate, error) {
	identity, ok := mediaprocessing.ParseGeneratedCachePath(entry.Path)
	if !ok || identity.ItemUUID != entry.ItemUUID || identity.Variant != entry.Variant || identity.ContentRevision != entry.ContentRevision {
		return CacheCleanupCandidate{}, errors.New("invalid cache cleanup identity")
	}
	// A DB row cannot authorize deletion of another processing profile.
	if entry.ProfileHash != "" {
		path, err := s.Cache.RelativePath(entry.ItemUUID, entry.ContentRevision, entry.Variant, entry.ProfileHash, "jpg")
		parsed, valid := mediaprocessing.ParseGeneratedCachePath(path)
		if err != nil || !valid || parsed.ProfileKey != identity.ProfileKey {
			return CacheCleanupCandidate{}, errors.New("invalid cache cleanup profile")
		}
	}
	info, err := s.Cache.InspectForCleanup(entry.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return CacheCleanupCandidate{}, err
	}
	c := CacheCleanupCandidate{Reason: entry.Reason, LastErrorCode: entry.LastErrorCode, entry: entry, info: info}
	stamp := "missing"
	if info != nil {
		c.ByteSize = info.Size()
		stat := info.Sys().(*syscall.Stat_t)
		stamp = fmt.Sprintf("%d:%d:%d:%d", stat.Dev, stat.Ino, info.Size(), info.ModTime().UnixNano())
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%s", entry.Path, entry.Reason, entry.ID, stamp)))
	c.ID = hex.EncodeToString(sum[:])
	return c, nil
}

// Review does not stage deletion, mutate scan cursors or access source media.
// Orphan bytes are explicitly a bounded observation, not an exact disk total.
func (s CacheLifecycleService) Review(ctx context.Context, now time.Time) (CacheCleanupReview, error) {
	summary, err := s.Database.Derivatives().LifecycleSummary(ctx)
	r := CacheCleanupReview{CacheLifecycleSummary: summary, Candidates: []CacheCleanupCandidate{}, GraceHours: 24}
	if err != nil {
		return r, err
	}
	entries, err := s.Database.Derivatives().CleanupEntries(ctx, cacheCleanupBatch)
	if err != nil {
		return r, err
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		c, e := s.candidate(entry)
		if e != nil {
			r.IgnoredFiles++
			continue
		}
		seen[entry.Path] = true
		r.Candidates = append(r.Candidates, c)
		r.ReclaimableBytes += c.ByteSize
	}
	deadline := time.Now().Add(5 * time.Second)
	for shard := 0; shard < 256; shard++ {
		_, complete, ignored, e := s.Cache.WalkCleanupShard(shard, "", 10000, deadline, func(path string, info fs.FileInfo) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if len(r.Candidates) >= 200 {
				return mediaprocessing.ErrCacheWalkBound
			}
			if seen[path] || info.ModTime().After(now.Add(-CacheOrphanGrace)) {
				return nil
			}
			identity, _ := mediaprocessing.ParseGeneratedCachePath(path)
			entry := productdb.CacheCleanupEntry{Path: path, ItemUUID: identity.ItemUUID, Variant: identity.Variant, ContentRevision: identity.ContentRevision, ByteSize: info.Size(), Reason: "ORPHAN"}
			eligible, e := s.Database.Derivatives().OrphanEligible(ctx, entry)
			if e != nil {
				return e
			}
			if !eligible {
				return nil
			}
			c, e := s.candidate(entry)
			if e != nil {
				r.IgnoredFiles++
				return nil
			}
			seen[path] = true
			r.Candidates = append(r.Candidates, c)
			r.OrphanBytes += c.ByteSize
			r.ReclaimableBytes += c.ByteSize
			return nil
		})
		r.IgnoredFiles += ignored
		if e != nil {
			return r, e
		}
		if !complete {
			r.Partial = true
			break
		}
	}
	if len(entries) >= cacheCleanupBatch {
		r.Partial = true
	}
	return r, nil
}

func (s CacheLifecycleService) remove(ctx context.Context, c CacheCleanupCandidate, automatic bool, now time.Time, result *CacheCleanupResult) error {
	removed, failed, err := s.Database.Derivatives().CleanupCacheEntry(ctx, c.entry, automatic, now, func() error {
		return s.Cache.RemoveReviewedGenerated(c.entry.Path, c.info)
	})
	if errors.Is(err, productdb.ErrCacheReviewStale) {
		result.Skipped++
		return nil
	}
	if err != nil {
		return err
	}
	if failed {
		result.Failed++
	}
	if removed {
		result.Removed++
		result.FreedBytes += c.ByteSize
	}
	return nil
}

func (s CacheLifecycleService) CleanupSelected(ctx context.Context, ids []string, now time.Time) (CacheCleanupResult, error) {
	result := CacheCleanupResult{}
	if len(ids) == 0 || len(ids) > 100 {
		return result, productdb.ErrCacheReviewStale
	}
	review, err := s.Review(ctx, now)
	if err != nil {
		return result, err
	}
	byID := map[string]CacheCleanupCandidate{}
	for _, c := range review.Candidates {
		byID[c.ID] = c
	}
	selected := map[string]bool{}
	for _, id := range ids {
		if _, ok := byID[id]; !ok || selected[id] {
			return result, productdb.ErrCacheReviewStale
		}
		selected[id] = true
	}
	for _, id := range ids {
		if err := s.remove(ctx, byID[id], false, now, &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

// Maintain advances a persistent, bounded shard cursor. Pending deletions and
// obsolete derivatives are handled regardless of quota/disk pressure.
func (s CacheLifecycleService) Maintain(ctx context.Context, now time.Time) (CacheCleanupResult, error) {
	result := CacheCleanupResult{}
	entries, err := s.Database.Derivatives().CleanupEntriesDue(ctx, cacheCleanupBatch, now)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		c, e := s.candidate(entry)
		if e != nil {
			result.Skipped++
			if entry.Reason == "PENDING_DELETE" {
				if err := s.Database.Derivatives().MarkCacheCleanupUnsafe(ctx, entry.Path, now); err != nil {
					return result, err
				}
				result.Failed++
			}
			continue
		}
		if e := s.remove(ctx, c, true, now, &result); e != nil {
			return result, e
		}
	}
	shard, cursor, err := s.Database.Derivatives().CacheScanCursor(ctx)
	if err != nil {
		return result, err
	}
	collected := []CacheCleanupCandidate{}
	last, complete, _, err := s.Cache.WalkCleanupShard(shard, cursor, 10000, time.Now().Add(2*time.Second), func(path string, info fs.FileInfo) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if len(collected) >= cacheCleanupBatch {
			return mediaprocessing.ErrCacheWalkBound
		}
		if info.ModTime().After(now.Add(-CacheOrphanGrace)) {
			return nil
		}
		identity, _ := mediaprocessing.ParseGeneratedCachePath(path)
		entry := productdb.CacheCleanupEntry{Path: path, ItemUUID: identity.ItemUUID, Variant: identity.Variant, ContentRevision: identity.ContentRevision, ByteSize: info.Size(), Reason: "ORPHAN"}
		eligible, e := s.Database.Derivatives().OrphanEligible(ctx, entry)
		if e != nil {
			return e
		}
		if eligible {
			c, e := s.candidate(entry)
			if e == nil {
				collected = append(collected, c)
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	for _, c := range collected {
		if err := s.remove(ctx, c, true, now, &result); err != nil {
			return result, err
		}
	}
	if complete {
		shard = (shard + 1) % 256
		last = ""
	}
	return result, s.Database.Derivatives().SaveCacheScanCursor(ctx, shard, last)
}
