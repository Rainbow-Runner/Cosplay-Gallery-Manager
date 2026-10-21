package processingworker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/imagemetadata"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

func (worker Worker) processMediaTimes(ctx context.Context, job mediaprocessing.Job) error {
	item, err := worker.Database.FindProcessingItem(ctx, job.ItemUUID)
	if err != nil {
		return err
	}
	if job.ContentRevision == nil || *job.ContentRevision != item.ContentRevision || item.Excluded || item.Availability != gallery.AvailabilityAvailable {
		return ErrStaleContent
	}
	zone, err := worker.Database.CaptureDates().Timezone(ctx, item.ItemUUID)
	if err != nil {
		return err
	}
	var payload struct {
		Timezone string `json:"timezone"`
	}
	if err = json.Unmarshal(job.PayloadJSON, &payload); err != nil || payload.Timezone != zone {
		return ErrStaleContent
	}
	if done, e := worker.Database.MediaTimes().HasCurrent(ctx, item.ItemUUID, item.ContentRevision, zone); e != nil {
		return e
	} else if done {
		return nil
	}
	ctx, cancel := archiveVideoTaskContext(ctx, item)
	defer cancel()
	input, err := worker.openInput(ctx, item, "")
	if err != nil {
		return err
	}
	defer input.Close()
	var value imagemetadata.TimeSummary
	if item.MediaKind == gallery.MediaKindStaticImage {
		value, err = imagemetadata.ExtractTimeSummaryPath(input.Path, zone)
	} else if item.MediaKind == gallery.MediaKindVideo {
		value.CaptureAtUTC, value.CapturePrecision, value.CaptureZoneSource, err = worker.CaptureProbe.CaptureTime(ctx, input.Path, zone)
	} else {
		return ErrStaleContent
	}
	if err != nil {
		return err
	}
	if err = input.Validate(); err != nil {
		return err
	}
	return worker.Database.MediaTimes().Publish(ctx, item.ItemUUID, item.ContentRevision, zone, value, time.Now(), job)
}

func (worker Worker) mediaTimesDuringDerivative(ctx context.Context, item productdb.ProcessingItem, path string, validate func() error, job mediaprocessing.Job) {
	zone, err := worker.Database.CaptureDates().Timezone(ctx, item.ItemUUID)
	if err != nil {
		return
	}
	if done, e := worker.Database.MediaTimes().HasCurrent(ctx, item.ItemUUID, item.ContentRevision, zone); e != nil || done {
		return
	}
	value, err := imagemetadata.ExtractTimeSummaryPath(path, zone)
	if err == nil {
		err = validate()
	}
	if err == nil {
		err = worker.Database.MediaTimes().Publish(ctx, item.ItemUUID, item.ContentRevision, zone, value, time.Now(), job)
	}
	if err != nil {
		slog.Warn("CGM_MEDIA_TIME_INLINE_FAILED", "item_uuid", item.ItemUUID)
	}
}
