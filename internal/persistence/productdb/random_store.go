package productdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"math"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

type randomCandidate struct {
	item      browse.RandomMediaItem
	galleryID int64
}

// RandomItems returns one non-paginated page. Quotas are soft: a short media
// type is filled from other eligible types, and repeated Galleries remain
// possible with a configurable multiplicative selection decay.
func (s *BrowseStore) RandomItems(ctx context.Context, scope browse.Scope, filter browse.RandomMediaFilter) ([]browse.RandomMediaItem, error) {
	settingsValue, err := (&SettingsStore{db: s.db}).Find(ctx)
	if err != nil {
		return nil, err
	}
	if filter != browse.RandomAll && filter != browse.RandomPhoto && filter != browse.RandomSelfie && filter != browse.RandomGIF && filter != browse.RandomVideo {
		return nil, errors.New("invalid random media filter")
	}
	pools := make(map[string][]randomCandidate)
	for _, pool := range []string{"STATIC", "GIF", "VIDEO"} {
		values, err := s.randomPool(ctx, scope, filter, pool, settingsValue.RandomLimit*20)
		if err != nil {
			return nil, err
		}
		pools[pool] = values
	}
	selectedCounts := make(map[int64]int)
	var selected []randomCandidate
	if filter == browse.RandomAll {
		quotas := softQuotaCounts(settingsValue.RandomLimit, settingsValue.RandomStaticQuota, settingsValue.RandomGIFQuota, settingsValue.RandomVideoQuota)
		for index, pool := range []string{"STATIC", "GIF", "VIDEO"} {
			values := pools[pool]
			selected = append(selected, weightedTake(&values, quotas[index], selectedCounts, settingsValue.RandomGalleryRepeatDecay)...)
			pools[pool] = values
		}
	} else {
		pool := "STATIC"
		if filter == browse.RandomGIF {
			pool = "GIF"
		} else if filter == browse.RandomVideo {
			pool = "VIDEO"
		}
		values := pools[pool]
		selected = append(selected, weightedTake(&values, settingsValue.RandomLimit, selectedCounts, settingsValue.RandomGalleryRepeatDecay)...)
		pools[pool] = values
	}
	if len(selected) < settingsValue.RandomLimit {
		remaining := append(append(pools["STATIC"], pools["GIF"]...), pools["VIDEO"]...)
		selected = append(selected, weightedTake(&remaining, settingsValue.RandomLimit-len(selected), selectedCounts, settingsValue.RandomGalleryRepeatDecay)...)
	}
	for index := len(selected) - 1; index > 0; index-- {
		swap := int(secureUnitFloat() * float64(index+1))
		if swap > index {
			swap = index
		}
		selected[index], selected[swap] = selected[swap], selected[index]
	}
	result := make([]browse.RandomMediaItem, 0, len(selected))
	for _, candidate := range selected {
		if err := s.populateRandomRelations(ctx, candidate.galleryID, &candidate.item); err != nil {
			return nil, err
		}
		result = append(result, candidate.item)
	}
	return result, nil
}

