package productdb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/stashapp/stash/internal/gallery"
)

func findSource(ctx context.Context, queryer galleryQueryer, id int64) (gallery.Source, error) {
	var (
		result    gallery.Source
		libraryID sql.NullInt64
		overLimit int
		createdAt string
		updatedAt string
	)
	err := queryer.QueryRowContext(ctx, `
		SELECT id, gallery_id, library_id, source_type, source_path, availability_state,
			reconcile_state, over_limit, created_at_utc, updated_at_utc
		FROM gallery_sources WHERE id = ?
	`, id).Scan(
		&result.ID, &result.GalleryID, &libraryID, &result.Type, &result.Path,
		&result.Availability, &result.ReconcileState, &overLimit,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return gallery.Source{}, ErrGalleryNotFound
	}
	if err != nil {
		return gallery.Source{}, err
	}
	result.OverLimit = overLimit == 1
	if libraryID.Valid {
		value := libraryID.Int64
		result.LibraryID = &value
	}
	if result.CreatedAtUTC, err = parseTime(createdAt); err != nil {
		return gallery.Source{}, err
	}
	if result.UpdatedAtUTC, err = parseTime(updatedAt); err != nil {
		return gallery.Source{}, err
	}
	return result, nil
}

func findItem(ctx context.Context, queryer galleryQueryer, id int64) (gallery.Item, error) {
	var (
		result        gallery.Item
		imageCategory sql.NullString
		excluded      int
		createdAt     string
		updatedAt     string
	)
	err := queryer.QueryRowContext(ctx, `
		SELECT id, item_uuid, gallery_id, source_id, relative_path, media_kind,
			content_format, image_category, position, caption, excluded, availability_state,
			processing_state, byte_size, quick_fingerprint, full_fingerprint,
			content_revision, created_at_utc, updated_at_utc
		FROM gallery_items WHERE id = ?
	`, id).Scan(
		&result.ID, &result.UUID, &result.GalleryID, &result.SourceID,
		&result.RelativePath, &result.MediaKind, &result.ContentFormat, &imageCategory, &result.Position,
		&result.Caption, &excluded, &result.Availability, &result.ProcessingState,
		&result.ByteSize, &result.QuickFingerprint, &result.FullFingerprint,
		&result.ContentRevision, &createdAt, &updatedAt,
	)
	if err != nil {
		return gallery.Item{}, err
	}
	result.ImageCategory = gallery.ImageCategory(imageCategory.String)
	result.Excluded = excluded == 1
	if result.CreatedAtUTC, err = parseTime(createdAt); err != nil {
		return gallery.Item{}, err
	}
	if result.UpdatedAtUTC, err = parseTime(updatedAt); err != nil {
		return gallery.Item{}, err
	}
	return result, nil
}
