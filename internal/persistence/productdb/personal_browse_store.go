package productdb

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func (s *BrowseStore) FavoriteGalleries(ctx context.Context, scope browse.Scope, page int) (browse.GalleryPage, error) {
	return s.galleryPage(ctx, scope, page, browse.GallerySortRecentlyAdded, ` AND personal.favorite=1`, nil,
		`personal.favorited_at_utc DESC,gallery.id DESC`)
}

func (s *BrowseStore) GalleryHistory(ctx context.Context, scope browse.Scope, page int) (browse.GalleryPage, error) {
	return s.galleryPage(ctx, scope, page, browse.GallerySortRecentlyAdded, ` AND personal.last_viewed_at_utc IS NOT NULL`, nil,
		`personal.last_viewed_at_utc DESC,gallery.id DESC`)
}

func (s *BrowseStore) FavoriteMedia(ctx context.Context, scope browse.Scope, page int, ratingSort bool) (browse.MediaPage, error) {
	if page < 1 || page > 1_000_000 {
		return browse.MediaPage{}, errors.New("media page is out of range")
	}
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return browse.MediaPage{}, err
	}
	args := []any{}
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	where := `item.excluded=0 AND item.availability_state='AVAILABLE' AND item.processing_state='READY' AND item_personal.favorite=1 AND ` + browseVisibleGalleryPredicate + scopeSQL
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id
		JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		JOIN gallery_item_personal_states item_personal ON item_personal.gallery_item_id=item.id WHERE `+where, args...).Scan(&total); err != nil {
		return browse.MediaPage{}, err
	}
	order := `item_personal.favorited_at_utc DESC,item.id DESC`
	if ratingSort {
		order = `item_personal.rating_half_steps IS NULL,item_personal.rating_half_steps DESC,item_personal.favorited_at_utc DESC,item.id DESC`
	}
	queryArgs := append(append([]any{}, args...), 24, (page-1)*24)
	rows, err := s.db.QueryContext(ctx, `SELECT item.item_uuid,item.media_kind,item.image_category,gallery.id,gallery.set_id,gallery.slug,
		derivative.content_revision,derivative.profile_hash,derivative.variant,derivative.mime_type,item_personal.rating_half_steps
		FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id JOIN gallery_item_personal_states item_personal ON item_personal.gallery_item_id=item.id
		JOIN media_derivatives derivative ON derivative.item_uuid=item.item_uuid AND derivative.is_current=1 AND derivative.state IN ('READY','STALE')
		AND derivative.variant=CASE WHEN item.media_kind='STATIC_IMAGE' THEN ? ELSE ? END
		WHERE `+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append([]any{mediaprocessing.VariantCard480, mediaprocessing.VariantStaticPoster}, queryArgs...)...)
	if err != nil {
		return browse.MediaPage{}, err
	}
	defer rows.Close()
	result := browse.MediaPage{Page: page, PageSize: 24, TotalItems: total, TotalPages: int(math.Ceil(float64(total) / 24))}
	for rows.Next() {
		var item browse.RandomMediaItem
		var galleryID int64
		var category sql.NullString
		var rating sql.NullInt64
		if err := rows.Scan(&item.ItemUUID, &item.MediaKind, &category, &galleryID, &item.GallerySetID, &item.GallerySlug, &item.Resource.ContentRevision,
			&item.Resource.ProfileHash, &item.Resource.Variant, &item.Resource.MIMEType, &rating); err != nil {
			return browse.MediaPage{}, err
		}
		item.ImageCategory = gallery.ImageCategory(category.String)
		item.Resource.ItemUUID = item.ItemUUID
		item.Favorite = true
		if rating.Valid {
			value := int(rating.Int64)
			item.RatingHalfSteps = &value
		}
		if err := s.populateRandomRelations(ctx, galleryID, &item); err != nil {
			return browse.MediaPage{}, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func (s *PersonalStateStore) SetGalleryFavoriteBySetID(ctx context.Context, setID string, favorite bool, now time.Time) error {
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM galleries WHERE set_id=?`, setID).Scan(&id); err != nil {
		return err
	}
	return s.SetGalleryFavorite(ctx, id, favorite, now)
}
func (s *PersonalStateStore) SetGalleryRatingBySetID(ctx context.Context, setID string, rating *int, expected int64, now time.Time) (int64, error) {
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM galleries WHERE set_id=?`, setID).Scan(&id); err != nil {
		return 0, err
	}
	if err := s.SetGalleryRating(ctx, id, rating, expected, now); err != nil {
		return 0, err
	}
	var revision int64
	err := s.db.QueryRowContext(ctx, `SELECT metadata_revision FROM galleries WHERE id=?`, id).Scan(&revision)
	return revision, err
}
func (s *PersonalStateStore) SetItemFavoriteByUUID(ctx context.Context, uuid string, favorite bool, now time.Time) error {
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM gallery_items WHERE item_uuid=?`, uuid).Scan(&id); err != nil {
		return err
	}
	return s.SetItemFavorite(ctx, id, favorite, now)
}
func (s *PersonalStateStore) SetItemRatingByUUID(ctx context.Context, uuid string, rating *int, expected int64, now time.Time) (int64, error) {
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM gallery_items WHERE item_uuid=?`, uuid).Scan(&id); err != nil {
		return 0, err
	}
	if err := s.SetItemRating(ctx, id, rating, expected, now); err != nil {
		return 0, err
	}
	var revision int64
	err := s.db.QueryRowContext(ctx, `SELECT gallery.metadata_revision FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id WHERE item.id=?`, id).Scan(&revision)
	return revision, err
}
func (s *PersonalStateStore) RecordGalleryViewBySetID(ctx context.Context, setID string, itemUUID *string, now time.Time) error {
	var galleryID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM galleries WHERE set_id=?`, setID).Scan(&galleryID); err != nil {
		return err
	}
	var itemID *int64
	if itemUUID != nil {
		var value int64
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM gallery_items WHERE item_uuid=? AND gallery_id=?`, *itemUUID, galleryID).Scan(&value); err != nil {
			return err
		}
		itemID = &value
	}
	return s.RecordGalleryView(ctx, galleryID, itemID, now)
}
