package productdb

import (
	"context"

	"github.com/stashapp/stash/internal/browse"
)

func (s *BrowseStore) GalleryDetailBySlug(ctx context.Context, scope browse.Scope, value string) (browse.GalleryDetail, error) {
	resolved, redirected, err := (&GalleryStore{db: s.db}).ResolveSlug(ctx, value)
	if err != nil {
		return browse.GalleryDetail{}, err
	}
	card, err := s.galleryCardByID(ctx, scope, resolved.ID)
	if err != nil {
		return browse.GalleryDetail{}, err
	}
	result := browse.GalleryDetail{Card: card, Description: resolved.Description, PhotographerName: resolved.PhotographerName,
		StudioName: resolved.StudioName, Redirected: redirected}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(byte_size),0) FROM gallery_items
		WHERE gallery_id=? AND excluded=0 AND availability_state='AVAILABLE'`, resolved.ID).Scan(&result.AvailableBytes); err != nil {
		return browse.GalleryDetail{}, err
	}
	if result.Credits, err = s.galleryCreditDetails(ctx, resolved.ID); err != nil {
		return browse.GalleryDetail{}, err
	}
	if result.Tags, err = s.galleryDirectTags(ctx, resolved.ID); err != nil {
		return browse.GalleryDetail{}, err
	}
	if result.ExternalLinks, err = s.galleryExternalLinks(ctx, resolved.ID); err != nil {
		return browse.GalleryDetail{}, err
	}
	return result, nil
}

func (s *BrowseStore) galleryCreditDetails(ctx context.Context, galleryID int64) ([]browse.CreditDetail, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT credit.id,coser.uuid,coser.name FROM gallery_credits credit
		JOIN cosers coser ON coser.uuid=credit.coser_uuid WHERE credit.gallery_id=? ORDER BY credit.position,credit.id`, galleryID)
	if err != nil {
		return nil, err
	}
	type creditRecord struct {
		id    int64
		value browse.CreditDetail
	}
	var values []creditRecord
	for rows.Next() {
		var value creditRecord
		if err := rows.Scan(&value.id, &value.value.Coser.UUID, &value.value.Coser.Name); err != nil {
			rows.Close()
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range values {
		castRows, err := s.db.QueryContext(ctx, `SELECT character.uuid,character.name,work.uuid,work.name FROM gallery_cast cast_item
			JOIN characters character ON character.uuid=cast_item.character_uuid JOIN works work ON work.uuid=character.work_uuid
			WHERE cast_item.gallery_credit_id=? ORDER BY cast_item.position,cast_item.id`, values[index].id)
		if err != nil {
			return nil, err
		}
		seenWorks := make(map[string]struct{})
		for castRows.Next() {
			var character, work browse.EntitySummary
			if err := castRows.Scan(&character.UUID, &character.Name, &work.UUID, &work.Name); err != nil {
				castRows.Close()
				return nil, err
			}
			values[index].value.Characters = append(values[index].value.Characters, character)
			if _, exists := seenWorks[work.UUID]; !exists {
				seenWorks[work.UUID] = struct{}{}
				values[index].value.Works = append(values[index].value.Works, work)
			}
		}
		if err := castRows.Close(); err != nil {
			return nil, err
		}
	}
	result := make([]browse.CreditDetail, len(values))
	for index := range values {
		result[index] = values[index].value
	}
	return result, nil
}

func (s *BrowseStore) galleryDirectTags(ctx context.Context, galleryID int64) ([]browse.EntitySummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tag.uuid,tag.name FROM gallery_tags relation JOIN tags tag ON tag.uuid=relation.tag_uuid
		WHERE relation.gallery_id=? ORDER BY relation.position,tag.uuid`, galleryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []browse.EntitySummary
	for rows.Next() {
		var value browse.EntitySummary
		if err := rows.Scan(&value.UUID, &value.Name); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *BrowseStore) galleryExternalLinks(ctx context.Context, galleryID int64) ([]browse.ExternalLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT link_uuid,link_type,label,url FROM gallery_external_links
		WHERE gallery_id=? ORDER BY position,link_uuid`, galleryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []browse.ExternalLink
	for rows.Next() {
		var value browse.ExternalLink
		if err := rows.Scan(&value.UUID, &value.Type, &value.Label, &value.URL); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
