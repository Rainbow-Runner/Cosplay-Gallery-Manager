package productdb

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

const browseGalleryPageSize = 24

type BrowseStore struct{ db *sql.DB }

var ErrBrowseGalleryNotVisible = errors.New("Gallery is not visible in the requested Browse scope")

func (db *Database) Browse() *BrowseStore { return &BrowseStore{db: db.DB} }

func (s *BrowseStore) Home(ctx context.Context, page int) (browse.GalleryPage, browse.Scope, error) {
	settingsValue, err := (&SettingsStore{db: s.db}).Find(ctx)
	if err != nil {
		return browse.GalleryPage{}, "", err
	}
	scope := browse.Scope(settingsValue.HomeScope)
	result, err := s.Galleries(ctx, scope, page, browse.GallerySortRecentlyAdded)
	return result, scope, err
}

func (s *BrowseStore) Galleries(ctx context.Context, scope browse.Scope, page int, sortBy browse.GallerySort) (browse.GalleryPage, error) {
	return s.GalleriesByCollection(ctx, scope, page, sortBy, "")
}

func (s *BrowseStore) GalleriesByCollection(ctx context.Context, scope browse.Scope, page int, sortBy browse.GallerySort, collectionType browse.CollectionType) (browse.GalleryPage, error) {
	collectionSQL, err := browseCollectionPredicate(collectionType)
	if err != nil {
		return browse.GalleryPage{}, err
	}
	return s.galleryPage(ctx, scope, page, sortBy, collectionSQL, nil, "")
}

// Timeline is the only Browse list that excludes unknown shoot dates and
// normalizes MONTH precision to the first day of that month for ordering. A
// Coser UUID optionally narrows the same ALL/LIST/MAGIC contract.
func (s *BrowseStore) Timeline(ctx context.Context, scope browse.Scope, page int, coserUUID string) (browse.GalleryPage, error) {
	extra := ` AND gallery.shoot_date IS NOT NULL`
	var args []any
	if coserUUID != "" {
		extra += ` AND EXISTS(SELECT 1 FROM gallery_credits timeline_credit WHERE timeline_credit.gallery_id=gallery.id AND timeline_credit.coser_uuid=?)`
		args = append(args, coserUUID)
	}
	order := `CASE gallery.shoot_date_precision WHEN 'MONTH' THEN gallery.shoot_date||'-01' ELSE gallery.shoot_date END DESC,
		gallery.added_at_utc DESC,gallery.id DESC`
	return s.galleryPage(ctx, scope, page, browse.GallerySortShootDate, extra, args, order)
}

