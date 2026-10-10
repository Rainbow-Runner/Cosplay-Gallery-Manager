package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

type animatedPreviewIdentity struct {
	galleryID int64
	revision  int64
}

func (s *BrowseStore) AnimatedPreviewStatus(ctx context.Context, itemUUID, ffmpegVersion, ffmpegUnavailableCode string) (browse.OnDemandResource, error) {
	identity, err := s.authorizeAnimatedPreview(ctx, itemUUID)
	if err != nil {
		return browse.OnDemandResource{}, err
	}
	return s.animatedPreviewStatus(ctx, itemUUID, identity.revision, ffmpegVersion, ffmpegUnavailableCode)
}

func (s *BrowseStore) RequestAnimatedPreview(ctx context.Context, itemUUID, ffmpegVersion, ffmpegUnavailableCode string, now time.Time) (browse.OnDemandResource, error) {
	identity, err := s.authorizeAnimatedPreview(ctx, itemUUID)
	if err != nil {
		return browse.OnDemandResource{}, err
	}
	status, err := s.animatedPreviewStatus(ctx, itemUUID, identity.revision, ffmpegVersion, ffmpegUnavailableCode)
	if err != nil || status.Resource != nil || status.Status == gallery.ProcessingError || ffmpegVersion == "" {
		return status, err
	}
	profile := mediaprocessing.AnimatedPreviewProfileHash(ffmpegVersion)
	key := ItemDerivativeJobKey(itemUUID, mediaprocessing.VariantAnimatedPreview, identity.revision, profile)
	jobs := &ProcessingJobStore{db: s.db}
	job, err := jobs.FindByKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		revision := identity.revision
		_, err = jobs.Enqueue(ctx, EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemDerivative, GalleryID: &identity.galleryID,
			ItemUUID: itemUUID, Variant: mediaprocessing.VariantAnimatedPreview, ContentRevision: &revision, ProfileHash: profile,
			Payload: map[string]any{"cache_tier": mediaprocessing.CacheEnhanced}, Priority: 300}, now)
	} else if err == nil && (job.Status == mediaprocessing.JobCompleted || job.Status == mediaprocessing.JobFailed || job.Status == mediaprocessing.JobCancelled) {
		_, err = jobs.Requeue(ctx, key, 300, now)
		if err != nil {
			if current, findErr := jobs.FindByKey(ctx, key); findErr == nil &&
				(current.Status == mediaprocessing.JobPending || current.Status == mediaprocessing.JobRunning || current.Status == mediaprocessing.JobRetryWait || current.Status == mediaprocessing.JobPaused) {
				err = nil
			}
		}
	}
	if err != nil {
		return browse.OnDemandResource{}, err
	}
	return s.animatedPreviewStatus(ctx, itemUUID, identity.revision, ffmpegVersion, ffmpegUnavailableCode)
}

func (s *BrowseStore) authorizeAnimatedPreview(ctx context.Context, itemUUID string) (animatedPreviewIdentity, error) {
	var result animatedPreviewIdentity
	err := s.db.QueryRowContext(ctx, `SELECT gallery.id,item.content_revision FROM gallery_items item
		JOIN galleries gallery ON gallery.id=item.gallery_id JOIN gallery_sources source ON source.id=item.source_id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE item.item_uuid=? AND item.media_kind='ANIMATED_IMAGE' AND item.content_format='IMAGE' AND item.excluded=0
		AND item.availability_state='AVAILABLE' AND `+browseVisibleGalleryPredicate, itemUUID).Scan(&result.galleryID, &result.revision)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrBrowseGalleryNotVisible
	}
	return result, err
}

func (s *BrowseStore) animatedPreviewStatus(ctx context.Context, itemUUID string, revision int64, ffmpegVersion, ffmpegUnavailableCode string) (browse.OnDemandResource, error) {
	if ffmpegVersion == "" {
		if ffmpegUnavailableCode == "" {
			ffmpegUnavailableCode = mediaprocessing.ErrorFFmpegUnavailable
		}
		return browse.OnDemandResource{Status: gallery.ProcessingError, ErrorCode: ffmpegUnavailableCode}, nil
	}
	profile := mediaprocessing.AnimatedPreviewProfileHash(ffmpegVersion)
	var resource browse.ResourceIdentity
	err := s.db.QueryRowContext(ctx, `SELECT item_uuid,content_revision,profile_hash,variant,mime_type FROM media_derivatives
		WHERE item_uuid=? AND variant=? AND content_revision=? AND profile_hash=? AND is_current=1 AND state IN ('READY','STALE')`,
		itemUUID, mediaprocessing.VariantAnimatedPreview, revision, profile).Scan(&resource.ItemUUID, &resource.ContentRevision, &resource.ProfileHash, &resource.Variant, &resource.MIMEType)
	if err == nil {
		return browse.OnDemandResource{Status: gallery.ProcessingReady, Resource: &resource}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return browse.OnDemandResource{}, err
	}
	job, err := (&ProcessingJobStore{db: s.db}).FindByKey(ctx, ItemDerivativeJobKey(itemUUID, mediaprocessing.VariantAnimatedPreview, revision, profile))
	if errors.Is(err, sql.ErrNoRows) {
		return browse.OnDemandResource{Status: gallery.ProcessingPending}, nil
	}
	if err != nil {
		return browse.OnDemandResource{}, err
	}
	switch job.Status {
	case mediaprocessing.JobRunning:
		return browse.OnDemandResource{Status: gallery.ProcessingProcessing}, nil
	case mediaprocessing.JobFailed, mediaprocessing.JobCancelled:
		return browse.OnDemandResource{Status: gallery.ProcessingError, ErrorCode: job.LastErrorCode}, nil
	default:
		return browse.OnDemandResource{Status: gallery.ProcessingPending}, nil
	}
}
