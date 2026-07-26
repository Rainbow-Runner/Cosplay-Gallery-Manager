package productdb

import (
	"context"

	"github.com/stashapp/stash/internal/gallery"
)

// ProcessingItem is an internal worker DTO. Physical paths must never be
// returned by Browse or Manage resource DTOs.
type ProcessingItem struct {
	ItemUUID        string
	GalleryID       int64
	SourceType      gallery.SourceType
	SourcePath      string
	RelativePath    string
	MediaKind       gallery.MediaKind
	ContentFormat   gallery.ContentFormat
	ContentRevision int64
	Availability    gallery.AvailabilityState
	Excluded        bool
}

func (db *Database) FindProcessingItem(ctx context.Context, itemUUID string) (ProcessingItem, error) {
	var result ProcessingItem
	var excluded int
	err := db.QueryRowContext(ctx, `SELECT item.item_uuid,item.gallery_id,source.source_type,source.source_path,
		item.relative_path,item.media_kind,item.content_format,item.content_revision,item.availability_state,item.excluded
		FROM gallery_items item JOIN gallery_sources source ON source.id=item.source_id WHERE item.item_uuid=?`, itemUUID).Scan(
		&result.ItemUUID, &result.GalleryID, &result.SourceType, &result.SourcePath, &result.RelativePath,
		&result.MediaKind, &result.ContentFormat, &result.ContentRevision, &result.Availability, &excluded)
	result.Excluded = excluded == 1
	return result, err
}
