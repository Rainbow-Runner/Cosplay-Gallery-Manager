package productdb

import (
	"context"
	"database/sql"
	"sort"

	"github.com/stashapp/stash/internal/gallery"
)

// Compare persisted metadata before any title-ownership or review writes.
// Source-title opt-in/out is itself an edit even when the visible title agrees.
func galleryMetadataUnchanged(ctx context.Context, tx *sql.Tx, current gallery.Gallery, input UpdateGalleryMetadataInput) (bool, error) {
	if current.Title != input.Title || current.Description != input.Description || current.ShootDate != input.ShootDate ||
		current.ShootDatePrecision != input.ShootDatePrecision || current.PublishDate != input.PublishDate ||
		current.PublishDatePrecision != input.PublishDatePrecision || current.ContentRating != input.ContentRating ||
		current.PhotographerName != input.PhotographerName || current.StudioName != input.StudioName || len(current.Aliases) != len(input.Aliases) {
		return false, nil
	}
	for i, alias := range input.Aliases {
		if current.Aliases[i] != normalizedDisplay(alias) {
			return false, nil
		}
	}
	if input.FollowSourceTitle != nil {
		var follows bool
		if err := tx.QueryRowContext(ctx, `SELECT title_origin='AUTO' FROM galleries WHERE id=?`, current.ID).Scan(&follows); err != nil {
			return false, err
		}
		if follows != *input.FollowSourceTitle {
			return false, nil
		}
	}
	return true, nil
}

// Only UUIDs, ownership and relative order are editable relation semantics.
// Sparse position values (e.g. 7/99 vs 1024/2048) do not alone justify a write.
func galleryRelationsUnchanged(ctx context.Context, tx *sql.Tx, galleryID int64, input ReplaceGalleryRelationsInput) (bool, error) {
	credits := append([]ReplaceGalleryCreditInput(nil), input.Credits...)
	sort.Slice(credits, func(i, j int) bool { return credits[i].Position < credits[j].Position })
	for i := range credits {
		credits[i].Cast = append([]ReplaceGalleryCastInput(nil), credits[i].Cast...)
		sort.Slice(credits[i].Cast, func(a, b int) bool { return credits[i].Cast[a].Position < credits[i].Cast[b].Position })
	}
	tags := append([]ReplaceGalleryTagInput(nil), input.Tags...)
	sort.Slice(tags, func(i, j int) bool { return tags[i].Position < tags[j].Position })
	rows, err := tx.QueryContext(ctx, `SELECT cr.coser_uuid,c.character_uuid FROM gallery_credits cr
		LEFT JOIN gallery_cast c ON c.gallery_credit_id=cr.id AND c.gallery_id=cr.gallery_id
		WHERE cr.gallery_id=? ORDER BY cr.position,cr.id,c.position,c.id`, galleryID)
	if err != nil {
		return false, err
	}
	creditIndex, castIndex := -1, 0
	previous := ""
	equal := true
	for rows.Next() {
		var coser string
		var character sql.NullString
		if err := rows.Scan(&coser, &character); err != nil {
			rows.Close()
			return false, err
		}
		if coser != previous {
			if creditIndex >= 0 && creditIndex < len(credits) && castIndex != len(credits[creditIndex].Cast) {
				equal = false
			}
			creditIndex++
			castIndex, previous = 0, coser
		}
		if creditIndex >= len(credits) || credits[creditIndex].CoserUUID != coser {
			equal = false
		}
		if character.Valid {
			if creditIndex >= len(credits) || castIndex >= len(credits[creditIndex].Cast) || credits[creditIndex].Cast[castIndex].CharacterUUID != character.String {
				equal = false
			}
			castIndex++
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if creditIndex+1 != len(credits) || creditIndex >= 0 && creditIndex < len(credits) && castIndex != len(credits[creditIndex].Cast) {
		equal = false
	}
	rows, err = tx.QueryContext(ctx, `SELECT tag_uuid FROM gallery_tags WHERE gallery_id=? ORDER BY position,tag_uuid`, galleryID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return false, err
		}
		if index >= len(tags) || tags[index].TagUUID != uuid {
			equal = false
		}
		index++
	}
	return equal && index == len(tags), rows.Err()
}
