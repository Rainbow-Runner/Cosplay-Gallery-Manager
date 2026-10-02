package productdb

import (
	"context"
	"encoding/json"
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

func TestVideoPlaybackRuntimeFreezesNVENCPlanAndSeparatesSoftwareFallbackProfile(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC)
	created, source := createBrowseGallery(t, db, "Hardware playback", gallery.ContentRatingNonAdult, now)
	activateBrowseFixture(t, db, created.ID, now)
	item := addBrowseItem(t, db, created.ID, source.ID, "hevc.mkv", gallery.MediaKindVideo, "", gallery.AvailabilityAvailable, gallery.ProcessingReady, 1024, now)
	probeProfile := mediaprocessing.VideoProbeProfileHash("7.1")
	publishVideoMetadata(t, db, item, probeProfile, "matroska", "hevc", "aac", 0, false, now)
	hardware := mediaprocessing.HardwareAccelerationStatus{ProbeState: mediaprocessing.HardwareProbeCompleted, Backends: []mediaprocessing.HardwareBackendStatus{{Backend: "NVENC", State: mediaprocessing.HardwareProbeAvailable, Device: "nvidia0", DecodeCodecs: []string{"hevc_cuvid"}}}}
	runtime := VideoPlaybackRuntime{FFmpegVersion: "7.1", Preference: mediaprocessing.VideoHardwarePreference{Mode: "NVENC", AllowSoftwareFallback: true}, Hardware: hardware}
	status, err := db.Browse().RequestVideoPlaybackWithRuntime(ctx, item.UUID, runtime, now.Add(time.Minute))
	if err != nil || status.Status != gallery.ProcessingPending {
		t.Fatalf("NVENC request = %#v, %v", status, err)
	}
	var profile string
	var payloadJSON []byte
	if err := db.QueryRowContext(ctx, `SELECT profile_hash,payload_json FROM processing_jobs WHERE item_uuid=? AND variant='VIDEO_PLAYBACK'`, item.UUID).Scan(&profile, &payloadJSON); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Execution mediaprocessing.VideoTranscodeExecutionPlan `json:"execution"`
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil || payload.Execution.EffectiveBackend != "NVENC" || payload.Execution.Workload != mediaprocessing.VideoTranscodeMP4 {
		t.Fatalf("frozen payload = %#v, %v", payload, err)
	}
	metadata, _ := db.VideoMetadata().Find(ctx, item.UUID)
	plan := mediaprocessing.PlaybackPlanFromMetadata(metadata)
	if profile != mediaprocessing.CompleteVideoProfileHash(metadata, plan, "7.1", payload.Execution) {
		t.Fatal("job profile does not match frozen execution")
	}

	runtime.Hardware.Backends[0].State = "RUNTIME_CIRCUIT_OPEN"
	softwareStatus, err := db.Browse().RequestVideoPlaybackWithRuntime(ctx, item.UUID, runtime, now.Add(2*time.Minute))
	if err != nil || softwareStatus.Status != gallery.ProcessingPending {
		t.Fatalf("software fallback request = %#v, %v", softwareStatus, err)
	}
	var jobs int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE item_uuid=? AND variant='VIDEO_PLAYBACK'`, item.UUID).Scan(&jobs); err != nil || jobs != 2 {
		t.Fatalf("isolated hardware/software jobs = %d, %v", jobs, err)
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
