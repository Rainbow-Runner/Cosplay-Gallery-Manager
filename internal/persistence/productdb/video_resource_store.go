package productdb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

type DirectVideoDescriptor struct {
	ItemUUID        string
	ContentRevision int64
	MIMEType        string
	Source          mediaaccess.Source
}

func (s *MediaResourceStore) AuthorizeDirectVideo(ctx context.Context, access ResourceAccess, itemUUID string, revision int64) (DirectVideoDescriptor, error) {
	if !access.Authenticated || access.Mode != ResourceBrowse {
		return DirectVideoDescriptor{}, ErrMediaResourceForbidden
	}
	var result DirectVideoDescriptor
	var galleryState, sourceAvailability, itemAvailability, sourceType, contentRating string
	var excluded, overLimit, hidden, blocking int
	var metadata mediaprocessing.VideoTechnicalMetadata
	err := s.db.QueryRowContext(ctx, `SELECT item.item_uuid,item.content_revision,source.source_type,source.source_path,item.relative_path,
		gallery.state,gallery.content_rating,item.excluded,source.over_limit,COALESCE(personal.hidden,0),source.availability_state,item.availability_state,
		(SELECT COUNT(*) FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL),
		video.container,video.video_codec,video.audio_codec,video.rotation,video.hdr,video.video_stream_index,video.audio_stream_index,video.display_width,video.display_height,video.probe_state,video.content_revision
		FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id JOIN gallery_sources source ON source.id=item.source_id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id JOIN video_technical_metadata video ON video.item_uuid=item.item_uuid
		WHERE item.item_uuid=? AND item.media_kind='VIDEO' AND item.content_revision=?`, itemUUID, revision).Scan(&result.ItemUUID, &result.ContentRevision, &sourceType, &result.Source.Path, &result.Source.RelativePath,
		&galleryState, &contentRating, &excluded, &overLimit, &hidden, &sourceAvailability, &itemAvailability, &blocking, &metadata.Container, &metadata.VideoCodec, &metadata.AudioCodec, &metadata.Rotation, &metadata.HDR,
		&metadata.VideoStreamIndex, &metadata.AudioStreamIndex, &metadata.DisplayWidth, &metadata.DisplayHeight, &metadata.ProbeState, &metadata.ContentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return DirectVideoDescriptor{}, ErrMediaResourceForbidden
	}
	if err != nil {
		return DirectVideoDescriptor{}, err
	}
	result.Source.Type = gallery.SourceType(sourceType)
	if galleryState != string(gallery.StateActive) || sourceAvailability != string(gallery.AvailabilityAvailable) || itemAvailability != string(gallery.AvailabilityAvailable) || excluded == 1 || overLimit == 1 || hidden == 1 || blocking > 0 || !resourceRatingInScope(contentRating, access.Scope) || result.Source.Type != gallery.SourceTypeDirectory || metadata.ProbeState != mediaprocessing.VideoProbeReady || metadata.ContentRevision != revision || mediaprocessing.PlaybackPlanFromMetadata(metadata).Mode != mediaprocessing.PlaybackDirect {
		return DirectVideoDescriptor{}, ErrMediaResourceForbidden
	}
	result.MIMEType = "video/mp4"
	return result, nil
}