func (s *BrowseStore) randomPool(ctx context.Context, scope browse.Scope, filter browse.RandomMediaFilter, pool string, limit int) ([]randomCandidate, error) {
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return nil, err
	}
	kindSQL := `item.media_kind='STATIC_IMAGE'`
	variant := mediaprocessing.VariantCard480
	if pool == "GIF" {
		kindSQL = `item.media_kind='ANIMATED_IMAGE'`
		variant = mediaprocessing.VariantStaticPoster
	} else if pool == "VIDEO" {
		kindSQL = `item.media_kind='VIDEO'`
		variant = mediaprocessing.VariantStaticPoster
	}
	if filter == browse.RandomPhoto {
		kindSQL += ` AND item.image_category='PHOTO'`
	} else if filter == browse.RandomSelfie {
		kindSQL += ` AND item.image_category='SELFIE'`
	} else if (filter == browse.RandomGIF && pool != "GIF") || (filter == browse.RandomVideo && pool != "VIDEO") {
		return nil, nil
	}
	args := []any{variant}
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT item.item_uuid,item.media_kind,item.image_category,gallery.id,gallery.set_id,gallery.slug,
		derivative.content_revision,derivative.profile_hash,derivative.variant,derivative.mime_type,
		COALESCE(personal_item.favorite,0),personal_item.rating_half_steps
		FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id
		JOIN gallery_sources source ON source.gallery_id=gallery.id
		JOIN media_derivatives derivative ON derivative.item_uuid=item.item_uuid AND derivative.variant=? AND derivative.is_current=1 AND derivative.state IN ('READY','STALE')
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		LEFT JOIN gallery_item_personal_states personal_item ON personal_item.gallery_item_id=item.id
		WHERE item.excluded=0 AND item.availability_state='AVAILABLE' AND item.processing_state='READY' AND `+kindSQL+`
		AND `+browseVisibleGalleryPredicate+scopeSQL+` ORDER BY RANDOM() LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []randomCandidate
	for rows.Next() {
		var value randomCandidate
		var category sql.NullString
		var favorite int
		var rating sql.NullInt64
		if err := rows.Scan(&value.item.ItemUUID, &value.item.MediaKind, &category, &value.galleryID,
			&value.item.GallerySetID, &value.item.GallerySlug, &value.item.Resource.ContentRevision,
			&value.item.Resource.ProfileHash, &value.item.Resource.Variant, &value.item.Resource.MIMEType,
			&favorite, &rating); err != nil {
			return nil, err
		}
		value.item.ImageCategory = gallery.ImageCategory(category.String)
		value.item.Resource.ItemUUID = value.item.ItemUUID
		value.item.Favorite = favorite == 1
		if rating.Valid {
			ratingValue := int(rating.Int64)
			value.item.RatingHalfSteps = &ratingValue
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func softQuotaCounts(limit int, values ...float64) []int {
	result := make([]int, len(values))
	type remainder struct {
		index int
		value float64
	}
	var remainders []remainder
	assigned := 0
	for index, value := range values {
		exact := float64(limit) * value
		result[index] = int(math.Floor(exact))
		assigned += result[index]
		remainders = append(remainders, remainder{index: index, value: exact - float64(result[index])})
	}
	for assigned < limit {
		best := 0
		for index := 1; index < len(remainders); index++ {
			if remainders[index].value > remainders[best].value {
				best = index
			}
		}
		result[remainders[best].index]++
		remainders[best].value = -1
		assigned++
	}
	return result
}

func weightedTake(pool *[]randomCandidate, count int, galleryCounts map[int64]int, decay float64) []randomCandidate {
	var result []randomCandidate
	for len(*pool) > 0 && len(result) < count {
		var total float64
		weights := make([]float64, len(*pool))
		for index, candidate := range *pool {
			weights[index] = math.Pow(decay, float64(galleryCounts[candidate.galleryID]))
			total += weights[index]
		}
		selectedIndex := 0
		if total > 0 {
			needle := secureUnitFloat() * total
			for index, weight := range weights {
				needle -= weight
				if needle <= 0 {
					selectedIndex = index
					break
				}
			}
		}
		selected := (*pool)[selectedIndex]
		*pool = append((*pool)[:selectedIndex], (*pool)[selectedIndex+1:]...)
		galleryCounts[selected.galleryID]++
		result = append(result, selected)
	}
	return result
}

func secureUnitFloat() float64 {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return 0.5
	}
	return float64(binary.LittleEndian.Uint64(data[:])>>11) / float64(uint64(1)<<53)
}

func (s *BrowseStore) populateRandomRelations(ctx context.Context, galleryID int64, item *browse.RandomMediaItem) error {
	rows, err := s.db.QueryContext(ctx, `SELECT coser.uuid,coser.name FROM gallery_credits credit JOIN cosers coser ON coser.uuid=credit.coser_uuid
		WHERE credit.gallery_id=? ORDER BY credit.position,credit.id LIMIT 3`, galleryID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value browse.EntitySummary
		if err := rows.Scan(&value.UUID, &value.Name); err != nil {
			rows.Close()
			return err
		}
		item.Cosers = append(item.Cosers, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	characters, err := s.db.QueryContext(ctx, `SELECT DISTINCT character.uuid,character.name,credit.position,cast_item.position
		FROM gallery_cast cast_item JOIN characters character ON character.uuid=cast_item.character_uuid
		JOIN gallery_credits credit ON credit.id=cast_item.gallery_credit_id WHERE cast_item.gallery_id=?
		ORDER BY credit.position,cast_item.position,character.uuid LIMIT 3`, galleryID)
	if err != nil {
		return err
	}
	defer characters.Close()
	for characters.Next() {
		var value browse.EntitySummary
		var creditPosition, castPosition int64
		if err := characters.Scan(&value.UUID, &value.Name, &creditPosition, &castPosition); err != nil {
			return err
		}
		item.Characters = append(item.Characters, value)
	}
	return characters.Err()
}
