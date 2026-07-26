package processingworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

type Worker struct {
	Database     *productdb.Database
	Materializer mediaaccess.Materializer
	Cache        mediaprocessing.CacheWriter
	Generators   []mediaprocessing.Generator
}

func (worker Worker) RunOne(ctx context.Context, owner string, lease time.Duration, now time.Time) (mediaprocessing.Job, error) {
	if worker.Database == nil {
		return mediaprocessing.Job{}, errors.New("processing worker database is required")
	}
	job, err := worker.Database.ProcessingJobs().ClaimNext(ctx, owner, lease, now)
	if err != nil {
		return mediaprocessing.Job{}, err
	}
	if job.Kind != mediaprocessing.JobItemDerivative {
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
	processErr := worker.processDerivative(processingContext, job)
	cancel()
	heartbeatErr := <-heartbeatDone
	if processErr == nil && heartbeatErr != nil {
		processErr = heartbeatErr
	}
	if processErr != nil {
		structural := errors.Is(processErr, ErrUnsupportedGeneration) || errors.Is(processErr, ErrStaleContent)
		_, failErr := worker.Database.ProcessingJobs().Fail(ctx, job.ID, owner, errorCode(processErr), structural, time.Now())
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
	var generator mediaprocessing.Generator
	for _, candidate := range worker.Generators {
		if candidate.Supports(request) {
			generator = candidate
			break
		}
	}
	if generator == nil {
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

func outputExtension(variant string) string {
	if variant == mediaprocessing.VariantVideoPlayback {
		return "mp4"
	}
	return "jpg"
}
func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrUnsupportedGeneration):
		return "UNSUPPORTED_GENERATOR"
	case errors.Is(err, ErrStaleContent):
		return "STALE_CONTENT"
	default:
		return "GENERATION_FAILED"
	}
}
