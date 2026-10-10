package productdb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
)

// DirectImageDescriptor contains the internal source identity needed by the
// authenticated original-image handler. It is never returned by GraphQL.
type DirectImageDescriptor struct {
	ItemUUID        string
	ContentRevision int64
	MediaKind       gallery.MediaKind
	Source          mediaaccess.Source
}

// AuthorizeDirectImage applies the same Browse visibility boundary as cached
// resources, while allowing both DIRECTORY and validated ZIP/CBZ sources.
func (s *MediaResourceStore) AuthorizeDirectImage(ctx context.Context, access ResourceAccess, itemUUID string, revision int64) (DirectImageDescriptor, error) {
	if !access.Authenticated || access.Mode != ResourceBrowse {
		return DirectImageDescriptor{}, ErrMediaResourceForbidden
	}
	var result DirectImageDescriptor
	var sourceType, galleryState, contentRating, sourceAvailability, itemAvailability string
	var excluded, overLimit, hidden, blocking int
	err := s.db.QueryRowContext(ctx, `SELECT item.item_uuid,item.content_revision,item.media_kind,source.source_type,source.source_path,item.relative_path,
		gallery.state,gallery.content_rating,item.excluded,source.over_limit,COALESCE(personal.hidden,0),source.availability_state,item.availability_state,
		(SELECT COUNT(*) FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL)
		FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id JOIN gallery_sources source ON source.id=item.source_id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE item.item_uuid=? AND item.content_revision=? AND item.content_format='IMAGE' AND item.media_kind IN ('STATIC_IMAGE','ANIMATED_IMAGE')`, itemUUID, revision).Scan(
		&result.ItemUUID, &result.ContentRevision, &result.MediaKind, &sourceType, &result.Source.Path, &result.Source.RelativePath,
		&galleryState, &contentRating, &excluded, &overLimit, &hidden, &sourceAvailability, &itemAvailability, &blocking)
	if errors.Is(err, sql.ErrNoRows) {
		return DirectImageDescriptor{}, ErrMediaResourceForbidden
	}
	if err != nil {
		return DirectImageDescriptor{}, err
	}
	result.Source.Type = gallery.SourceType(sourceType)
	if galleryState != string(gallery.StateActive) || sourceAvailability != string(gallery.AvailabilityAvailable) ||
		itemAvailability != string(gallery.AvailabilityAvailable) || excluded == 1 || overLimit == 1 || hidden == 1 ||
		blocking > 0 || !resourceRatingInScope(contentRating, access.Scope) ||
		(result.Source.Type != gallery.SourceTypeDirectory && result.Source.Type != gallery.SourceTypeArchive) {
		return DirectImageDescriptor{}, ErrMediaResourceForbidden
	}
	return result, nil
}
