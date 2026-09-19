package processingworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/imagemetadata"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"golang.org/x/sys/unix"
)

type Worker struct {
	Database              *productdb.Database
	Materializer          mediaaccess.Materializer
	Cache                 mediaprocessing.CacheWriter
	Generators            []mediaprocessing.Generator
	VideoProbe            mediaprocessing.VideoProbe
	CaptureProbe          mediaprocessing.ProbeAdapter
	ProbeProfileHash      string
	PosterProfileHash     string
	ProbeUnavailableCode  string
	FFmpegUnavailableCode string
}

func (worker Worker) RunOne(ctx context.Context, owner string, lease time.Duration, now time.Time) (mediaprocessing.Job, error) {
	if worker.Database == nil {
		return mediaprocessing.Job{}, errors.New("processing worker database is required")
	}
	job, err := worker.Database.ProcessingJobs().ClaimNext(ctx, owner, lease, now)
	if err != nil {
		return mediaprocessing.Job{}, err
	}
	if job.Kind != mediaprocessing.JobItemDerivative && job.Kind != mediaprocessing.JobItemTechnicalMetadata {
		_, failErr := worker.Database.ProcessingJobs().Fail(ctx, job.ID, owner, "UNSUPPORTED_JOB_KIND", true, now)
		if failErr != nil {
			return job, failErr
		}
		return job, errors.New("worker only handles Item derivative jobs")
	}
	processingContext, cancel := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	interval := lease / 3
	if interval < time.Second {
		interval = time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-processingContext.Done():
				heartbeatDone <- nil
				return
			case tick := <-ticker.C:
				if err := worker.Database.ProcessingJobs().Heartbeat(processingContext, job.ID, owner, lease, tick); err != nil {
					heartbeatDone <- err
					cancel()
					return
				}
			}
		}
	}()
	var processErr error
	if job.Kind == mediaprocessing.JobItemTechnicalMetadata {
		if job.Variant == productdb.CaptureDateVariant {
			processErr = worker.processCaptureDate(processingContext, job)
		} else {
			processErr = worker.processVideoProbe(processingContext, job, now)
		}
	} else {
		processErr = worker.processDerivative(processingContext, job)
	}
	cancel()
	heartbeatErr := <-heartbeatDone
	if processErr == nil && heartbeatErr != nil {
		processErr = heartbeatErr
	}
	if processErr != nil {
		code := ErrorCode(processErr)
		structural := errors.Is(processErr, ErrUnsupportedGeneration) || errors.Is(processErr, ErrStaleContent) || code == mediaprocessing.ErrorFFprobeUnavailable || code == mediaprocessing.ErrorFFprobeVersionUnsupported || code == mediaprocessing.ErrorFFmpegUnavailable || code == mediaprocessing.ErrorFFmpegVersionUnsupported || code == mediaprocessing.ErrorVideoTrackMissing
		_, failErr := worker.Database.ProcessingJobs().Fail(ctx, job.ID, owner, code, structural, time.Now())
		if failErr != nil {
			return job, fmt.Errorf("processing failed (%v) and recording failure failed: %w", processErr, failErr)
		}
		return job, processErr
	}
	if err := worker.Database.ProcessingJobs().Complete(ctx, job.ID, owner, time.Now()); err != nil {
		return job, err
	}
	return job, nil
}

func (worker Worker) processCaptureDate(ctx context.Context, job mediaprocessing.Job) error {
	item, err := worker.Database.FindProcessingItem(ctx, job.ItemUUID)
	if err != nil {
		return err
	}
	if job.ContentRevision == nil || item.ContentRevision != *job.ContentRevision || item.Excluded || item.Availability != gallery.AvailabilityAvailable || (item.MediaKind != gallery.MediaKindStaticImage && item.MediaKind != gallery.MediaKindVideo) {
		return ErrStaleContent
	}
	materialized, err := worker.Materializer.Open(ctx, mediaaccess.Source{Type: item.SourceType, Path: item.SourcePath, RelativePath: item.RelativePath})
	if err != nil {
		return err
	}
	defer materialized.Close()
	var date, tag string
	if item.MediaKind == gallery.MediaKindStaticImage {
		metadata, err := imagemetadata.ExtractPath(materialized.Path)
		if err != nil {
			return err
		}
		for _, entry := range metadata.Entries {
			if entry.Key == "exif.DateTimeOriginal" {
				for _, layout := range []string{"2006:01:02 15:04:05", "2006-01-02 15:04:05"} {
					parsed, parseErr := time.Parse(layout, entry.Value)
					if parseErr == nil && parsed.Year() >= 1900 && parsed.Year() <= 2100 {
						date = parsed.Format("2006-01-02")
						tag = "exif.DateTimeOriginal"
						break
					}
				}
				break
			}
		}
	} else {
		timezone, err := worker.Database.CaptureDates().Timezone(ctx, item.ItemUUID)
		if err != nil {
			return err
		}
		date, tag, err = worker.CaptureProbe.CaptureDate(ctx, materialized.Path, timezone)
		if err != nil {
			return err
		}
	}
	return worker.Database.CaptureDates().Publish(ctx, item.ItemUUID, item.ContentRevision, date, tag, time.Now())
}