func (s *BrowseStore) galleryPage(ctx context.Context, scope browse.Scope, page int, sortBy browse.GallerySort, extraWhere string, extraArgs []any, orderOverride string) (browse.GalleryPage, error) {
	if page < 1 || page > 1_000_000 {
		return browse.GalleryPage{}, errors.New("Browse Gallery page is out of range")
	}
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return browse.GalleryPage{}, err
	}
	orderSQL, err := browseGalleryOrder(sortBy)
	if err != nil {
		return browse.GalleryPage{}, err
	}
	if orderOverride != "" {
		orderSQL = orderOverride
	}
	where := browseVisibleGalleryPredicate + scopeSQL + extraWhere
	args := []any{}
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	args = append(args, extraArgs...)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM galleries gallery
		JOIN gallery_sources source ON source.gallery_id=gallery.id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE `+where, args...).Scan(&total); err != nil {
		return browse.GalleryPage{}, err
	}
	queryArgs := append(append([]any{}, args...), browseGalleryPageSize, (page-1)*browseGalleryPageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT gallery.id,gallery.set_id,gallery.slug,gallery.title,gallery.content_rating,
		gallery.shoot_date,gallery.shoot_date_precision,gallery.added_at_utc,gallery.scrubber_revision,
		CASE WHEN EXISTS(SELECT 1 FROM gallery_cast cast_item WHERE cast_item.gallery_id=gallery.id) THEN 'COSPLAY' ELSE 'ALBUM' END,
		COALESCE(personal.favorite,0),personal.rating_half_steps,
		COALESCE(cover.effective_kind,'NONE'),COALESCE(cover.cover_revision,0),COALESCE(cover.warning_code,''),
		cover_derivative.item_uuid,cover_derivative.content_revision,cover_derivative.profile_hash,
		cover_derivative.variant,cover_derivative.mime_type,
		(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.excluded=0 AND item.availability_state='AVAILABLE' AND item.media_kind='STATIC_IMAGE' AND item.image_category='PHOTO'),
		(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.excluded=0 AND item.availability_state='AVAILABLE' AND item.media_kind='STATIC_IMAGE' AND item.image_category='SELFIE'),
		(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.excluded=0 AND item.availability_state='AVAILABLE' AND item.media_kind='ANIMATED_IMAGE'),
		(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.excluded=0 AND item.availability_state='AVAILABLE' AND item.media_kind='VIDEO'),
		(SELECT COUNT(*) FROM gallery_items item JOIN media_derivatives derivative ON derivative.item_uuid=item.item_uuid
		 WHERE item.gallery_id=gallery.id AND item.excluded=0 AND item.availability_state='AVAILABLE' AND derivative.is_current=1 AND derivative.state IN ('READY','STALE')
		 AND ((item.media_kind='STATIC_IMAGE' AND derivative.variant=?) OR (item.media_kind IN ('ANIMATED_IMAGE','VIDEO') AND derivative.variant=?)))
		FROM galleries gallery JOIN gallery_sources source ON source.gallery_id=gallery.id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		LEFT JOIN gallery_covers cover ON cover.gallery_id=gallery.id
		LEFT JOIN media_derivatives cover_derivative ON cover_derivative.item_uuid=cover.effective_item_uuid AND cover_derivative.is_current=1
		 AND cover_derivative.variant=CASE WHEN cover.effective_kind='VIDEO_POSTER' THEN ? ELSE ? END
		WHERE `+where+` ORDER BY `+orderSQL+` LIMIT ? OFFSET ?`, append([]any{mediaprocessing.VariantCard480, mediaprocessing.VariantStaticPoster,
		mediaprocessing.VariantStaticPoster, mediaprocessing.VariantCard480}, queryArgs...)...)
	if err != nil {
		return browse.GalleryPage{}, err
	}
	defer rows.Close()
	var cards []browse.GalleryCard
	var galleryIDs []int64
	for rows.Next() {
		var card browse.GalleryCard
		var galleryID int64
		var shootDate, precision sql.NullString
		var addedAt string
		var favorite int
		var rating sql.NullInt64
		var coverKind gallery.CoverKind
		var warning string
		var resourceItem, resourceProfile, resourceVariant, resourceMIME sql.NullString
		var resourceRevision sql.NullInt64
		if err := rows.Scan(&galleryID, &card.SetID, &card.Slug, &card.Title, &card.ContentRating,
			&shootDate, &precision, &addedAt, &card.ScrubberRevision, &card.CollectionType,
			&favorite, &rating, &coverKind, &card.Cover.Revision, &warning,
			&resourceItem, &resourceRevision, &resourceProfile, &resourceVariant, &resourceMIME,
			&card.Media.Photo, &card.Media.Selfie, &card.Media.GIF, &card.Media.Video, &card.ScrubberCount); err != nil {
			return browse.GalleryPage{}, err
		}
		card.ShootDate, card.ShootDatePrecision = shootDate.String, gallery.ShootDatePrecision(precision.String)
		card.AddedAtUTC, err = parseTime(addedAt)
		if err != nil {
			return browse.GalleryPage{}, err
		}
		card.Favorite = favorite == 1
		if rating.Valid {
			value := int(rating.Int64)
			card.RatingHalfSteps = &value
		}
		card.Cover.Kind, card.Cover.Managed, card.Cover.Warning = coverKind, coverKind == gallery.CoverManaged, warning != ""
		if resourceItem.Valid && resourceRevision.Valid {
			card.Cover.Resource = &browse.ResourceIdentity{ItemUUID: resourceItem.String, ContentRevision: resourceRevision.Int64,
				ProfileHash: resourceProfile.String, Variant: resourceVariant.String, MIMEType: resourceMIME.String}
		}
		cards = append(cards, card)
		galleryIDs = append(galleryIDs, galleryID)
	}
	if err := rows.Err(); err != nil {
		return browse.GalleryPage{}, err
	}
	for index, galleryID := range galleryIDs {
		if err := s.populateGalleryCardRelations(ctx, galleryID, &cards[index]); err != nil {
			return browse.GalleryPage{}, err
		}
	}
	return browse.GalleryPage{Items: cards, Page: page, PageSize: browseGalleryPageSize, TotalItems: total,
		TotalPages: int(math.Ceil(float64(total) / browseGalleryPageSize))}, nil
}

const browseVisibleGalleryPredicate = `gallery.state='ACTIVE' AND gallery.added_at_utc IS NOT NULL
	AND source.availability_state='AVAILABLE' AND source.over_limit=0 AND COALESCE(personal.hidden,0)=0
	AND NOT EXISTS(SELECT 1 FROM gallery_source_issues issue WHERE issue.source_id=source.id
		AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL)`

