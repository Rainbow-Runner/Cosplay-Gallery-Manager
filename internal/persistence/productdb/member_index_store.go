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
	if err := s.db.QueryRowContext(ctx, `SELECT metadata_revision,scan_revision,time_evidence_revision FROM galleries WHERE id=?`, galleryID).Scan(
		&result.MetadataRevision, &result.ScanRevision, &result.TimeEvidenceRevision); err != nil {
		return browse.GalleryMemberIndex{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT item.item_uuid,item.media_kind,item.content_format,item.image_category,
		item.position,item.caption,item.processing_state,COALESCE(personal.favorite,0),personal.rating_half_steps,
		card.item_uuid,card.content_revision,card.profile_hash,card.variant,card.mime_type,COALESCE(card.width,0),COALESCE(card.height,0),
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
			&cardItem, &cardRevision, &cardProfile, &cardVariant, &cardMIME, &item.PreviewWidth, &item.PreviewHeight,
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
	if err := rows.Err(); err != nil {
		return result, err
	}
	rows.Close()
	displays, err := s.displayResources(ctx, galleryID, "")
	if err != nil {
		return result, err
	}
	for index := range result.Items {
		item := &result.Items[index]
		if display := displays[item.ItemUUID]; display != nil {
			item.FallbackCardResource = item.CardResource
			item.CardResource = display.Resource
			item.PreviewWidth, item.PreviewHeight = display.Width, display.Height
		}
	}
	rows, err = s.db.QueryContext(ctx, `SELECT item.item_uuid,COALESCE(e.capture_at_utc,''),COALESCE(e.capture_precision,''),COALESCE(e.capture_zone_source,''),e.item_uuid,e.state FROM gallery_items item JOIN gallery_sources src ON src.id=item.source_id LEFT JOIN media_libraries lib ON lib.id=src.library_id LEFT JOIN item_media_times e ON e.item_uuid=item.item_uuid AND e.content_revision=item.content_revision AND e.profile=? AND e.timezone=COALESCE(lib.capture_timezone,'UTC') WHERE item.gallery_id=? AND item.excluded=0 AND item.availability_state='AVAILABLE' AND item.media_kind IN ('STATIC_IMAGE','VIDEO')`, MediaTimeProfile, galleryID)
	if err != nil {
		return result, err
	}
	byUUID := map[string]*browse.GalleryMember{}
	for i := range result.Items {
		byUUID[result.Items[i].ItemUUID] = &result.Items[i]
	}
	result.TimeEvidenceStatus = "COMPLETE"
	for rows.Next() {
		var uuid, stamp, precision, zone string
		var found, state sql.NullString
		if err = rows.Scan(&uuid, &stamp, &precision, &zone, &found, &state); err != nil {
			rows.Close()
			return result, err
		}
		if !found.Valid {
			result.TimeEvidenceStatus = "PENDING"
		} else if state.String == "PARTIAL" && result.TimeEvidenceStatus != "PENDING" {
			result.TimeEvidenceStatus = "PARTIAL"
		}
		if item := byUUID[uuid]; item != nil {
			item.CaptureTimeUTC = stamp
			item.CaptureTimePrecision = precision
			item.CaptureTimeZoneSource = zone
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	return result, nil
}
