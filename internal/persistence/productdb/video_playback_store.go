package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

type VideoPlaybackRuntime struct {
	FFmpegVersion         string
	FFmpegUnavailableCode string
	Preference            mediaprocessing.VideoHardwarePreference
	Hardware              mediaprocessing.HardwareAccelerationStatus
}

func softwareVideoPlaybackRuntime(version, unavailable string) VideoPlaybackRuntime {
	return VideoPlaybackRuntime{FFmpegVersion: version, FFmpegUnavailableCode: unavailable,
		Preference: mediaprocessing.VideoHardwarePreference{Mode: "SOFTWARE", AllowSoftwareFallback: true}}
}

type videoPlaybackIdentity struct {
	galleryID int64
	revision  int64
	metadata  mediaprocessing.VideoTechnicalMetadata
	source    mediaaccess.Source
	sourceID  int64
}

func (s *BrowseStore) VideoPlaybackStatus(ctx context.Context, itemUUID, ffmpegVersion, ffmpegUnavailableCode string) (browse.VideoPlaybackStatus, error) {
	return s.VideoPlaybackStatusWithRuntime(ctx, itemUUID, softwareVideoPlaybackRuntime(ffmpegVersion, ffmpegUnavailableCode))
}

func (s *BrowseStore) VideoPlaybackStatusWithRuntime(ctx context.Context, itemUUID string, runtime VideoPlaybackRuntime) (browse.VideoPlaybackStatus, error) {
	identity, err := s.authorizeVideoPlayback(ctx, itemUUID)
	if err != nil {
		return browse.VideoPlaybackStatus{}, err
	}
	return s.videoPlaybackStatus(ctx, itemUUID, identity, runtime)
}

func (s *BrowseStore) RequestVideoPlayback(ctx context.Context, itemUUID, ffmpegVersion, ffmpegUnavailableCode string, now time.Time) (browse.VideoPlaybackStatus, error) {
	return s.RequestVideoPlaybackWithRuntime(ctx, itemUUID, softwareVideoPlaybackRuntime(ffmpegVersion, ffmpegUnavailableCode), now)
}

func (s *BrowseStore) RequestVideoPlaybackWithRuntime(ctx context.Context, itemUUID string, runtime VideoPlaybackRuntime, now time.Time) (browse.VideoPlaybackStatus, error) {
	identity, err := s.authorizeVideoPlayback(ctx, itemUUID)
	if err != nil {
		return browse.VideoPlaybackStatus{}, err
	}
	status, err := s.videoPlaybackStatus(ctx, itemUUID, identity, runtime)
	if err != nil || status.Status == gallery.ProcessingReady || status.Status == gallery.ProcessingError ||
		identity.metadata.ProbeState != mediaprocessing.VideoProbeReady || identity.metadata.ContentRevision != identity.revision {
		return status, err
	}
	plan := playbackPlan(identity)
	if plan.Mode == mediaprocessing.PlaybackDirect {
		return status, nil
	}
	if runtime.FFmpegVersion == "" {
		return status, nil
	}
	execution, code := completeVideoExecution(plan, identity.metadata, runtime)
	if code != "" {
		status.Status, status.ErrorCode = gallery.ProcessingError, code
		return status, nil
	}
	profile := mediaprocessing.CompleteVideoProfileHash(identity.metadata, plan, runtime.FFmpegVersion, execution)
	key := ItemDerivativeJobKey(itemUUID, mediaprocessing.VariantVideoPlayback, identity.revision, profile)
	jobs := &ProcessingJobStore{db: s.db}
	job, err := jobs.FindByKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		revision := identity.revision
		_, err = jobs.Enqueue(ctx, EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemDerivative, GalleryID: &identity.galleryID, ItemUUID: itemUUID, Variant: mediaprocessing.VariantVideoPlayback, ContentRevision: &revision, ProfileHash: profile, Payload: map[string]any{"cache_tier": mediaprocessing.CacheEnhanced, "playback_mode": plan.Mode, "execution": execution}, Priority: 600}, now)
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
	return s.videoPlaybackStatus(ctx, itemUUID, identity, runtime)
}