func browseScopePredicate(scope browse.Scope) (string, any, error) {
	switch scope {
	case browse.ScopeList:
		return ` AND gallery.content_rating=?`, gallery.ContentRatingNonAdult, nil
	case browse.ScopeMagic:
		return ` AND gallery.content_rating=?`, gallery.ContentRatingAdult, nil
	case browse.ScopeAll:
		return "", "", nil
	default:
		return "", "", errors.New("invalid Browse scope")
	}
}

func browseCollectionPredicate(collectionType browse.CollectionType) (string, error) {
	switch collectionType {
	case "":
		return "", nil
	case browse.CollectionCosplay:
		return ` AND EXISTS(SELECT 1 FROM gallery_cast collection_cast WHERE collection_cast.gallery_id=gallery.id)`, nil
	case browse.CollectionAlbum:
		return ` AND NOT EXISTS(SELECT 1 FROM gallery_cast collection_cast WHERE collection_cast.gallery_id=gallery.id)`, nil
	default:
		return "", errors.New("invalid Gallery collection type")
	}
}

func browseGalleryOrder(sortBy browse.GallerySort) (string, error) {
	switch sortBy {
	case "", browse.GallerySortRecentlyAdded:
		return `gallery.added_at_utc DESC,gallery.id DESC`, nil
	case browse.GallerySortName:
		return `gallery.title COLLATE NOCASE,gallery.id`, nil
	case browse.GallerySortShootDate:
		return `gallery.shoot_date IS NULL,gallery.shoot_date DESC,gallery.added_at_utc DESC,gallery.id DESC`, nil
	case browse.GallerySortRating:
		return `personal.rating_half_steps IS NULL,personal.rating_half_steps DESC,gallery.added_at_utc DESC,gallery.id DESC`, nil
	default:
		return "", errors.New("invalid Browse Gallery sort")
	}
}

func (s *BrowseStore) populateGalleryCardRelations(ctx context.Context, galleryID int64, card *browse.GalleryCard) error {
	if err := s.populateCardCredits(ctx, galleryID, card); err != nil {
		return err
	}
	return s.populateCardCast(ctx, galleryID, card)
}

func (s *BrowseStore) galleryCardByID(ctx context.Context, scope browse.Scope, galleryID int64) (browse.GalleryCard, error) {
	page, err := s.galleryPage(ctx, scope, 1, browse.GallerySortRecentlyAdded, ` AND gallery.id=?`, []any{galleryID}, "")
	if err != nil {
		return browse.GalleryCard{}, err
	}
	if len(page.Items) != 1 {
		return browse.GalleryCard{}, ErrBrowseGalleryNotVisible
	}
	return page.Items[0], nil
}

func (s *BrowseStore) populateCardCredits(ctx context.Context, galleryID int64, card *browse.GalleryCard) error {
	rows, err := s.db.QueryContext(ctx, `SELECT coser.uuid,coser.name,coser.avatar_path<>'',coser.metadata_revision FROM gallery_credits credit
		JOIN cosers coser ON coser.uuid=credit.coser_uuid WHERE credit.gallery_id=? ORDER BY credit.position,credit.id`, galleryID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value browse.PersonSummary
		if err := rows.Scan(&value.UUID, &value.Name, &value.AvatarAvailable, &value.AssetRevision); err != nil {
			return err
		}
		card.CreditCount++
		if len(card.Credits) < 3 {
			card.Credits = append(card.Credits, value)
		}
	}
	return rows.Err()
}

func (s *BrowseStore) populateCardCast(ctx context.Context, galleryID int64, card *browse.GalleryCard) error {
	rows, err := s.db.QueryContext(ctx, `SELECT character.uuid,character.name,work.uuid,work.name
		FROM gallery_cast cast_item JOIN characters character ON character.uuid=cast_item.character_uuid
		JOIN works work ON work.uuid=character.work_uuid JOIN gallery_credits credit ON credit.id=cast_item.gallery_credit_id
		WHERE cast_item.gallery_id=? ORDER BY credit.position,cast_item.position,cast_item.id`, galleryID)
	if err != nil {
		return err
	}
	defer rows.Close()
	seenWork := make(map[string]struct{})
	for rows.Next() {
		var character, work browse.EntitySummary
		if err := rows.Scan(&character.UUID, &character.Name, &work.UUID, &work.Name); err != nil {
			return err
		}
		card.CharacterCount++
		if len(card.Characters) < 3 {
			card.Characters = append(card.Characters, character)
		}
		if _, exists := seenWork[work.UUID]; !exists {
			seenWork[work.UUID] = struct{}{}
			card.WorkCount++
			if len(card.Works) < 3 {
				card.Works = append(card.Works, work)
			}
		}
	}
	return rows.Err()
}
