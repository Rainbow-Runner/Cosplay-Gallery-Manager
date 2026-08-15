package productdb

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestVideoPlaybackRequestDirectDoesNotQueueAndProxyRequestsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 15, 16, 0, 0, 0, time.UTC)
	created, source := createBrowseGallery(t, db, "Playback", gallery.ContentRatingNonAdult, now)
	activateBrowseFixture(t, db, created.ID, now)
	direct := addBrowseItem(t, db, created.ID, source.ID, "direct.mp4", gallery.MediaKindVideo, "", gallery.AvailabilityAvailable, gallery.ProcessingReady, 1024, now)
	proxy := addBrowseItem(t, db, created.ID, source.ID, "proxy.mkv", gallery.MediaKindVideo, "", gallery.AvailabilityAvailable, gallery.ProcessingReady, 2048, now)
	probeProfile := mediaprocessing.VideoProbeProfileHash("6.1")
	publishVideoMetadata(t, db, direct, probeProfile, "mp4", "h264", "aac", 0, false, now)
	publishVideoMetadata(t, db, proxy, probeProfile, "matroska", "h264", "flac", 0, false, now)

	directStatus, err := db.Browse().RequestVideoPlayback(ctx, direct.UUID, "6.1", "", now.Add(time.Minute))
	if err != nil || directStatus.Mode != string(mediaprocessing.PlaybackDirect) || directStatus.Status != gallery.ProcessingReady || directStatus.Resource != nil {
		t.Fatalf("direct status = %#v, %v", directStatus, err)
	}
	var directJobs int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE item_uuid=? AND variant=?`, direct.UUID, mediaprocessing.VariantVideoPlayback).Scan(&directJobs); err != nil || directJobs != 0 {
		t.Fatalf("direct jobs = %d, %v", directJobs, err)
	}

	withoutFFmpeg, err := db.Browse().RequestVideoPlayback(ctx, proxy.UUID, "", mediaprocessing.ErrorFFmpegUnavailable, now.Add(2*time.Minute))
	if err != nil || withoutFFmpeg.Status != gallery.ProcessingError || withoutFFmpeg.ErrorCode != mediaprocessing.ErrorFFmpegUnavailable {
		t.Fatalf("missing FFmpeg status = %#v, %v", withoutFFmpeg, err)
	}
	unsupportedFFmpeg, err := db.Browse().VideoPlaybackStatus(ctx, proxy.UUID, "", mediaprocessing.ErrorFFmpegVersionUnsupported)
	if err != nil || unsupportedFFmpeg.ErrorCode != mediaprocessing.ErrorFFmpegVersionUnsupported {
		t.Fatalf("unsupported FFmpeg status = %#v, %v", unsupportedFFmpeg, err)
	}

	const callers = 8
	var wait sync.WaitGroup
	errorsByCaller := make(chan error, callers)
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			status, err := db.Browse().RequestVideoPlayback(ctx, proxy.UUID, "6.1", "", now.Add(3*time.Minute))
			if err == nil && (status.Mode != string(mediaprocessing.PlaybackRemux) || status.Status != gallery.ProcessingPending) {
				t.Errorf("proxy status = %#v", status)
			}
			errorsByCaller <- err
		}()
	}
	wait.Wait()
	close(errorsByCaller)
	for err := range errorsByCaller {
		if err != nil {
			t.Fatalf("concurrent proxy request: %v", err)
		}
	}
	var proxyJobs int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE item_uuid=? AND variant=?`, proxy.UUID, mediaprocessing.VariantVideoPlayback).Scan(&proxyJobs); err != nil || proxyJobs != 1 {
		t.Fatalf("proxy jobs = %d, %v", proxyJobs, err)
	}
}

func publishVideoMetadata(t *testing.T, db *Database, item gallery.Item, profile, container, videoCodec, audioCodec string, rotation int, hdr bool, now time.Time) {
	t.Helper()
	if err := db.VideoMetadata().MarkPending(context.Background(), item.UUID, item.ContentRevision, profile); err != nil {
		t.Fatal(err)
	}
	audioIndex := 1
	metadata := mediaprocessing.VideoTechnicalMetadata{ItemUUID: item.UUID, ContentRevision: item.ContentRevision, ProbeProfileHash: profile,
		Container: container, VideoStreamIndex: 0, VideoCodec: videoCodec, AudioStreamIndex: &audioIndex, AudioCodec: audioCodec,
		DisplayWidth: 1920, DisplayHeight: 1080, Rotation: rotation, HDR: hdr}
	if audioCodec == "" {
		metadata.AudioStreamIndex = nil
	}
	if err := db.VideoMetadata().PublishReady(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}
}