func (s *BrowseStore) authorizeVideoPlayback(ctx context.Context, itemUUID string) (videoPlaybackIdentity, error) {
	var result videoPlaybackIdentity
	err := s.db.QueryRowContext(ctx, `SELECT gallery.id,item.content_revision,source.id,source.source_type,source.source_path,item.relative_path FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id JOIN gallery_sources source ON source.id=item.source_id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE item.item_uuid=? AND item.media_kind='VIDEO' AND item.excluded=0
		AND item.availability_state='AVAILABLE' AND source.source_type IN ('DIRECTORY','ARCHIVE') AND source.availability_state='AVAILABLE' AND source.over_limit=0
		AND NOT EXISTS(SELECT 1 FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL)
		AND `+browseVisibleGalleryPredicate, itemUUID).Scan(&result.galleryID, &result.revision, &result.sourceID, &result.source.Type, &result.source.Path, &result.source.RelativePath)
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

func (s *BrowseStore) videoPlaybackStatus(ctx context.Context, itemUUID string, identity videoPlaybackIdentity, runtime VideoPlaybackRuntime) (browse.VideoPlaybackStatus, error) {
	result := browse.VideoPlaybackStatus{ItemUUID: itemUUID, ContentRevision: identity.revision, Status: gallery.ProcessingPending}
	if identity.metadata.ContentRevision != identity.revision {
		return s.archiveProbePendingStatus(ctx, result, identity)
	}
	if identity.metadata.ProbeState == mediaprocessing.VideoProbeError {
		result.Status = gallery.ProcessingError
		result.ErrorCode = identity.metadata.LastErrorCode
		return result, nil
	}
	if identity.metadata.ProbeState != mediaprocessing.VideoProbeReady || identity.metadata.ContentRevision != identity.revision {
		return s.archiveProbePendingStatus(ctx, result, identity)
	}
	plan := playbackPlan(identity)
	result.Mode = string(plan.Mode)
	if identity.source.Type == gallery.SourceTypeArchive {
		limits, evidence, err := (&Database{DB: s.db}).ArchiveAccessEvidence(ctx, identity.sourceID)
		if err == nil {
			var member *mediaaccess.DirectArchiveMember
			member, err = mediaaccess.OpenArchiveMember(ctx, identity.source, limits, evidence)
			if member != nil {
				_ = member.Close()
			}
		}
		if err != nil {
			result.Status, result.Mode, result.ErrorCode = gallery.ProcessingError, "", mediaaccess.ArchiveVideoErrorCode(err)
			return result, nil
		}
		if plan.Mode == mediaprocessing.PlaybackDirect {
			result.Status = gallery.ProcessingReady
			return result, nil
		}
	}
	if plan.Mode == mediaprocessing.PlaybackDirect {
		result.Status = gallery.ProcessingReady
		return result, nil
	}
	if runtime.FFmpegVersion == "" {
		result.Status = gallery.ProcessingError
		result.ErrorCode = runtime.FFmpegUnavailableCode
		if result.ErrorCode == "" {
			result.ErrorCode = mediaprocessing.ErrorFFmpegUnavailable
		}
		return result, nil
	}
	execution, code := completeVideoExecution(plan, identity.metadata, runtime)
	if code != "" {
		result.Status, result.ErrorCode = gallery.ProcessingError, code
		return result, nil
	}
	profile := mediaprocessing.CompleteVideoProfileHash(identity.metadata, plan, runtime.FFmpegVersion, execution)
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

func playbackPlan(identity videoPlaybackIdentity) mediaprocessing.VideoPlaybackPlan {
	if identity.source.Type == gallery.SourceTypeArchive {
		return mediaprocessing.ArchivePlaybackPlanFromMetadata(identity.metadata)
	}
	return mediaprocessing.PlaybackPlanFromMetadata(identity.metadata)
}

func (s *BrowseStore) archiveProbePendingStatus(ctx context.Context, result browse.VideoPlaybackStatus, identity videoPlaybackIdentity) (browse.VideoPlaybackStatus, error) {
	if identity.source.Type != gallery.SourceTypeArchive {
		return result, nil
	}
	var status, code string
	err := s.db.QueryRowContext(ctx, `SELECT status,last_error_code FROM processing_jobs WHERE item_uuid=? AND content_revision=? AND job_kind='ITEM_TECHNICAL_METADATA' AND variant=''
		ORDER BY CASE WHEN status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED') THEN 0 ELSE 1 END,id DESC LIMIT 1`, result.ItemUUID, identity.revision).Scan(&status, &code)
	if errors.Is(err, sql.ErrNoRows) {
		result.Status, result.ErrorCode = gallery.ProcessingError, "VIDEO_PROBE_UNAVAILABLE"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	switch mediaprocessing.JobStatus(status) {
	case mediaprocessing.JobRunning:
		result.Status = gallery.ProcessingProcessing
	case mediaprocessing.JobFailed, mediaprocessing.JobCancelled, mediaprocessing.JobCompleted:
		result.Status, result.ErrorCode = gallery.ProcessingError, code
		if result.ErrorCode == "" {
			result.ErrorCode = "VIDEO_PROBE_UNAVAILABLE"
		}
	}
	return result, nil
}

func completeVideoExecution(plan mediaprocessing.VideoPlaybackPlan, metadata mediaprocessing.VideoTechnicalMetadata, runtime VideoPlaybackRuntime) (mediaprocessing.VideoTranscodeExecutionPlan, string) {
	execution := mediaprocessing.PlanVideoTranscode(plan, metadata, runtime.Preference, runtime.Hardware, mediaprocessing.VideoTranscodeMP4)
	if execution.EffectiveBackend == "VAAPI" {
		if !runtime.Preference.AllowSoftwareFallback {
			return execution, "VIDEO_HARDWARE_BACKEND_NOT_IMPLEMENTED"
		}
		execution = mediaprocessing.SoftwareVideoTranscodePlan(plan, metadata, mediaprocessing.VideoTranscodeMP4)
		execution.ReasonCode = "HARDWARE_EXECUTOR_NOT_IMPLEMENTED"
	}
	if !execution.Executable {
		return execution, "VIDEO_HARDWARE_UNAVAILABLE"
	}
	return execution, ""
}
