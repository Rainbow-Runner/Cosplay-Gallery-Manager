package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

type VideoMetadataStore struct{ db *sql.DB }

func (db *Database) VideoMetadata() *VideoMetadataStore { return &VideoMetadataStore{db: db.DB} }

// EnqueueBackfill schedules a bounded, low-priority batch for existing
// DIRECTORY videos whose current revision has never been probed with the
// active profile. It never reads source media and never retries an already
// recorded probe error automatically.
func (s *VideoMetadataStore) EnqueueBackfill(ctx context.Context, profile string, limit int, now time.Time) (int, error) {
	if profile == "" || limit < 1 || limit > 500 {
		return 0, errors.New("invalid video probe backfill request")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT item.item_uuid,item.gallery_id,item.content_revision
		FROM gallery_items item JOIN gallery_sources source ON source.id=item.source_id
		LEFT JOIN video_technical_metadata video ON video.item_uuid=item.item_uuid
		WHERE item.media_kind='VIDEO' AND item.excluded=0 AND item.availability_state='AVAILABLE'
		AND source.source_type='DIRECTORY' AND source.availability_state='AVAILABLE'
		AND (video.item_uuid IS NULL OR video.content_revision<>item.content_revision OR video.probe_profile_hash<>?)
		ORDER BY item.id LIMIT ?`, profile, limit)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		uuid      string
		galleryID int64
		revision  int64
	}
	var candidates []candidate
	for rows.Next() {
		var value candidate
		if err := rows.Scan(&value.uuid, &value.galleryID, &value.revision); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, value)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	timestamp := formatTime(normalisedTime(now))
	queued := 0
	for _, value := range candidates {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return queued, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO video_technical_metadata(item_uuid,content_revision,probe_profile_hash,probe_state)
			VALUES(?,?,?,'PENDING') ON CONFLICT(item_uuid) DO UPDATE SET content_revision=excluded.content_revision,
			probe_profile_hash=excluded.probe_profile_hash,probe_state='PENDING',last_error_code='',completed_at_utc=NULL`, value.uuid, value.revision, profile); err == nil {
			key := ItemTechnicalMetadataJobKey(value.uuid, value.revision, profile)
			_, err = tx.ExecContext(ctx, `INSERT INTO processing_jobs(job_key,job_kind,gallery_id,item_uuid,variant,content_revision,profile_hash,payload_json,
				status,priority,max_attempts,not_before_utc,created_at_utc,updated_at_utc)
				VALUES(?,'ITEM_TECHNICAL_METADATA',?,?,'',?,?,'{}','PENDING',25,3,?,?,?)
				ON CONFLICT(job_key) DO UPDATE SET priority=MAX(priority,excluded.priority),updated_at_utc=excluded.updated_at_utc`,
				key, value.galleryID, value.uuid, value.revision, profile, timestamp, timestamp, timestamp)
		}
		if err != nil {
			_ = tx.Rollback()
			return queued, err
		}
		if err := tx.Commit(); err != nil {
			return queued, err
		}
		queued++
	}
	return queued, nil
}

func (s *VideoMetadataStore) Find(ctx context.Context, itemUUID string) (mediaprocessing.VideoTechnicalMetadata, error) {
	return scanVideoTechnicalMetadata(s.db.QueryRowContext(ctx, videoMetadataSelect+` WHERE item_uuid=?`, itemUUID))
}

func (s *VideoMetadataStore) MarkPending(ctx context.Context, itemUUID string, revision int64, profile string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO video_technical_metadata(item_uuid,content_revision,probe_profile_hash,probe_state)
		VALUES(?,?,?,'PENDING') ON CONFLICT(item_uuid) DO UPDATE SET content_revision=excluded.content_revision,
		probe_profile_hash=excluded.probe_profile_hash,probe_state='PENDING',last_error_code='',completed_at_utc=NULL`, itemUUID, revision, profile)
	return err
}

func (s *VideoMetadataStore) PublishReady(ctx context.Context, value mediaprocessing.VideoTechnicalMetadata, now time.Time) error {
	completed := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `UPDATE video_technical_metadata SET probe_state='READY',last_error_code='',container=?,duration_seconds=?,start_time_seconds=?,
		total_bitrate=?,video_bitrate=?,video_stream_index=?,video_codec=?,video_profile=?,pixel_format=?,coded_width=?,coded_height=?,display_width=?,display_height=?,
		frame_rate=?,rotation=?,color_range=?,color_space=?,color_primaries=?,color_transfer=?,hdr=?,audio_stream_index=?,audio_codec=?,audio_channels=?,audio_sample_rate=?,
		ffprobe_version=?,completed_at_utc=? WHERE item_uuid=? AND content_revision=? AND probe_profile_hash=?`,
		value.Container, value.DurationSeconds, value.StartTimeSeconds, value.TotalBitrate, value.VideoBitrate, value.VideoStreamIndex, value.VideoCodec, value.VideoProfile, value.PixelFormat,
		value.CodedWidth, value.CodedHeight, value.DisplayWidth, value.DisplayHeight, value.FrameRate, value.Rotation, value.ColorRange, value.ColorSpace, value.ColorPrimaries, value.ColorTransfer,
		boolInt(value.HDR), value.AudioStreamIndex, value.AudioCodec, value.AudioChannels, value.AudioSampleRate, value.FFprobeVersion, completed, value.ItemUUID, value.ContentRevision, value.ProbeProfileHash)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("video probe result is stale")
	}
	return nil
}

func (s *VideoMetadataStore) PublishError(ctx context.Context, itemUUID string, revision int64, profile, code string, now time.Time) error {
	if len(code) > 100 {
		return errors.New("video probe error code exceeds limit")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE video_technical_metadata SET probe_state='ERROR',last_error_code=?,completed_at_utc=?
		WHERE item_uuid=? AND content_revision=? AND probe_profile_hash=?`, code, formatTime(normalisedTime(now)), itemUUID, revision, profile)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("video probe failure is stale")
	}
	return nil
}