func (worker Worker) processVideoProbe(ctx context.Context, job mediaprocessing.Job, now time.Time) error {
	item, err := worker.Database.FindProcessingItem(ctx, job.ItemUUID)
	if err != nil {
		return err
	}
	if job.ContentRevision == nil || item.ContentRevision != *job.ContentRevision || item.MediaKind != gallery.MediaKindVideo || item.SourceType != gallery.SourceTypeDirectory || item.Excluded || item.Availability != gallery.AvailabilityAvailable {
		return ErrStaleContent
	}
	if worker.ProbeProfileHash != "" && job.ProfileHash != worker.ProbeProfileHash {
		if err := worker.Database.VideoMetadata().MarkPending(ctx, item.ItemUUID, item.ContentRevision, worker.ProbeProfileHash); err != nil {
			return err
		}
		revision := item.ContentRevision
		_, err := worker.Database.ProcessingJobs().Enqueue(ctx, productdb.EnqueueJobInput{Key: productdb.ItemTechnicalMetadataJobKey(item.ItemUUID, revision, worker.ProbeProfileHash),
			Kind: mediaprocessing.JobItemTechnicalMetadata, GalleryID: &item.GalleryID, ItemUUID: item.ItemUUID, ContentRevision: &revision,
			ProfileHash: worker.ProbeProfileHash, Payload: map[string]any{}, Priority: job.Priority}, now)
		return err
	}
	if worker.VideoProbe == nil {
		code := worker.ProbeUnavailableCode
		if code == "" {
			code = mediaprocessing.ErrorFFprobeUnavailable
		}
		err := &mediaprocessing.VideoProcessingError{Code: code, Err: errors.New("ffprobe is unavailable")}
		_ = worker.Database.VideoMetadata().PublishError(ctx, item.ItemUUID, item.ContentRevision, job.ProfileHash, mediaprocessing.VideoErrorCode(err), time.Now())
		return err
	}
	materialized, err := worker.Materializer.Open(ctx, mediaaccess.Source{Type: item.SourceType, Path: item.SourcePath, RelativePath: item.RelativePath})
	if err != nil {
		return err
	}
	defer materialized.Close()
	metadata, err := worker.VideoProbe.Probe(ctx, materialized.Path)
	if err != nil {
		_ = worker.Database.VideoMetadata().PublishError(ctx, item.ItemUUID, item.ContentRevision, job.ProfileHash, mediaprocessing.VideoErrorCode(err), time.Now())
		return err
	}
	metadata.ItemUUID, metadata.ContentRevision, metadata.ProbeProfileHash = item.ItemUUID, item.ContentRevision, job.ProfileHash
	if err := worker.Database.VideoMetadata().PublishReady(ctx, metadata, time.Now()); err != nil {
		return err
	}
	profile := worker.PosterProfileHash
	if profile == "" {
		profile = mediaprocessing.DefaultProfileHash()
	}
	revision := item.ContentRevision
	_, err = worker.Database.ProcessingJobs().Enqueue(ctx, productdb.EnqueueJobInput{Key: productdb.ItemDerivativeJobKey(item.ItemUUID, mediaprocessing.VariantStaticPoster, revision, profile),
		Kind: mediaprocessing.JobItemDerivative, GalleryID: &item.GalleryID, ItemUUID: item.ItemUUID, Variant: mediaprocessing.VariantStaticPoster, ContentRevision: &revision, ProfileHash: profile,
		Payload: map[string]any{"cache_tier": mediaprocessing.CacheBase}, Priority: job.Priority}, time.Now())
	return err
}

var (
	ErrUnsupportedGeneration = errors.New("no media generator supports the requested derivative")
	ErrStaleContent          = errors.New("processing job content revision is stale")
)

