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

type onDemandLightboxIdentity struct {
	galleryID       int64
	contentRevision int64
}

// LightboxStatus is a path-free status read for a Browse-visible static item.
func (s *BrowseStore) LightboxStatus(ctx context.Context, itemUUID string) (browse.OnDemandResource, error) {
	identity, err := s.authorizeOnDemandLightbox(ctx, itemUUID)
	if err != nil {
		return browse.OnDemandResource{}, err
	}
	return s.lightboxStatus(ctx, itemUUID, identity.contentRevision)
}

// RequestLightbox idempotently enqueues or reopens the optional 4096 resource.
// Concurrent requests converge on the stable job key.
func (s *BrowseStore) RequestLightbox(ctx context.Context, itemUUID string, now time.Time) (browse.OnDemandResource, error) {
	identity, err := s.authorizeOnDemandLightbox(ctx, itemUUID)
	if err != nil {
		return browse.OnDemandResource{}, err
	}
	status, err := s.lightboxStatus(ctx, itemUUID, identity.contentRevision)
	if err != nil || status.Resource != nil {
		return status, err
	}
	profile := mediaprocessing.DefaultProfileHash()
	key := ItemDerivativeJobKey(itemUUID, mediaprocessing.VariantLightbox4096, identity.contentRevision, profile)
	job, err := (&ProcessingJobStore{db: s.db}).FindByKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		revision := identity.contentRevision
		_, err = (&ProcessingJobStore{db: s.db}).Enqueue(ctx, EnqueueJobInput{
			Key: key, Kind: mediaprocessing.JobItemDerivative, GalleryID: &identity.galleryID,
			ItemUUID: itemUUID, Variant: mediaprocessing.VariantLightbox4096, ContentRevision: &revision,
			ProfileHash: profile, Payload: map[string]any{"cache_tier": mediaprocessing.CacheEnhanced}, Priority: 200,
		}, now)
	} else if err == nil && (job.Status == mediaprocessing.JobCompleted || job.Status == mediaprocessing.JobFailed || job.Status == mediaprocessing.JobCancelled) {
		_, err = (&ProcessingJobStore{db: s.db}).Requeue(ctx, key, 200, now)
		if err != nil {
			// Another request may have reopened the same terminal job first.
			if current, findErr := (&ProcessingJobStore{db: s.db}).FindByKey(ctx, key); findErr == nil &&
				(current.Status == mediaprocessing.JobPending || current.Status == mediaprocessing.JobRunning || current.Status == mediaprocessing.JobRetryWait || current.Status == mediaprocessing.JobPaused) {
				err = nil
			}
		}
	}
	if err != nil {
		return browse.OnDemandResource{}, err
	}
	return s.lightboxStatus(ctx, itemUUID, identity.contentRevision)
}

func (s *BrowseStore) authorizeOnDemandLightbox(ctx context.Context, itemUUID string) (onDemandLightboxIdentity, error) {
	var value onDemandLightboxIdentity
	err := s.db.QueryRowContext(ctx, `SELECT gallery.id,item.content_revision
		FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id
		JOIN gallery_sources source ON source.id=item.source_id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE item.item_uuid=? AND item.media_kind='STATIC_IMAGE' AND item.excluded=0
		AND item.availability_state='AVAILABLE' AND `+browseVisibleGalleryPredicate, itemUUID).Scan(&value.galleryID, &value.contentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return onDemandLightboxIdentity{}, ErrBrowseGalleryNotVisible
	}
	return value, err
}

func (s *BrowseStore) lightboxStatus(ctx context.Context, itemUUID string, revision int64) (browse.OnDemandResource, error) {
	var resource browse.ResourceIdentity
	err := s.db.QueryRowContext(ctx, `SELECT item_uuid,content_revision,profile_hash,variant,mime_type
		FROM media_derivatives WHERE item_uuid=? AND variant=? AND content_revision=? AND is_current=1
		AND state IN ('READY','STALE')`, itemUUID, mediaprocessing.VariantLightbox4096, revision).Scan(
		&resource.ItemUUID, &resource.ContentRevision, &resource.ProfileHash, &resource.Variant, &resource.MIMEType)
	if err == nil {
		return browse.OnDemandResource{Status: gallery.ProcessingReady, Resource: &resource}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return browse.OnDemandResource{}, err
	}
	key := ItemDerivativeJobKey(itemUUID, mediaprocessing.VariantLightbox4096, revision, mediaprocessing.DefaultProfileHash())
	job, err := (&ProcessingJobStore{db: s.db}).FindByKey(ctx, key)
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
