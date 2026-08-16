package productdb

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/stashapp/stash/internal/settings"
)

var ErrSettingsRevisionConflict = errors.New("settings revision conflict")

type SettingsStore struct{ db *sql.DB }

func (db *Database) Settings() *SettingsStore { return &SettingsStore{db: db.DB} }

func (s *SettingsStore) Find(ctx context.Context) (settings.Runtime, error) {
	var result settings.Runtime
	var scrubber, mediaFilter, galleryControls, mediaControls, detailControls, autoScan, suspended, dailyBackup int
	err := s.db.QueryRowContext(ctx, `SELECT settings_revision,home_scope,gallery_card_scrubber_enabled,
		gallery_detail_media_filter_enabled,gallery_card_controls_visible,media_card_controls_visible,
		detail_personal_controls_visible,gallery_animated_playback_limit,gallery_animated_lock_interval_ms,
		related_limit,tag_parent_weight,tag_minimum_score,tag_maximum_depth,
		random_limit,random_static_quota,random_gif_quota,random_video_quota,random_gallery_repeat_decay,
		enhanced_cache_maximum_bytes,minimum_free_bytes,minimum_free_percent,automatic_scan_enabled,
		automatic_schedules_suspended,daily_backup_enabled,daily_backup_retention,archive_max_entries,archive_max_entry_bytes,archive_max_total_bytes,
		archive_max_compression_ratio,archive_max_image_pixels FROM runtime_settings WHERE id=1`).Scan(&result.Revision, &result.HomeScope,
		&scrubber, &mediaFilter, &galleryControls, &mediaControls, &detailControls,
		&result.GalleryAnimatedPlaybackLimit, &result.GalleryAnimatedLockIntervalMS, &result.RelatedLimit,
		&result.TagParentWeight, &result.TagMinimumScore, &result.TagMaximumDepth, &result.RandomLimit,
		&result.RandomStaticQuota, &result.RandomGIFQuota, &result.RandomVideoQuota, &result.RandomGalleryRepeatDecay,
		&result.EnhancedCacheMaximumBytes, &result.MinimumFreeBytes, &result.MinimumFreePercent, &autoScan, &suspended,
		&dailyBackup, &result.DailyBackupRetention,
		&result.ArchiveMaxEntries, &result.ArchiveMaxEntryBytes, &result.ArchiveMaxTotalBytes,
		&result.ArchiveMaxCompressionRatio, &result.ArchiveMaxImagePixels)
	if err != nil {
		return settings.Runtime{}, err
	}
	result.GalleryCardScrubberEnabled = scrubber == 1
	result.GalleryDetailMediaFilterEnabled = mediaFilter == 1
	result.GalleryCardControlsVisible = galleryControls == 1
	result.MediaCardControlsVisible = mediaControls == 1
	result.DetailPersonalControlsVisible = detailControls == 1
	result.AutomaticScanEnabled = autoScan == 1
	result.AutomaticSchedulesSuspended = suspended == 1
	result.DailyBackupEnabled = dailyBackup == 1
	return result, nil
}

