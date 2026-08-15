package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/product"
)

type videoPlaybackIdentity struct {
	galleryID int64
	revision  int64
	metadata  mediaprocessing.VideoTechnicalMetadata
}

func (s *BrowseStore) VideoPlaybackStatus(ctx context.Context, itemUUID, ffmpegVersion, ffmpegUnavailableCode string) (browse.VideoPlaybackStatus, error) {
	identity, err := s.authorizeVideoPlayback(ctx, itemUUID)
	if err != nil {
		return browse.VideoPlaybackStatus{}, err
	}
	return s.videoPlaybackStatus(ctx, itemUUID, identity, ffmpegVersion, ffmpegUnavailableCode)
}

func (s *BrowseStore) RequestVideoPlayback(ctx context.Context, itemUUID, ffmpegVersion, ffmpegUnavailableCode string, now time.Time) (browse.VideoPlaybackStatus, error) {
	identity, err := s.authorizeVideoPlayback(ctx, itemUUID)
	if err != nil {
		return browse.VideoPlaybackStatus{}, err
	}
	status, err := s.videoPlaybackStatus(ctx, itemUUID, identity, ffmpegVersion, ffmpegUnavailableCode)
	if err != nil || status.Status == gallery.ProcessingReady || status.Status == gallery.ProcessingError ||
		identity.metadata.ProbeState != mediaprocessing.VideoProbeReady || identity.metadata.ContentRevision != identity.revision {
		return status, err
	}
	plan := mediaprocessing.PlaybackPlanFromMetadata(identity.metadata)
	if plan.Mode == mediaprocessing.PlaybackDirect {
		return status, nil
	}
	if ffmpegVersion == "" {
		return status, nil
	}
	profile := videoPlaybackProfile(identity.metadata, plan, ffmpegVersion)
	key := ItemDerivativeJobKey(itemUUID, mediaprocessing.VariantVideoPlayback, identity.revision, profile)
	jobs := &ProcessingJobStore{db: s.db}
	job, err := jobs.FindByKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		revision := identity.revision
		_, err = jobs.Enqueue(ctx, EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemDerivative, GalleryID: &identity.galleryID, ItemUUID: itemUUID, Variant: mediaprocessing.VariantVideoPlayback, ContentRevision: &revision, ProfileHash: profile, Payload: map[string]any{"cache_tier": mediaprocessing.CacheEnhanced, "playback_mode": plan.Mode}, Priority: 600}, now)
	} else if err == nil && (job.Status == mediaprocessing.JobCompleted || job.Status == mediaprocessing.JobFailed || job.Status == mediaprocessing.JobCancelled) {
		_, err = jobs.Requeue(ctx, key, 600, now)
		if err != nil {
			if current, findErr := jobs.FindByKey(ctx, key); findErr == nil && (current.Status == mediaprocessing.JobPending || current.Status == mediaprocessing.JobRunning || current.Status == mediaprocessing.JobRetryWait) {
				err = nil
			}
		}
	}
	if err != nil {
		return browse.VideoPlaybackStatus{}, err
	}
	return s.videoPlaybackStatus(ctx, itemUUID, identity, ffmpegVersion, ffmpegUnavailableCode)
}

func (s *BrowseStore) authorizeVideoPlayback(ctx context.Context, itemUUID string) (videoPlaybackIdentity, error) {
	var result videoPlaybackIdentity
	err := s.db.QueryRowContext(ctx, `SELECT gallery.id,item.content_revision FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id JOIN gallery_sources source ON source.id=item.source_id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE item.item_uuid=? AND item.media_kind='VIDEO' AND item.excluded=0
		AND item.availability_state='AVAILABLE' AND source.source_type='DIRECTORY' AND source.availability_state='AVAILABLE' AND source.over_limit=0
		AND NOT EXISTS(SELECT 1 FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL)
		AND `+browseVisibleGalleryPredicate, itemUUID).Scan(&result.galleryID, &result.revision)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrBrowseGalleryNotVisible
	}
	if err != nil {
		return result, err
	}
	result.metadata, err = (&VideoMetadataStore{db: s.db}).Find(ctx, itemUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if result.metadata.ContentRevision != result.revision {
		return result, nil
	}
	return result, nil
}

