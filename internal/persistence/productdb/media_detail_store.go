package productdb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func (s *BrowseStore) MediaDetail(ctx context.Context, itemUUID string) (browse.MediaDetail, error) {
	var galleryID int64
	var setID string
	if err := s.db.QueryRowContext(ctx, `SELECT gallery.id,gallery.set_id FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id
		WHERE item.item_uuid=?`, itemUUID).Scan(&galleryID, &setID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return browse.MediaDetail{}, ErrBrowseGalleryNotVisible
		}
		return browse.MediaDetail{}, err
	}
	card, err := s.galleryCardByID(ctx, browse.ScopeAll, galleryID)
	if err != nil {
		return browse.MediaDetail{}, err
	}
	index, err := s.GalleryMemberIndex(ctx, setID, browse.ScopeAll)
	if err != nil {
		return browse.MediaDetail{}, err
	}
	result := browse.MediaDetail{Gallery: card, MetadataRevision: index.MetadataRevision}
	found := false
	for _, item := range index.Items {
		if item.ItemUUID == itemUUID {
			result.Item = item
			found = true
			break
		}
	}
	if !found {
		return browse.MediaDetail{}, ErrBrowseGalleryNotVisible
	}
	variants := []string{mediaprocessing.VariantLightbox4096, mediaprocessing.VariantAnimatedPreview, mediaprocessing.VariantVideoPlayback,
		mediaprocessing.VariantCard480, mediaprocessing.VariantStaticPoster}
	for _, variant := range variants {
		var resource browse.ResourceIdentity
		err := s.db.QueryRowContext(ctx, `SELECT item_uuid,content_revision,profile_hash,variant,mime_type FROM media_derivatives
			WHERE item_uuid=? AND variant=? AND is_current=1 AND state IN ('READY','STALE')`, itemUUID, variant).Scan(
			&resource.ItemUUID, &resource.ContentRevision, &resource.ProfileHash, &resource.Variant, &resource.MIMEType)
		if err == nil {
			result.DisplayResource = &resource
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return browse.MediaDetail{}, err
		}
	}
	if result.Item.MediaKind == "VIDEO" {
		metadata, findErr := (&VideoMetadataStore{db: s.db}).Find(ctx, itemUUID)
		if findErr == nil {
			result.VideoTechnical = &browse.VideoTechnicalSummary{ProbeState: string(metadata.ProbeState), ErrorCode: metadata.LastErrorCode, Container: metadata.Container,
				DurationSeconds: metadata.DurationSeconds, Width: metadata.DisplayWidth, Height: metadata.DisplayHeight, FrameRate: metadata.FrameRate,
				VideoCodec: metadata.VideoCodec, AudioCodec: metadata.AudioCodec}
		} else if !errors.Is(findErr, sql.ErrNoRows) {
			return browse.MediaDetail{}, findErr
		}
	}
	return result, nil
}