func (s *SettingsStore) Update(ctx context.Context, expectedRevision int64, input settings.Runtime, now time.Time) (settings.Runtime, error) {
	if err := validateRuntimeSettings(input); err != nil {
		return settings.Runtime{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE runtime_settings SET settings_revision=settings_revision+1,
		home_scope=?,gallery_card_scrubber_enabled=?,gallery_detail_media_filter_enabled=?,
		gallery_card_controls_visible=?,media_card_controls_visible=?,detail_personal_controls_visible=?,
		gallery_animated_playback_limit=?,gallery_animated_lock_interval_ms=?,
		related_limit=?,tag_parent_weight=?,tag_minimum_score=?,tag_maximum_depth=?,random_limit=?,
		random_static_quota=?,random_gif_quota=?,random_video_quota=?,random_gallery_repeat_decay=?,
		enhanced_cache_maximum_bytes=?,minimum_free_bytes=?,minimum_free_percent=?,automatic_scan_enabled=?,
		automatic_schedules_suspended=?,daily_backup_enabled=?,daily_backup_retention=?,archive_max_entries=?,archive_max_entry_bytes=?,archive_max_total_bytes=?,
		archive_max_compression_ratio=?,archive_max_image_pixels=?,updated_at_utc=? WHERE id=1 AND settings_revision=?`, input.HomeScope,
		input.GalleryCardScrubberEnabled, input.GalleryDetailMediaFilterEnabled, input.GalleryCardControlsVisible,
		input.MediaCardControlsVisible, input.DetailPersonalControlsVisible, input.GalleryAnimatedPlaybackLimit,
		input.GalleryAnimatedLockIntervalMS, input.RelatedLimit, input.TagParentWeight,
		input.TagMinimumScore, input.TagMaximumDepth, input.RandomLimit, input.RandomStaticQuota, input.RandomGIFQuota,
		input.RandomVideoQuota, input.RandomGalleryRepeatDecay, input.EnhancedCacheMaximumBytes, input.MinimumFreeBytes,
		input.MinimumFreePercent, input.AutomaticScanEnabled, input.AutomaticSchedulesSuspended, input.DailyBackupEnabled, input.DailyBackupRetention, input.ArchiveMaxEntries,
		input.ArchiveMaxEntryBytes, input.ArchiveMaxTotalBytes, input.ArchiveMaxCompressionRatio, input.ArchiveMaxImagePixels,
		formatTime(normalisedTime(now)), expectedRevision)
	if err != nil {
		return settings.Runtime{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return settings.Runtime{}, err
	}
	if rows != 1 {
		return settings.Runtime{}, ErrSettingsRevisionConflict
	}
	return s.Find(ctx)
}

func validateRuntimeSettings(value settings.Runtime) error {
	if value.HomeScope != settings.HomeList && value.HomeScope != settings.HomeMagic && value.HomeScope != settings.HomeAll {
		return errors.New("invalid Home scope")
	}
	if value.RelatedLimit < 1 || value.RelatedLimit > 24 || value.TagParentWeight < 0 || value.TagParentWeight > 1 ||
		value.TagMinimumScore < 0 || value.TagMinimumScore > 1 || value.TagMaximumDepth < 0 || value.TagMaximumDepth > 10 ||
		value.RandomLimit < 1 || value.RandomLimit > 100 || value.RandomGalleryRepeatDecay < 0 || value.RandomGalleryRepeatDecay > 1 ||
		value.EnhancedCacheMaximumBytes < 0 || value.MinimumFreeBytes < 0 || value.MinimumFreePercent < 0 || value.MinimumFreePercent > 1 {
		return errors.New("runtime setting is outside its supported range")
	}
	if value.GalleryAnimatedPlaybackLimit < 1 || value.GalleryAnimatedPlaybackLimit > 16 {
		return errors.New("gallery animated playback limit must be between 1 and 16")
	}
	if value.GalleryAnimatedLockIntervalMS < 700 || value.GalleryAnimatedLockIntervalMS > 1000 {
		return errors.New("gallery animated lock interval must be between 700 and 1000 milliseconds")
	}
	if value.DailyBackupRetention < 1 || value.DailyBackupRetention > 365 {
		return errors.New("daily backup retention must be between 1 and 365")
	}
	if value.ArchiveMaxEntries <= 0 || value.ArchiveMaxEntryBytes <= 0 || value.ArchiveMaxTotalBytes <= 0 ||
		value.ArchiveMaxCompressionRatio <= 0 || value.ArchiveMaxImagePixels <= 0 {
		return errors.New("archive resource limits must all be positive")
	}
	if value.RandomStaticQuota < 0 || value.RandomGIFQuota < 0 || value.RandomVideoQuota < 0 ||
		math.Abs(value.RandomStaticQuota+value.RandomGIFQuota+value.RandomVideoQuota-1) > 0.000001 {
		return errors.New("random media quotas must be non-negative and sum to 1")
	}
	return nil
}