func (s *BrowseStore) videoPlaybackStatus(ctx context.Context, itemUUID string, identity videoPlaybackIdentity, ffmpegVersion, ffmpegUnavailableCode string) (browse.VideoPlaybackStatus, error) {
	result := browse.VideoPlaybackStatus{ItemUUID: itemUUID, ContentRevision: identity.revision, Status: gallery.ProcessingPending}
	if identity.metadata.ProbeState == mediaprocessing.VideoProbeError {
		result.Status = gallery.ProcessingError
		result.ErrorCode = identity.metadata.LastErrorCode
		return result, nil
	}
	if identity.metadata.ProbeState != mediaprocessing.VideoProbeReady || identity.metadata.ContentRevision != identity.revision {
		return result, nil
	}
	plan := mediaprocessing.PlaybackPlanFromMetadata(identity.metadata)
	result.Mode = string(plan.Mode)
	if plan.Mode == mediaprocessing.PlaybackDirect {
		result.Status = gallery.ProcessingReady
		return result, nil
	}
	if ffmpegVersion == "" {
		result.Status = gallery.ProcessingError
		result.ErrorCode = ffmpegUnavailableCode
		if result.ErrorCode == "" {
			result.ErrorCode = mediaprocessing.ErrorFFmpegUnavailable
		}
		return result, nil
	}
	profile := videoPlaybackProfile(identity.metadata, plan, ffmpegVersion)
	var resource browse.ResourceIdentity
	err := s.db.QueryRowContext(ctx, `SELECT item_uuid,content_revision,profile_hash,variant,mime_type FROM media_derivatives WHERE item_uuid=? AND variant=? AND content_revision=? AND profile_hash=? AND is_current=1 AND state IN ('READY','STALE')`, itemUUID, mediaprocessing.VariantVideoPlayback, identity.revision, profile).Scan(&resource.ItemUUID, &resource.ContentRevision, &resource.ProfileHash, &resource.Variant, &resource.MIMEType)
	if err == nil {
		result.Status = gallery.ProcessingReady
		result.Resource = &resource
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	job, err := (&ProcessingJobStore{db: s.db}).FindByKey(ctx, ItemDerivativeJobKey(itemUUID, mediaprocessing.VariantVideoPlayback, identity.revision, profile))
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	switch job.Status {
	case mediaprocessing.JobRunning:
		result.Status = gallery.ProcessingProcessing
	case mediaprocessing.JobFailed, mediaprocessing.JobCancelled:
		result.Status = gallery.ProcessingError
		result.ErrorCode = job.LastErrorCode
	}
	return result, nil
}

func videoPlaybackProfile(metadata mediaprocessing.VideoTechnicalMetadata, plan mediaprocessing.VideoPlaybackPlan, ffmpegVersion string) string {
	value, _ := (mediaprocessing.Profile{ContractVersion: product.MediaProcessingProfileVersion, Generator: "cgm-video-playback", GeneratorVersion: "1", DependencyVersion: ffmpegVersion,
		Configuration: map[string]any{"matrix": "common-browser-v1", "mode": plan.Mode, "video_track": plan.SelectVideoTrack, "audio_track": plan.SelectAudioTrack,
			"source_video_codec": metadata.VideoCodec, "source_audio_codec": metadata.AudioCodec, "copy_video": plan.CopyVideo, "copy_audio": plan.CopyAudio,
			"target_container": "mp4", "target_video": "h264", "target_audio": "aac-or-none", "maximum_width": plan.MaximumWidth, "maximum_height": plan.MaximumHeight,
			"rotation": metadata.Rotation, "hdr_to_sdr": metadata.HDR, "video_encoding": "libx264-medium-crf20-high-yuv420p", "audio_encoding": "aac-192k", "faststart": true}}).Hash()
	return value
}
