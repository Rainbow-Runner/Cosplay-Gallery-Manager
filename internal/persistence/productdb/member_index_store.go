package productdb

import (
	"context"
	"database/sql"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

// GalleryMemberIndex returns the complete lightweight Browse index once. It
// has no pagination and no cache/source paths; the UI expands each visual
// group in batches of 24 while Lightbox navigation uses this full order.
func (s *BrowseStore) GalleryMemberIndex(ctx context.Context, setID string, scope browse.Scope) (browse.GalleryMemberIndex, error) {
	galleryID, err := s.visibleGalleryID(ctx, setID, scope)
	if err != nil {
		return browse.GalleryMemberIndex{}, err
	}
	result := browse.GalleryMemberIndex{SetID: setID}
	if err := s.db.QueryRowContext(ctx, `SELECT metadata_revision,scan_revision FROM galleries WHERE id=?`, galleryID).Scan(
		&result.MetadataRevision, &result.ScanRevision); err != nil {
		return browse.GalleryMemberIndex{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT item.item_uuid,item.media_kind,item.content_format,item.image_category,
		item.position,item.caption,item.processing_state,COALESCE(personal.favorite,0),personal.rating_half_steps,
		card.item_uuid,card.content_revision,card.profile_hash,card.variant,card.mime_type,
		large.item_uuid,large.content_revision,large.profile_hash,large.variant,large.mime_type
		FROM gallery_items item
		LEFT JOIN gallery_item_personal_states personal ON personal.gallery_item_id=item.id
		LEFT JOIN media_derivatives card ON card.item_uuid=item.item_uuid AND card.is_current=1 AND card.state IN ('READY','STALE')
		 AND card.variant=CASE WHEN item.media_kind='STATIC_IMAGE' THEN ? ELSE ? END
		LEFT JOIN media_derivatives large ON large.item_uuid=item.item_uuid AND large.is_current=1 AND large.state IN ('READY','STALE')
		 AND large.variant=? AND item.media_kind='STATIC_IMAGE'
		WHERE item.gallery_id=? AND item.excluded=0 AND item.availability_state='AVAILABLE'
		ORDER BY CASE WHEN item.media_kind='STATIC_IMAGE' AND item.image_category='PHOTO' THEN 0
		 WHEN item.media_kind='STATIC_IMAGE' AND item.image_category='SELFIE' THEN 1
		 WHEN item.media_kind='ANIMATED_IMAGE' THEN 2 ELSE 3 END,item.position,item.item_uuid`,
		mediaprocessing.VariantCard480, mediaprocessing.VariantStaticPoster, mediaprocessing.VariantLightbox4096, galleryID)
	if err != nil {
		return browse.GalleryMemberIndex{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item browse.GalleryMember
		var category sql.NullString
		var favorite int
		var rating sql.NullInt64
		var cardItem, cardProfile, cardVariant, cardMIME sql.NullString
		var cardRevision sql.NullInt64
		var largeItem, largeProfile, largeVariant, largeMIME sql.NullString
		var largeRevision sql.NullInt64
		if err := rows.Scan(&item.ItemUUID, &item.MediaKind, &item.ContentFormat, &category, &item.Position,
			&item.Caption, &item.ProcessingState, &favorite, &rating,
			&cardItem, &cardRevision, &cardProfile, &cardVariant, &cardMIME,
			&largeItem, &largeRevision, &largeProfile, &largeVariant, &largeMIME); err != nil {
			return browse.GalleryMemberIndex{}, err
		}
		item.ImageCategory = gallery.ImageCategory(category.String)
		item.Favorite = favorite == 1
		if rating.Valid {
			value := int(rating.Int64)
			item.RatingHalfSteps = &value
		}
		if cardItem.Valid && cardRevision.Valid {
			item.CardResource = &browse.ResourceIdentity{ItemUUID: cardItem.String, ContentRevision: cardRevision.Int64,
				ProfileHash: cardProfile.String, Variant: cardVariant.String, MIMEType: cardMIME.String}
		}
		if largeItem.Valid && largeRevision.Valid {
			item.LargeResource = &browse.ResourceIdentity{ItemUUID: largeItem.String, ContentRevision: largeRevision.Int64,
				ProfileHash: largeProfile.String, Variant: largeVariant.String, MIMEType: largeMIME.String}
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
