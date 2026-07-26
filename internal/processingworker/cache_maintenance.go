package processingworker

import (
	"context"
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
		if err := db.Derivatives().ForgetGenerated(ctx, candidate.ID); err != nil {
			return result, err
		}
		key := productdb.ItemDerivativeJobKey(candidate.ItemUUID, candidate.Variant, candidate.ContentRevision, candidate.ProfileHash)
		if _, err := db.ProcessingJobs().Requeue(ctx, key, 50, now); err != nil {
			return result, err
		}
		// The database stops serving the derivative before bytes are removed.
		// A deletion failure therefore leaves only an unreachable generated
		// orphan, never a database pointer to a missing cache file.
		if err := cache.RemoveEnhanced(candidate.CacheRelativePath); err != nil {
			return result, err
		}
		result.Removed++
		result.FreedBytes += candidate.ByteSize
	}
	return result, nil
}
