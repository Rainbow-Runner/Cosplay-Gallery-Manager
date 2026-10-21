package productserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/imagemetadata"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/settings"
)

// MediaEmbeddedMetadata reads display-only metadata on explicit detail-page
// demand. Image metadata is never written to the product database or cache.
// Browse visibility is checked before the internal source path is resolved.
func (s *Server) MediaEmbeddedMetadata(ctx context.Context, itemUUID string, visibleFields []string) (browse.MediaInformationSummary, error) {
	if err := settings.ValidateMediaMetadataVisibleFields(visibleFields); err != nil {
		return browse.MediaInformationSummary{}, err
	}
	detail, err := s.Database.Browse().MediaDetail(ctx, itemUUID)
	if err != nil {
		return browse.MediaInformationSummary{}, err
	}
	item, err := s.Database.FindProcessingItem(ctx, itemUUID)
	if err != nil {
		return browse.MediaInformationSummary{}, err
	}
	visible := settings.MediaMetadataVisibleSet(visibleFields)
	result := browse.MediaInformationSummary{State: "READY"}
	appendEntry := func(key, label, group, value string) {
		if visible[key] && strings.TrimSpace(value) != "" {
			result.Entries = append(result.Entries, browse.MediaInformationEntry{Key: key, VisibilityKey: key, Label: label, Group: group, Value: value, Source: "CGM", RawValue: value})
		}
	}
	var byteSize int64
	var createdAt string
	if err := s.Database.QueryRowContext(ctx, `SELECT byte_size,created_at_utc FROM gallery_items WHERE item_uuid=?`, itemUUID).Scan(&byteSize, &createdAt); err != nil {
		return browse.MediaInformationSummary{}, err
	}
	appendEntry(settings.MetadataFileType, "Media type", "FILE", fmt.Sprintf("%s · %s", detail.Item.MediaKind, detail.Item.ContentFormat))
	appendEntry(settings.MetadataFileSize, "File size", "FILE", formatMetadataByteSize(byteSize))
	appendEntry(settings.MetadataFileAddedAt, "Added to CGM", "FILE", createdAt)
	appendEntry(settings.MetadataCurrentFilename, "Current filename", "FILE", filepath.Base(filepath.FromSlash(item.RelativePath)))
	var sourceModified, sourceOrigin string
	if err := s.Database.QueryRowContext(ctx, `SELECT source_modified_at_utc,source_modified_origin FROM gallery_items WHERE item_uuid=?`, itemUUID).Scan(&sourceModified, &sourceOrigin); err != nil {
		return browse.MediaInformationSummary{}, err
	}
	if sourceModified != "" && visible[settings.MetadataSourceModified] {
		result.Entries = append(result.Entries, browse.MediaInformationEntry{Key: settings.MetadataSourceModified, VisibilityKey: settings.MetadataSourceModified, Label: "Source modified time", Group: "FILE", Value: sourceModified, RawValue: sourceModified, Source: sourceOrigin, Timestamp: sourceModified})
	}

	if item.MediaKind == gallery.MediaKindVideo {
		metadata, findErr := s.Database.VideoMetadata().Find(ctx, itemUUID)
		if errors.Is(findErr, sql.ErrNoRows) {
			return result, nil
		}
		if findErr != nil {
			return browse.MediaInformationSummary{}, findErr
		}
		if metadata.ProbeState == mediaprocessing.VideoProbeReady {
			appendEntry(settings.MetadataFileDimensions, "Dimensions", "FILE", fmt.Sprintf("%d × %d", metadata.DisplayWidth, metadata.DisplayHeight))
			appendEntry(settings.MetadataVideoDuration, "Duration", "VIDEO", formatMetadataDuration(metadata.DurationSeconds))
			appendEntry(settings.MetadataVideoContainer, "Container", "VIDEO", metadata.Container)
			appendEntry(settings.MetadataVideoCodec, "Video codec", "VIDEO", metadata.VideoCodec)
			if metadata.FrameRate > 0 {
				appendEntry(settings.MetadataVideoFrameRate, "Frame rate", "VIDEO", fmt.Sprintf("%.2f fps", metadata.FrameRate))
			}
			appendEntry(settings.MetadataAudioCodec, "Audio codec", "VIDEO", metadata.AudioCodec)
		}
		return result, nil
	}

	s.mediaMetadataOnce.Do(func() { s.mediaMetadataSlots = make(chan struct{}, 2) })
	select {
	case s.mediaMetadataSlots <- struct{}{}:
		defer func() { <-s.mediaMetadataSlots }()
	case <-ctx.Done():
		return browse.MediaInformationSummary{}, ctx.Err()
	}
	runtimeSettings, err := s.Database.Settings().Find(ctx)
	if err != nil {
		return browse.MediaInformationSummary{}, err
	}
	materialized, err := (mediaaccess.Materializer{TemporaryRoot: filepath.Join(s.Config.CachePath, "tmp"), MaximumBytes: runtimeSettings.ArchiveMaxEntryBytes}).Open(ctx,
		mediaaccess.Source{Type: item.SourceType, Path: item.SourcePath, RelativePath: item.RelativePath})
	if err != nil {
		result.State = "ERROR"
		result.ErrorCode = "IMAGE_METADATA_SOURCE_UNAVAILABLE"
		return result, nil
	}
	defer materialized.Close()
	extracted, err := imagemetadata.ExtractDisplayPath(materialized.Path)
	if err != nil {
		result.State = "ERROR"
		result.ErrorCode = "IMAGE_METADATA_READ_FAILED"
		return result, nil
	}
	if err := materialized.Validate(); err != nil {
		result.State = "ERROR"
		result.ErrorCode = "IMAGE_METADATA_SOURCE_CHANGED"
		return result, nil
	}
	if extracted.Width > 0 && extracted.Height > 0 {
		appendEntry(settings.MetadataFileDimensions, "Dimensions", "FILE", fmt.Sprintf("%d × %d", extracted.Width, extracted.Height))
	}
	result.Truncated = extracted.Truncated
	result.Warnings = extracted.Warnings
	for _, entry := range extracted.Entries {
		if visible[entry.VisibilityKey] {
			result.Entries = append(result.Entries, browse.MediaInformationEntry{Key: entry.Key, VisibilityKey: entry.VisibilityKey, Label: entry.Label, Group: entry.Group, Value: entry.Value, Source: entry.Source, RawValue: entry.RawValue, Timestamp: entry.Timestamp})
		}
	}
	for _, event := range extracted.History {
		conversion := event.Action == "converted" || event.Action == "derived"
		if conversion && !visible[settings.MetadataHistoryConversions] || !conversion && !visible[settings.MetadataHistoryEdits] {
			continue
		}
		result.History = append(result.History, browse.MediaHistoryEvent{Index: event.Index, Action: event.Action, Software: event.Software, When: event.When, Timestamp: event.Timestamp, Parameters: event.Parameters,
			InputFormat: event.InputFormat, OutputFormat: event.OutputFormat, RawParameters: event.RawParameters, Changed: event.Changed, DetailsRedacted: event.DetailsRedacted})
	}
	var modified *time.Time
	for _, entry := range result.Entries {
		if entry.VisibilityKey != settings.MetadataDateModified || entry.Timestamp == "" {
			continue
		}
		stamp, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			continue
		}
		if modified != nil && !modified.Equal(stamp) {
			result.Warnings = append(result.Warnings, "MODIFIED_TIME_CONFLICT")
			break
		}
		modified = &stamp
	}
	return result, nil
}

func formatMetadataByteSize(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	amount, unit := float64(value), "B"
	for _, candidate := range units {
		amount /= 1024
		unit = candidate
		if amount < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.2f %s", amount, unit)
}

func formatMetadataDuration(value float64) string {
	total := int64(value + .5)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
