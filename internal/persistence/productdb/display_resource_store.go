package productdb

import (
	"context"
	"database/sql"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

type DisplayImageStatus struct{ Total, Ready, Pending, Failed int }

type displayBaseResource struct {
	Resource      *browse.ResourceIdentity
	Width, Height int
}

func (s *ProcessingJobStore) DisplayImageStatus(ctx context.Context, setID string) (DisplayImageStatus, error) {
	var result DisplayImageStatus
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(EXISTS(SELECT 1 FROM media_derivatives d WHERE d.item_uuid=i.item_uuid AND d.content_revision=i.content_revision AND d.variant=? AND d.profile_hash=? AND d.is_current=1 AND d.state='READY')),0),
		COALESCE(SUM(EXISTS(SELECT 1 FROM processing_jobs j WHERE j.item_uuid=i.item_uuid AND j.content_revision=i.content_revision AND j.variant=? AND j.profile_hash=? AND j.status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED'))),0),
		COALESCE(SUM(EXISTS(SELECT 1 FROM processing_jobs j WHERE j.item_uuid=i.item_uuid AND j.content_revision=i.content_revision AND j.variant=? AND j.profile_hash=? AND j.status='FAILED')),0)
		FROM gallery_items i JOIN galleries g ON g.id=i.gallery_id WHERE g.set_id=? AND i.excluded=0 AND i.availability_state='AVAILABLE' AND i.media_kind='STATIC_IMAGE' AND i.content_format='IMAGE'`,
		mediaprocessing.VariantCardDisplay960, mediaprocessing.DisplayProfileHash(), mediaprocessing.VariantCardDisplay960, mediaprocessing.DisplayProfileHash(), mediaprocessing.VariantCardDisplay960, mediaprocessing.DisplayProfileHash(), setID).Scan(&result.Total, &result.Ready, &result.Pending, &result.Failed)
	return result, err
}

// displayResources reads only current-contract resources, never old-revision
// or weakly invalidated assets. Cover and member cards share these identities.
func (s *BrowseStore) displayResources(ctx context.Context, galleryID int64, itemUUID string) (map[string]*displayBaseResource, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.item_uuid,d.content_revision,d.profile_hash,d.variant,d.mime_type,d.width,d.height
		FROM media_derivatives d JOIN gallery_items i ON i.item_uuid=d.item_uuid
		WHERE i.gallery_id=? AND i.excluded=0 AND i.availability_state='AVAILABLE' AND i.media_kind='STATIC_IMAGE' AND i.content_format='IMAGE'
		AND d.variant=? AND d.profile_hash=? AND d.content_revision=i.content_revision AND d.is_current=1 AND d.state='READY'
		AND (?='' OR i.item_uuid=?)`, galleryID, mediaprocessing.VariantCardDisplay960, mediaprocessing.DisplayProfileHash(), itemUUID, itemUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]*displayBaseResource)
	for rows.Next() {
		var r browse.ResourceIdentity
		var width, height int
		if err := rows.Scan(&r.ItemUUID, &r.ContentRevision, &r.ProfileHash, &r.Variant, &r.MIMEType, &width, &height); err != nil {
			return nil, err
		}
		result[r.ItemUUID] = &displayBaseResource{Resource: &r, Width: width, Height: height}
	}
	return result, rows.Err()
}

// PrepareDisplayImages is explicit, idempotent backfill. It never runs at startup.
// Failed jobs can be retried through the existing task management API.
func (s *ProcessingJobStore) PrepareDisplayImages(ctx context.Context, setID string, now time.Time) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.item_uuid,i.content_revision,i.gallery_id FROM gallery_items i
		JOIN galleries g ON g.id=i.gallery_id JOIN gallery_sources source ON source.id=i.source_id
		WHERE g.set_id=? AND i.excluded=0 AND i.availability_state='AVAILABLE' AND source.availability_state='AVAILABLE' AND source.over_limit=0
		AND i.media_kind='STATIC_IMAGE' AND i.content_format='IMAGE'
		AND NOT EXISTS(SELECT 1 FROM media_derivatives d WHERE d.item_uuid=i.item_uuid AND d.variant=? AND d.profile_hash=?
		AND d.content_revision=i.content_revision AND d.is_current=1 AND d.state='READY')`, setID, mediaprocessing.VariantCardDisplay960, mediaprocessing.DisplayProfileHash())
	if err != nil {
		return 0, err
	}
	type candidate struct {
		uuid                string
		revision, galleryID int64
	}
	var items []candidate
	for rows.Next() {
		var i candidate
		if err := rows.Scan(&i.uuid, &i.revision, &i.galleryID); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, i := range items {
		key := ItemDerivativeJobKey(i.uuid, mediaprocessing.VariantCardDisplay960, i.revision, mediaprocessing.DisplayProfileHash())
		job, err := s.FindByKey(ctx, key)
		if err != nil && err != sql.ErrNoRows {
			return count, err
		}
		if err == nil && job.Status != mediaprocessing.JobCompleted && job.Status != mediaprocessing.JobCancelled {
			continue
		}
		if err == nil {
			if _, err = s.RequeueID(ctx, job.ID, 399, now); err != nil {
				return count, err
			}
		} else {
			_, err = s.Enqueue(ctx, EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemDerivative, GalleryID: &i.galleryID, ItemUUID: i.uuid, Variant: mediaprocessing.VariantCardDisplay960, ContentRevision: &i.revision, ProfileHash: mediaprocessing.DisplayProfileHash(), Payload: map[string]any{"cache_tier": mediaprocessing.CacheBase}, Priority: 399}, now)
			if err != nil {
				return count, err
			}
		}
		count++
	}
	return count, nil
}