func (s *VideoMetadataStore) Retry(ctx context.Context, itemUUID, requestedProfile string, now time.Time) error {
	var galleryID, revision int64
	var mediaKind, availability string
	var excluded int
	var sourceType string
	if err := s.db.QueryRowContext(ctx, `SELECT item.gallery_id,item.content_revision,item.media_kind,item.availability_state,item.excluded,source.source_type
		FROM gallery_items item JOIN gallery_sources source ON source.id=item.source_id WHERE item.item_uuid=?`, itemUUID).
		Scan(&galleryID, &revision, &mediaKind, &availability, &excluded, &sourceType); err != nil {
		return err
	}
	if mediaKind != "VIDEO" || availability != "AVAILABLE" || excluded != 0 || sourceType != string(gallery.SourceTypeDirectory) {
		return errors.New("video Item is not processable")
	}
	profile := requestedProfile
	if profile == "" {
		profile = mediaprocessing.DefaultProfileHash()
	}
	var existingProfile string
	if requestedProfile == "" {
		if err := s.db.QueryRowContext(ctx, `SELECT probe_profile_hash FROM video_technical_metadata WHERE item_uuid=?`, itemUUID).Scan(&existingProfile); err == nil && existingProfile != "" {
			profile = existingProfile
		}
	}
	if err := s.MarkPending(ctx, itemUUID, revision, profile); err != nil {
		return err
	}
	key := ItemTechnicalMetadataJobKey(itemUUID, revision, profile)
	jobs := &ProcessingJobStore{db: s.db}
	job, err := jobs.FindByKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = jobs.Enqueue(ctx, EnqueueJobInput{Key: key, Kind: mediaprocessing.JobItemTechnicalMetadata, GalleryID: &galleryID, ItemUUID: itemUUID, ContentRevision: &revision, ProfileHash: profile, Payload: map[string]any{}, Priority: 500}, now)
		return err
	}
	if err != nil {
		return err
	}
	if job.Status == mediaprocessing.JobCompleted || job.Status == mediaprocessing.JobFailed || job.Status == mediaprocessing.JobCancelled {
		_, err = jobs.Requeue(ctx, key, 500, now)
	}
	return err
}

const videoMetadataSelect = `SELECT item_uuid,content_revision,probe_profile_hash,probe_state,last_error_code,container,duration_seconds,start_time_seconds,
	total_bitrate,video_bitrate,video_stream_index,video_codec,video_profile,pixel_format,coded_width,coded_height,display_width,display_height,frame_rate,rotation,
	color_range,color_space,color_primaries,color_transfer,hdr,audio_stream_index,audio_codec,audio_channels,audio_sample_rate,ffprobe_version,completed_at_utc
	FROM video_technical_metadata`

func scanVideoTechnicalMetadata(row rowScanner) (mediaprocessing.VideoTechnicalMetadata, error) {
	var result mediaprocessing.VideoTechnicalMetadata
	var hdr int
	var audioIndex sql.NullInt64
	var completed sql.NullString
	if err := row.Scan(&result.ItemUUID, &result.ContentRevision, &result.ProbeProfileHash, &result.ProbeState, &result.LastErrorCode, &result.Container, &result.DurationSeconds,
		&result.StartTimeSeconds, &result.TotalBitrate, &result.VideoBitrate, &result.VideoStreamIndex, &result.VideoCodec, &result.VideoProfile, &result.PixelFormat, &result.CodedWidth,
		&result.CodedHeight, &result.DisplayWidth, &result.DisplayHeight, &result.FrameRate, &result.Rotation, &result.ColorRange, &result.ColorSpace, &result.ColorPrimaries, &result.ColorTransfer,
		&hdr, &audioIndex, &result.AudioCodec, &result.AudioChannels, &result.AudioSampleRate, &result.FFprobeVersion, &completed); err != nil {
		return result, err
	}
	result.HDR = hdr == 1
	if audioIndex.Valid {
		value := int(audioIndex.Int64)
		result.AudioStreamIndex = &value
	}
	if completed.Valid {
		value, err := parseTime(completed.String)
		if err != nil {
			return result, err
		}
		result.CompletedAtUTC = &value
	}
	return result, nil
}
