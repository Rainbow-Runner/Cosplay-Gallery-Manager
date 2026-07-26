package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
)

var (
	ErrGalleryItemNotFound       = errors.New("GalleryItem not found")
	ErrGalleryItemNotForgettable = errors.New("GalleryItem must be excluded or missing before it can be forgotten")
	ErrMoveAcrossMediaGroups     = errors.New("GalleryItem cannot be dragged across media groups")
)

// FindItem returns one Gallery-scoped member. It does not expose a standalone
// media business entity; the returned item always retains its Gallery ID.
func (s *GalleryStore) FindItem(ctx context.Context, itemID int64) (gallery.Item, error) {
	item, err := findItem(ctx, s.db, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return gallery.Item{}, ErrGalleryItemNotFound
	}
	return item, err
}

// SetItemExcluded persists the user's decision. Scans update file facts but
// never overwrite this flag, so a present file remains excluded after rescan.
func (s *GalleryStore) SetItemExcluded(
	ctx context.Context,
	itemID int64,
	excluded bool,
	expectedRevision int64,
	now time.Time,
) (gallery.Item, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.Item{}, err
	}
	defer func() { _ = tx.Rollback() }()

	item, err := findItem(ctx, tx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return gallery.Item{}, ErrGalleryItemNotFound
	}
	if err != nil {
		return gallery.Item{}, err
	}
	if item.Excluded == excluded {
		current, err := findGallery(ctx, tx, item.GalleryID)
		if err != nil {
			return gallery.Item{}, err
		}
		if current.MetadataRevision != expectedRevision {
			return gallery.Item{}, ErrMetadataRevisionConflict
		}
		return item, nil
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE gallery_items SET excluded = ?, updated_at_utc = ? WHERE id = ?
	`, boolInt(excluded), formatTime(normalisedTime(now)), itemID); err != nil {
		return gallery.Item{}, fmt.Errorf("updating GalleryItem exclusion: %w", err)
	}
	if err := touchGalleryMetadata(ctx, tx, item.GalleryID, expectedRevision, now); err != nil {
		return gallery.Item{}, err
	}
	if err := demoteInvalidActiveGallery(ctx, tx, item.GalleryID); err != nil {
		return gallery.Item{}, err
	}
	updated, err := findItem(ctx, tx, itemID)
	if err != nil {
		return gallery.Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.Item{}, err
	}
	return updated, nil
}

// ForgetItem removes only the application's member record. It is deliberately
// incapable of deleting the source file. The portable UUID is permanently
// tombstoned so stale Manifests cannot silently recreate the old identity.
func (s *GalleryStore) ForgetItem(
	ctx context.Context,
	itemID int64,
	expectedRevision int64,
	now time.Time,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	item, err := findItem(ctx, tx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrGalleryItemNotFound
	}
	if err != nil {
		return err
	}
	if !item.Excluded && item.Availability != gallery.AvailabilityMissing {
		return ErrGalleryItemNotForgettable
	}
	if err := requireActivePortableKind(ctx, tx, item.UUID, portableid.KindGalleryItem); err != nil {
		return err
	}
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO portable_uuid_tombstones (uuid, entity_kind, deleted_at_utc, reason)
		VALUES (?, 'GALLERY_ITEM', ?, 'GalleryItem explicitly forgotten')
	`, item.UUID, timestamp); err != nil {
		return fmt.Errorf("tombstoning forgotten GalleryItem: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_items WHERE id = ?`, itemID); err != nil {
		return fmt.Errorf("forgetting GalleryItem record: %w", err)
	}
	if err := touchGalleryMetadata(ctx, tx, item.GalleryID, expectedRevision, now); err != nil {
		return err
	}
	if err := demoteInvalidActiveGallery(ctx, tx, item.GalleryID); err != nil {
		return err
	}
	return tx.Commit()
}

