package processingworker

import (
	"context"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

type CacheMaintenanceResult struct {
	PlannedBytes       int64
	FreedBytes         int64
	Removed            int
	PauseNewProcessing bool
}

// MaintainEnhancedCache deletes only database-confirmed ENHANCED derivatives.
// BASE resources and all user media sources are outside this operation.
func MaintainEnhancedCache(ctx context.Context, db *productdb.Database, cache mediaprocessing.CacheWriter, pressure mediaprocessing.CachePressure, limit int, now time.Time) (CacheMaintenanceResult, error) {
	plan := mediaprocessing.PlanCacheCleanup(pressure)
	result := CacheMaintenanceResult{PlannedBytes: plan.BytesToFree, PauseNewProcessing: plan.PauseNewProcessing}
	if plan.BytesToFree <= 0 {
		return result, nil
	}
	candidates, err := db.Derivatives().EnhancedLRUCandidates(ctx, plan.BytesToFree, limit)
	if err != nil {
		return result, err
	}
	for _, candidate := range candidates {
		service := CacheLifecycleService{Database: db, Cache: cache}
		entry, err := service.candidate(productdb.CacheCleanupEntry{
			ID: candidate.ID, Path: candidate.CacheRelativePath, ItemUUID: candidate.ItemUUID,
			Variant: candidate.Variant, ProfileHash: candidate.ProfileHash,
			ContentRevision: candidate.ContentRevision, ByteSize: candidate.ByteSize, Reason: "LRU",
		})
		if err != nil {
			return result, err
		}
		var cleaned CacheCleanupResult
		if err := service.remove(ctx, entry, true, now, &cleaned); err != nil {
			return result, err
		}
		if cleaned.Failed > 0 {
			return result, errors.New("CACHE_FILE_DELETE_FAILED")
		}
		result.Removed += cleaned.Removed
		result.FreedBytes += cleaned.FreedBytes
	}
	return result, nil
}
