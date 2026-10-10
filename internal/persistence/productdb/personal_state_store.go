package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type GalleryPersonalState struct {
	GalleryID       int64
	Favorite        bool
	FavoritedAtUTC  *time.Time
	RatingHalfSteps *int
	RatedAtUTC      *time.Time
	Hidden          bool
	LastViewedAtUTC *time.Time
	LastItemID      *int64
}

type GalleryItemPersonalState struct {
	GalleryItemID   int64
	Favorite        bool
	FavoritedAtUTC  *time.Time
	RatingHalfSteps *int
	RatedAtUTC      *time.Time
}

type PersonalStateStore struct {
	db *sql.DB
}

func (db *Database) PersonalStates() *PersonalStateStore {
	return &PersonalStateStore{db: db.DB}
}

func (s *PersonalStateStore) SetGalleryFavorite(
	ctx context.Context,
	galleryID int64,
	favorite bool,
	now time.Time,
) error {
	var favoritedAt interface{}
	if favorite {
		favoritedAt = formatTime(normalisedTime(now))
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_personal_states (gallery_id, favorite, favorited_at_utc)
		VALUES (?, ?, ?)
		ON CONFLICT(gallery_id) DO UPDATE SET
			favorite = excluded.favorite,
			favorited_at_utc = excluded.favorited_at_utc
	`, galleryID, favorite, favoritedAt)
	return err
}

func (s *PersonalStateStore) SetGalleryHidden(ctx context.Context, galleryID int64, hidden bool) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_personal_states (gallery_id, hidden)
		VALUES (?, ?)
		ON CONFLICT(gallery_id) DO UPDATE SET hidden = excluded.hidden
	`, galleryID, hidden)
	return err
}

func (s *PersonalStateStore) SetGalleryRating(
	ctx context.Context,
	galleryID int64,
	ratingHalfSteps *int,
	expectedMetadataRevision int64,
	now time.Time,
) error {
	if err := validateRating(ratingHalfSteps); err != nil {
		return err
	}
	return s.updateRating(ctx, galleryID, 0, ratingHalfSteps, expectedMetadataRevision, now)
}

func (s *PersonalStateStore) SetItemFavorite(
	ctx context.Context,
	itemID int64,
	favorite bool,
	now time.Time,
) error {
	var favoritedAt interface{}
	if favorite {
		favoritedAt = formatTime(normalisedTime(now))
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_item_personal_states (
			gallery_item_id, favorite, favorited_at_utc
		) VALUES (?, ?, ?)
		ON CONFLICT(gallery_item_id) DO UPDATE SET
			favorite = excluded.favorite,
			favorited_at_utc = excluded.favorited_at_utc
	`, itemID, favorite, favoritedAt)
	return err
}

func (s *PersonalStateStore) SetItemRating(
	ctx context.Context,
	itemID int64,
	ratingHalfSteps *int,
	expectedGalleryMetadataRevision int64,
	now time.Time,
) error {
	if err := validateRating(ratingHalfSteps); err != nil {
		return err
	}
	return s.updateRating(ctx, 0, itemID, ratingHalfSteps, expectedGalleryMetadataRevision, now)
}

func (s *PersonalStateStore) updateRating(
	ctx context.Context,
	galleryID int64,
	itemID int64,
	ratingHalfSteps *int,
	expectedRevision int64,
	now time.Time,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if itemID != 0 {
		if err := tx.QueryRowContext(ctx, `
			SELECT gallery_id FROM gallery_items WHERE id = ?
		`, itemID).Scan(&galleryID); err != nil {
			return err
		}
	}

	var ratedAt interface{}
	if ratingHalfSteps != nil {
		ratedAt = formatTime(normalisedTime(now))
	}
	if itemID == 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_personal_states (
				gallery_id, rating_half_steps, rated_at_utc
			) VALUES (?, ?, ?)
			ON CONFLICT(gallery_id) DO UPDATE SET
				rating_half_steps = excluded.rating_half_steps,
				rated_at_utc = excluded.rated_at_utc
		`, galleryID, ratingHalfSteps, ratedAt); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_item_personal_states (
				gallery_item_id, rating_half_steps, rated_at_utc
			) VALUES (?, ?, ?)
			ON CONFLICT(gallery_item_id) DO UPDATE SET
				rating_half_steps = excluded.rating_half_steps,
				rated_at_utc = excluded.rated_at_utc
		`, itemID, ratingHalfSteps, ratedAt); err != nil {
			return err
		}
	}

	if err := touchGalleryMetadata(ctx, tx, galleryID, expectedRevision, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PersonalStateStore) RecordGalleryView(
	ctx context.Context,
	galleryID int64,
	lastItemID *int64,
	now time.Time,
) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_personal_states (
			gallery_id, last_viewed_at_utc, last_item_id
		) VALUES (?, ?, ?)
		ON CONFLICT(gallery_id) DO UPDATE SET
			last_viewed_at_utc = excluded.last_viewed_at_utc,
			last_item_id = excluded.last_item_id
	`, galleryID, formatTime(normalisedTime(now)), lastItemID)
	return err
}

func (s *PersonalStateStore) UpdateLightboxItem(ctx context.Context, galleryID int64, lastItemID int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_personal_states (gallery_id, last_item_id)
		VALUES (?, ?)
		ON CONFLICT(gallery_id) DO UPDATE SET last_item_id = excluded.last_item_id
	`, galleryID, lastItemID)
	return err
}

func (s *PersonalStateStore) ClearGalleryHistory(ctx context.Context, galleryID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE gallery_personal_states
		SET last_viewed_at_utc = NULL, last_item_id = NULL
		WHERE gallery_id = ?
	`, galleryID)
	return err
}

func (s *PersonalStateStore) Gallery(ctx context.Context, galleryID int64) (GalleryPersonalState, error) {
	var (
		result     GalleryPersonalState
		favorite   int
		hidden     int
		favorited  sql.NullString
		rating     sql.NullInt64
		rated      sql.NullString
		lastViewed sql.NullString
		lastItem   sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT gallery_id, favorite, favorited_at_utc, rating_half_steps,
			rated_at_utc, hidden, last_viewed_at_utc, last_item_id
		FROM gallery_personal_states WHERE gallery_id = ?
	`, galleryID).Scan(
		&result.GalleryID, &favorite, &favorited, &rating, &rated,
		&hidden, &lastViewed, &lastItem,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return GalleryPersonalState{GalleryID: galleryID}, nil
	}
	if err != nil {
		return GalleryPersonalState{}, err
	}
	result.Favorite = favorite == 1
	result.Hidden = hidden == 1
	if err := assignOptionalTime(favorited, &result.FavoritedAtUTC); err != nil {
		return GalleryPersonalState{}, err
	}
	if err := assignOptionalTime(rated, &result.RatedAtUTC); err != nil {
		return GalleryPersonalState{}, err
	}
	if err := assignOptionalTime(lastViewed, &result.LastViewedAtUTC); err != nil {
		return GalleryPersonalState{}, err
	}
	if rating.Valid {
		value := int(rating.Int64)
		result.RatingHalfSteps = &value
	}
	if lastItem.Valid {
		value := lastItem.Int64
		result.LastItemID = &value
	}
	return result, nil
}

func validateRating(value *int) error {
	if value != nil && (*value < 1 || *value > 10) {
		return fmt.Errorf("rating_half_steps must be null or between 1 and 10, found %d", *value)
	}
	return nil
}

func assignOptionalTime(value sql.NullString, target **time.Time) error {
	if !value.Valid {
		return nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return err
	}
	*target = &parsed
	return nil
}