func (worker Worker) processDerivative(ctx context.Context, job mediaprocessing.Job) error {
	item, err := worker.Database.FindProcessingItem(ctx, job.ItemUUID)
	if err != nil {
		return err
	}
	if job.ContentRevision == nil || item.ContentRevision != *job.ContentRevision {
		return ErrStaleContent
	}
	if item.Excluded || item.Availability != gallery.AvailabilityAvailable {
		return ErrStaleContent
	}
	if item.MediaKind == gallery.MediaKindVideo && item.SourceType != gallery.SourceTypeDirectory {
		return ErrStaleContent
	}
	if job.Variant == mediaprocessing.VariantVideoPlayback {
		if err := worker.requireVideoProxyCapacity(ctx); err != nil {
			return err
		}
	}
	materialized, err := worker.Materializer.Open(ctx, mediaaccess.Source{Type: item.SourceType, Path: item.SourcePath, RelativePath: item.RelativePath})
	if err != nil {
		return err
	}
	defer materialized.Close()
	extension := outputExtension(job.Variant)
	relative, err := worker.Cache.RelativePath(item.ItemUUID, item.ContentRevision, job.Variant, job.ProfileHash, extension)
	if err != nil {
		return err
	}
	request := mediaprocessing.GenerateRequest{ItemUUID: item.ItemUUID, MediaKind: item.MediaKind, ContentFormat: item.ContentFormat,
		ContentRevision: item.ContentRevision, Variant: job.Variant, ProfileHash: job.ProfileHash, SourcePath: materialized.Path}
	if item.MediaKind == gallery.MediaKindVideo {
		metadata, findErr := worker.Database.VideoMetadata().Find(ctx, item.ItemUUID)
		if findErr != nil || metadata.ContentRevision != item.ContentRevision || metadata.ProbeState != mediaprocessing.VideoProbeReady {
			return ErrStaleContent
		}
		request.VideoTechnical = &metadata
		if job.Variant == mediaprocessing.VariantVideoPlayback {
			plan := mediaprocessing.PlaybackPlanFromMetadata(metadata)
			if plan.Mode == mediaprocessing.PlaybackDirect {
				return ErrUnsupportedGeneration
			}
			request.VideoPlan = &plan
		}
	}
	var generator mediaprocessing.Generator
	for _, candidate := range worker.Generators {
		if candidate.Supports(request) {
			generator = candidate
			break
		}
	}
	if generator == nil {
		if item.MediaKind == gallery.MediaKindVideo {
			code := worker.FFmpegUnavailableCode
			if code == "" {
				code = mediaprocessing.ErrorFFmpegUnavailable
			}
			return &mediaprocessing.VideoProcessingError{Code: code, Err: errors.New("ffmpeg is unavailable")}
		}
		return ErrUnsupportedGeneration
	}
	var generated mediaprocessing.GenerateResult
	_, size, err := worker.Cache.WriteAtomicPath(relative, func(destination string) error {
		request.DestinationPath = destination
		var generateErr error
		generated, generateErr = generator.Generate(ctx, request)
		return generateErr
	})
	if err != nil {
		return err
	}
	if generated.ByteSize == 0 {
		generated.ByteSize = size
	}
	tier, knownVariant := mediaprocessing.RequiredCacheTier(job.Variant)
	if !knownVariant {
		_ = worker.Cache.RemoveUnpublished(relative)
		return ErrUnsupportedGeneration
	}
	var payload struct {
		CacheTier mediaprocessing.CacheTier `json:"cache_tier"`
	}
	if json.Unmarshal(job.PayloadJSON, &payload) == nil && payload.CacheTier != "" && payload.CacheTier != tier {
		_ = worker.Cache.RemoveUnpublished(relative)
		return errors.New("processing job cache tier conflicts with variant retention policy")
	}
	_, err = worker.Database.Derivatives().Publish(ctx, productdb.PublishDerivativeInput{ItemUUID: item.ItemUUID, Variant: job.Variant,
		CacheTier: tier, ContentRevision: item.ContentRevision, ProfileHash: job.ProfileHash, CacheRelativePath: relative,
		MIMEType: generated.MIMEType, ByteSize: generated.ByteSize, Width: generated.Width, Height: generated.Height}, time.Now())
	if err != nil {
		_ = worker.Cache.RemoveUnpublished(relative)
		return err
	}
	if _, coverErr := worker.Database.Covers().Reconcile(ctx, item.GalleryID, false, time.Now()); errors.Is(coverErr, sql.ErrNoRows) {
		_, _ = worker.Database.Covers().Initialize(ctx, item.GalleryID, time.Now())
	}
	return nil
}

func (worker Worker) requireVideoProxyCapacity(ctx context.Context) error {
	runtime, err := worker.Database.Settings().Find(ctx)
	if err != nil {
		return err
	}
	_, enhanced, err := worker.Database.Derivatives().CacheTierBytes(ctx)
	if err != nil {
		return err
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(worker.Cache.Root, &stat); err != nil {
		return err
	}
	plan := mediaprocessing.PlanCacheCleanup(mediaprocessing.CachePressure{EnhancedBytes: enhanced,
		AvailableBytes: int64(stat.Bavail) * int64(stat.Bsize), TotalBytes: int64(stat.Blocks) * int64(stat.Bsize),
		MaximumEnhancedBytes: runtime.EnhancedCacheMaximumBytes, MinimumFreeBytes: runtime.MinimumFreeBytes, MinimumFreePercent: runtime.MinimumFreePercent})
	if plan.BytesToFree > 0 || plan.PauseNewProcessing {
		return &mediaprocessing.VideoProcessingError{Code: mediaprocessing.ErrorDiskSpaceLow, Err: errors.New("enhanced cache has insufficient capacity")}
	}
	return nil
}

func outputExtension(variant string) string {
	if variant == mediaprocessing.VariantVideoPlayback {
		return "mp4"
	}
	if variant == mediaprocessing.VariantAnimatedPreview {
		return "webp"
	}
	return "jpg"
}
func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrUnsupportedGeneration):
		return "UNSUPPORTED_GENERATOR"
	case errors.Is(err, ErrStaleContent):
		return "STALE_CONTENT"
	default:
		if code := mediaprocessing.VideoErrorCode(err); code != "" {
			return code
		}
		return "GENERATION_FAILED"
	}
}