// MoveItemWithinGroup places itemID immediately before beforeItemID. A nil
// target appends it to its media group. Gaps normally make this a one-row
// update; when a gap is exhausted only that media group is moved to fresh
// positions above the Gallery-wide high-water mark.
func (s *GalleryStore) MoveItemWithinGroup(
	ctx context.Context,
	itemID int64,
	beforeItemID *int64,
	expectedRevision int64,
	now time.Time,
) (gallery.Item, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.Item{}, err
	}
	defer func() { _ = tx.Rollback() }()

	item, err := findItem(ctx, tx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return gallery.Item{}, ErrGalleryItemNotFound
	}
	if err != nil {
		return gallery.Item{}, err
	}
	group := itemGroup(item)
	ordered, err := loadItemsInGroup(ctx, tx, item.GalleryID, group)
	if err != nil {
		return gallery.Item{}, err
	}

	withoutItem := make([]gallery.Item, 0, len(ordered)-1)
	for _, current := range ordered {
		if current.ID != itemID {
			withoutItem = append(withoutItem, current)
		}
	}
	insertAt := len(withoutItem)
	if beforeItemID != nil {
		target, err := findItem(ctx, tx, *beforeItemID)
		if errors.Is(err, sql.ErrNoRows) {
			return gallery.Item{}, ErrGalleryItemNotFound
		}
		if err != nil {
			return gallery.Item{}, err
		}
		if target.GalleryID != item.GalleryID || itemGroup(target) != group {
			return gallery.Item{}, ErrMoveAcrossMediaGroups
		}
		if target.ID == itemID {
			current, err := findGallery(ctx, tx, item.GalleryID)
			if err != nil {
				return gallery.Item{}, err
			}
			if current.MetadataRevision != expectedRevision {
				return gallery.Item{}, ErrMetadataRevisionConflict
			}
			return item, nil
		}
		for index := range withoutItem {
			if withoutItem[index].ID == target.ID {
				insertAt = index
				break
			}
		}
	}

	reordered := append(withoutItem, gallery.Item{})
	copy(reordered[insertAt+1:], reordered[insertAt:])
	reordered[insertAt] = item
	if sameItemOrder(ordered, reordered) {
		current, err := findGallery(ctx, tx, item.GalleryID)
		if err != nil {
			return gallery.Item{}, err
		}
		if current.MetadataRevision != expectedRevision {
			return gallery.Item{}, ErrMetadataRevisionConflict
		}
		return item, nil
	}

	position, hasGap, err := movePosition(ctx, tx, item.GalleryID, reordered, insertAt)
	if err != nil {
		return gallery.Item{}, err
	}
	if hasGap {
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_items SET position = ?, updated_at_utc = ? WHERE id = ?`,
			position, formatTime(normalisedTime(now)), itemID); err != nil {
			return gallery.Item{}, err
		}
	} else if err := rebalanceItemGroup(ctx, tx, item.GalleryID, reordered, now); err != nil {
		return gallery.Item{}, err
	}
	if err := touchGalleryMetadata(ctx, tx, item.GalleryID, expectedRevision, now); err != nil {
		return gallery.Item{}, err
	}
	updated, err := findItem(ctx, tx, itemID)
	if err != nil {
		return gallery.Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.Item{}, err
	}
	return updated, nil
}

type memberGroup int

const (
	memberGroupPhoto memberGroup = iota
	memberGroupSelfie
	memberGroupAnimated
	memberGroupVideo
)

func itemGroup(item gallery.Item) memberGroup {
	if item.MediaKind == gallery.MediaKindStaticImage {
		if item.ImageCategory == gallery.ImageCategorySelfie {
			return memberGroupSelfie
		}
		return memberGroupPhoto
	}
	if item.MediaKind == gallery.MediaKindAnimatedImage {
		return memberGroupAnimated
	}
	return memberGroupVideo
}

func loadItemsInGroup(ctx context.Context, tx *sql.Tx, galleryID int64, group memberGroup) ([]gallery.Item, error) {
	condition := "media_kind = 'VIDEO'"
	switch group {
	case memberGroupPhoto:
		condition = "media_kind = 'STATIC_IMAGE' AND image_category = 'PHOTO'"
	case memberGroupSelfie:
		condition = "media_kind = 'STATIC_IMAGE' AND image_category = 'SELFIE'"
	case memberGroupAnimated:
		condition = "media_kind = 'ANIMATED_IMAGE'"
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM gallery_items WHERE gallery_id = ? AND `+condition+` ORDER BY position`, galleryID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]gallery.Item, 0, len(ids))
	for _, id := range ids {
		item, err := findItem(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func sameItemOrder(left []gallery.Item, right []gallery.Item) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID {
			return false
		}
	}
	return true
}

func movePosition(ctx context.Context, tx *sql.Tx, galleryID int64, items []gallery.Item, index int) (int64, bool, error) {
	if index == len(items)-1 {
		var highWater int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) FROM gallery_items WHERE gallery_id = ?`, galleryID).Scan(&highWater); err != nil {
			return 0, false, err
		}
		if highWater <= int64(^uint64(0)>>1)-1024 {
			return highWater + 1024, true, nil
		}
		return 0, false, nil
	}
	if index == 0 {
		next := items[1].Position
		if next > 1 {
			return next / 2, true, nil
		}
		return 0, false, nil
	}
	previous := items[index-1].Position
	next := items[index+1].Position
	if next-previous > 1 {
		return previous + (next-previous)/2, true, nil
	}
	return 0, false, nil
}

func rebalanceItemGroup(ctx context.Context, tx *sql.Tx, galleryID int64, items []gallery.Item, now time.Time) error {
	var highWater int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) FROM gallery_items WHERE gallery_id = ?`, galleryID).Scan(&highWater); err != nil {
		return err
	}
	if len(items) > 0 && highWater > int64(^uint64(0)>>1)-int64(len(items)+1)*1024 {
		return errors.New("GalleryItem position space exhausted")
	}
	timestamp := formatTime(normalisedTime(now))
	for index, item := range items {
		position := highWater + int64(index+1)*1024
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_items SET position = ?, updated_at_utc = ? WHERE id = ?`, position, timestamp, item.ID); err != nil {
			return err
		}
	}
	return nil
}
