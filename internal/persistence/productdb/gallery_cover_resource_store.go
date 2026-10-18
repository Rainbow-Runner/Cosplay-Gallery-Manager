package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

var ErrGalleryCover1600Disabled = errors.New("GALLERY_COVER_1600_DISABLED")

// A caller may enhance only the current static cover of a visible Gallery,
// never arbitrary members, drafts, hidden Galleries, or physical source paths.
func (s *BrowseStore) authorizeGalleryCover(ctx context.Context, setID, itemUUID, variant string) (onDemandLightboxIdentity, error) {
	if variant != mediaprocessing.VariantCard960 && variant != mediaprocessing.VariantCard1600 && variant != mediaprocessing.VariantCardDisplay1600 {
		return onDemandLightboxIdentity{}, ErrBrowseGalleryNotVisible
	}
	var value onDemandLightboxIdentity
	err := s.db.QueryRowContext(ctx, `SELECT gallery.id,item.content_revision
		FROM galleries gallery JOIN gallery_sources source ON source.gallery_id=gallery.id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		JOIN gallery_covers cover ON cover.gallery_id=gallery.id AND cover.effective_kind='ITEM'
		JOIN gallery_items item ON item.item_uuid=cover.effective_item_uuid AND item.gallery_id=gallery.id
		WHERE gallery.set_id=? AND item.item_uuid=? AND item.media_kind='STATIC_IMAGE'
		AND (?<>'CARD_DISPLAY_1600' OR item.content_format='IMAGE')
		AND item.excluded=0 AND item.availability_state='AVAILABLE' AND `+browseVisibleGalleryPredicate,
		setID, itemUUID, variant).Scan(&value.galleryID, &value.contentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return value, ErrBrowseGalleryNotVisible
	}
	if err == nil && (variant == mediaprocessing.VariantCard1600 || variant == mediaprocessing.VariantCardDisplay1600) {
		settings, findErr := (&SettingsStore{db: s.db}).Find(ctx)
		if findErr != nil {
			return value, findErr
		}
		if !settings.GalleryCover1600Enabled {
			return value, ErrGalleryCover1600Disabled
		}
	}
	return value, err
}

func (s *BrowseStore) GalleryCoverStatus(ctx context.Context, setID, itemUUID, variant string) (browse.OnDemandResource, error) {
	identity, err := s.authorizeGalleryCover(ctx, setID, itemUUID, variant)
	if err != nil {
		return browse.OnDemandResource{}, err
	}
	return s.imageDerivativeStatus(ctx, itemUUID, identity.contentRevision, variant, mediaprocessing.GalleryCoverProfileHash(variant))
}

func (s *BrowseStore) RequestGalleryCover(ctx context.Context, setID, itemUUID, variant string, now time.Time) (browse.OnDemandResource, error) {
	identity, err := s.authorizeGalleryCover(ctx, setID, itemUUID, variant)
	if err != nil {
		return browse.OnDemandResource{}, err
	}
	profile := mediaprocessing.GalleryCoverProfileHash(variant)
	status, err := s.imageDerivativeStatus(ctx, itemUUID, identity.contentRevision, variant, profile)
	if err != nil || status.Resource != nil {
		return status, err
	}
	key := ItemDerivativeJobKey(itemUUID, variant, identity.contentRevision, profile)
	job, err := (&ProcessingJobStore{db: s.db}).FindByKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		revision := identity.contentRevision
		_, err = (&ProcessingJobStore{db: s.db}).Enqueue(ctx, EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemDerivative,
			GalleryID: &identity.galleryID, ItemUUID: itemUUID, Variant: variant, ContentRevision: &revision, ProfileHash: profile,
			Payload: map[string]any{"cache_tier": mediaprocessing.CacheEnhanced, "gallery_cover": true}, Priority: 150}, now)
	} else if err == nil && (job.Status == mediaprocessing.JobCompleted || job.Status == mediaprocessing.JobCancelled) {
		// Failed jobs remain failed until a user retries them through the task
		// manager; merely browsing must not repeatedly reopen structural errors.
		_, err = (&ProcessingJobStore{db: s.db}).Requeue(ctx, key, 150, now)
		if err != nil {
			if current, findErr := (&ProcessingJobStore{db: s.db}).FindByKey(ctx, key); findErr == nil &&
				(current.Status == mediaprocessing.JobPending || current.Status == mediaprocessing.JobRunning || current.Status == mediaprocessing.JobRetryWait || current.Status == mediaprocessing.JobPaused) {
				err = nil
			}
		}
	}
	if err != nil {
		return browse.OnDemandResource{}, err
	}
	return s.imageDerivativeStatus(ctx, itemUUID, identity.contentRevision, variant, profile)
}

// IsCurrentGalleryCover is rechecked by the worker before reading originals
// and before publication so a queued old cover cannot enhance other members.
func (s *BrowseStore) IsCurrentGalleryCover(ctx context.Context, galleryID int64, itemUUID string) (bool, error) {
	var found int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_covers cover
		JOIN galleries gallery ON gallery.id=cover.gallery_id
		JOIN gallery_sources source ON source.gallery_id=gallery.id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		JOIN gallery_items item ON item.item_uuid=cover.effective_item_uuid AND item.gallery_id=gallery.id
		WHERE cover.gallery_id=? AND cover.effective_kind='ITEM' AND cover.effective_item_uuid=?
		AND item.media_kind='STATIC_IMAGE' AND item.excluded=0 AND item.availability_state='AVAILABLE'
		AND `+browseVisibleGalleryPredicate, galleryID, itemUUID).Scan(&found)
	return found == 1, err
}
