package mediaprocessing

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/ffmpeg"
)

func TestProgressiveVideoSegmentsAndSeek(t *testing.T) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg unavailable")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	cmd := exec.Command(path, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "9", "-c:v", "mpeg4", "-c:a", "aac", source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, output)
	}
	audioTrack := 1
	metadata := VideoTechnicalMetadata{Container: "mp4", VideoCodec: "mpeg4", VideoStreamIndex: 0, AudioStreamIndex: &audioTrack, AudioCodec: "aac", DisplayWidth: 320, DisplayHeight: 180, DurationSeconds: 9}
	plan := PlaybackPlanFromMetadata(metadata)
	var firstTimes [2]float64
	for _, start := range []int{0, 1} {
		directory := filepath.Join(root, string(rune('0'+start)))
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		var updates int
		err := RunVideoCommand(context.Background(), ffmpeg.NewEncoder(path), ProgressiveVideoArgs(source, directory, plan, start, 4), "test-video", "HLS_ENCODE", 9, func(progress VideoProgress) {
			updates++
			if progress.Speed < 0 || progress.Percent < 0 || progress.Percent > 100 {
				t.Errorf("invalid progress: %+v", progress)
			}
		})
		if err != nil {
			t.Fatalf("start %d: %v", start, err)
		}
		if updates == 0 {
			t.Fatalf("start %d has no FFmpeg progress", start)
		}
		playlist, err := os.ReadFile(filepath.Join(directory, "internal.m3u8"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(playlist), "#EXT-X-ENDLIST") {
			t.Fatalf("start %d playlist incomplete", start)
		}
		for index := start; index < 3; index++ {
			name := filepath.Join(directory, "segment-"+formatSegment(index)+".ts")
			info, err := os.Stat(name)
			if err != nil || info.Size() == 0 {
				t.Fatalf("start %d missing segment %s: %v", start, name, err)
			}
		}
		if probe, err := exec.LookPath("ffprobe"); err == nil {
			out, err := exec.Command(probe, "-v", "error", "-show_entries", "format=start_time", "-of", "default=noprint_wrappers=1:nokey=1", filepath.Join(directory, "segment-"+formatSegment(start)+".ts")).Output()
			if err != nil {
				t.Fatal(err)
			}
			firstTimes[start], err = strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
			if err != nil {
				t.Fatal(err)
			}
		}
		if start == 0 {
			proxy := filepath.Join(root, "proxy.mp4")
			err := RunVideoCommand(context.Background(), ffmpeg.NewEncoder(path), []string{"-hide_banner", "-loglevel", "error", "-y", "-i", filepath.Join(directory, "internal.m3u8"), "-map", "0:v:0", "-map", "0:a:0?", "-c", "copy", "-movflags", "+faststart", "-f", "mp4", proxy}, "test-video", "HLS_MP4_REMUX", 9, nil)
			if err != nil {
				t.Fatalf("promote HLS to MP4: %v", err)
			}
			if info, err := os.Stat(proxy); err != nil || info.Size() == 0 {
				t.Fatalf("missing promoted MP4: %v", err)
			}
			if probe, err := exec.LookPath("ffprobe"); err == nil {
				output, err := exec.Command(probe, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", proxy).Output()
				if err != nil {
					t.Fatal(err)
				}
				seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
				if err != nil || seconds < 8.5 || seconds > 9.5 {
					t.Fatalf("promoted MP4 duration = %v, %v", seconds, err)
				}
			}
		}
	}
	if firstTimes[0] > 0 && firstTimes[1] > 0 && (firstTimes[1]-firstTimes[0] < 3.5 || firstTimes[1]-firstTimes[0] > 4.5) {
		t.Fatalf("seek segment timestamps not aligned: %v", firstTimes)
	}
}

func formatSegment(index int) string { return strings.Repeat("0", 5) + string(rune('0'+index)) }

func TestProgressiveVideoFirstSegmentPrecedesCompletion(t *testing.T) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg unavailable")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	if output, err := exec.Command(path, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=12", "-t", "7", "-an", "-c:v", "mpeg4", source).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, output)
	}
	directory := filepath.Join(root, "segments")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	metadata := VideoTechnicalMetadata{Container: "mp4", VideoCodec: "mpeg4", VideoStreamIndex: 0, DisplayWidth: 160, DisplayHeight: 90, DurationSeconds: 7}
	args := ProgressiveVideoArgs(source, directory, PlaybackPlanFromMetadata(metadata), 0, 2)
	for index, value := range args {
		if value == "-i" {
			args = append(args[:index], append([]string{"-re"}, args[index:]...)...)
			break
		}
	}
	done := make(chan error, 1)
	go func() {
		done <- RunVideoCommand(context.Background(), ffmpeg.NewEncoder(path), args, "test-video", "HLS_ENCODE", 7, nil)
	}()
	deadline := time.After(6 * time.Second)
	for {
		if info, err := os.Stat(filepath.Join(directory, "segment-000000.ts")); err == nil && info.Size() > 0 {
			select {
			case err := <-done:
				t.Fatalf("whole transcode finished before first segment was observed: %v", err)
			default:
			}
			break
		}
		select {
		case err := <-done:
			t.Fatalf("transcode finished before first segment: %v", err)
		case <-deadline:
			t.Fatal("first segment was not published in time")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestProgressiveVideoNVENCArgumentsArePlanBounded(t *testing.T) {
	plan := VideoPlaybackPlan{Mode: PlaybackTranscode, SelectVideoTrack: 0, SelectAudioTrack: -1, MaximumWidth: 1920, MaximumHeight: 1080}
	execution := VideoTranscodeExecutionPlan{Workload: VideoTranscodeHLS, EffectiveBackend: "NVENC", Device: "nvidia0", Decoder: "hevc_cuvid", FilterStrategy: "CUDA", Encoder: "h264_nvenc", RateControl: nvencHLSRateControl, Executable: true}
	args, err := ProgressiveVideoArgsForExecution("source.mp4", "segments", plan, execution, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, required := range []string{"-hwaccel cuda", "-hwaccel_device 0", "-hwaccel_output_format cuda", "-c:v hevc_cuvid", "scale_cuda=", "-c:v h264_nvenc", "-cq 25", "-b:v 3M", "-maxrate 5M", "-bufsize 10M", "-forced-idr 1"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("arguments lack %q: %s", required, joined)
		}
	}
	if strings.Contains(joined, "libx264") {
		t.Fatalf("hardware arguments contain software encoder: %s", joined)
	}
	execution.Device = "../../dev/nvidia0"
	if _, err := ProgressiveVideoArgsForExecution("source.mp4", "segments", plan, execution, 0, 4); err == nil {
		t.Fatal("unsafe NVIDIA device was accepted")
	}
	execution.Device, execution.RateControl = "nvidia0", "unreviewed"
	if _, err := ProgressiveVideoArgsForExecution("source.mp4", "segments", plan, execution, 0, 4); err == nil {
		t.Fatal("unreviewed NVENC rate control was accepted")
	}
	execution = VideoTranscodeExecutionPlan{EffectiveBackend: "VAAPI", Encoder: "h264_vaapi", Executable: true}
	if _, err := ProgressiveVideoArgsForExecution("source.mp4", "segments", plan, execution, 0, 4); err == nil {
		t.Fatal("incomplete VAAPI plan was accepted")
	}
}

func TestProgressiveVideoVAAPIArgumentsArePlanBounded(t *testing.T) {
	plan := VideoPlaybackPlan{Mode: PlaybackTranscode, SelectVideoTrack: 0, SelectAudioTrack: -1, MaximumWidth: 1920, MaximumHeight: 1080}
	execution := VideoTranscodeExecutionPlan{Workload: VideoTranscodeHLS, EffectiveBackend: "VAAPI", Device: "renderD128", Decoder: "hevc", FilterStrategy: "VAAPI", Encoder: "h264_vaapi", RateControl: vaapiRateControl, Executable: true}
	args, err := ProgressiveVideoArgsForSchedule("source.mp4", "segments", plan, execution, 0, ProgressiveSegmentSchedule{FirstSeconds: 2, FollowingSeconds: 4})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, required := range []string{"-init_hw_device vaapi=va:/dev/dri/renderD128", "-filter_hw_device va", "-hwaccel vaapi", "-hwaccel_device va", "-hwaccel_output_format vaapi", "scale_vaapi=", "-c:v h264_vaapi", "-rc_mode CQP", "-qp 25", "-quality 4", "-force_key_frames expr:gte(t,2+n_forced*4)"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("VAAPI arguments lack %q: %s", required, joined)
		}
	}
	execution.FilterStrategy = "VAAPI_CPU_SCALE"
	hybridArgs, err := ProgressiveVideoArgsForSchedule("source.mp4", "segments", plan, execution, 0, ProgressiveSegmentSchedule{FirstSeconds: 2, FollowingSeconds: 4})
	if err != nil {
		t.Fatal(err)
	}
	hybrid := strings.Join(hybridArgs, " ")
	for _, required := range []string{"hwdownload", "scale=w=", "flags=fast_bilinear", "hwupload", "-c:v h264_vaapi"} {
		if !strings.Contains(hybrid, required) {
			t.Fatalf("VAAPI hybrid arguments lack %q: %s", required, hybrid)
		}
	}
	execution.Device = "/dev/dri/renderD128"
	if _, err := ProgressiveVideoArgsForExecution("source.mp4", "segments", plan, execution, 0, 4); err == nil {
		t.Fatal("absolute VAAPI device was accepted")
	}
}

func TestProgressiveSegmentScheduleAndArguments(t *testing.T) {
	schedule := ProgressiveSegmentSchedule{FirstSeconds: 2, FollowingSeconds: 4}
	if err := schedule.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := []int{schedule.Start(0), schedule.Start(1), schedule.Start(2), schedule.Start(7)}; got[0] != 0 || got[1] != 2 || got[2] != 6 || got[3] != 26 {
		t.Fatalf("segment starts = %v", got)
	}
	if got := schedule.Count(9); got != 3 {
		t.Fatalf("segment count = %d", got)
	}
	if got := []float64{schedule.Duration(9, 0), schedule.Duration(9, 1), schedule.Duration(9, 2)}; got[0] != 2 || got[1] != 4 || got[2] != 3 {
		t.Fatalf("segment durations = %v", got)
	}
	if got := []int{schedule.IndexAt(0), schedule.IndexAt(1.99), schedule.IndexAt(2), schedule.IndexAt(5.99), schedule.IndexAt(6)}; got[0] != 0 || got[1] != 0 || got[2] != 1 || got[3] != 1 || got[4] != 2 {
		t.Fatalf("segment indexes = %v", got)
	}
	plan := VideoPlaybackPlan{Mode: PlaybackTranscode, SelectVideoTrack: 0, SelectAudioTrack: -1, MaximumWidth: 1920, MaximumHeight: 1080}
	execution := VideoTranscodeExecutionPlan{Workload: VideoTranscodeHLS, EffectiveBackend: "SOFTWARE", FilterStrategy: "CPU", Encoder: "libx264", RateControl: "veryfast-crf20", Executable: true}
	initial, err := ProgressiveVideoArgsForSchedule("source.mp4", "segments", plan, execution, 0, schedule)
	if err != nil {
		t.Fatal(err)
	}
	initialArgs := strings.Join(initial, " ")
	for _, expected := range []string{"-force_key_frames expr:gte(t,2+n_forced*4)", "-hls_time 2", "-start_number 0"} {
		if !strings.Contains(initialArgs, expected) {
			t.Fatalf("initial arguments lack %q: %s", expected, initialArgs)
		}
	}
	seek, err := ProgressiveVideoArgsForSchedule("source.mp4", "segments", plan, execution, 3, schedule)
	if err != nil {
		t.Fatal(err)
	}
	seekArgs := strings.Join(seek, " ")
	for _, expected := range []string{"-ss 10", "-force_key_frames expr:gte(t,n_forced*4)", "-output_ts_offset 10", "-hls_time 4", "-start_number 3"} {
		if !strings.Contains(seekArgs, expected) {
			t.Fatalf("seek arguments lack %q: %s", expected, seekArgs)
		}
	}
}

func TestProgressiveVideoProfileSeparatesSoftwareAndNVENC(t *testing.T) {
	metadata := VideoTechnicalMetadata{VideoCodec: "hevc", AudioCodec: "aac", PixelFormat: "yuv420p", VideoStreamIndex: 0, DisplayWidth: 2160, DisplayHeight: 3840}
	plan := PlaybackPlanFromMetadata(metadata)
	software := PlanVideoTranscode(plan, metadata, VideoHardwarePreference{Mode: "SOFTWARE", AllowSoftwareFallback: true}, HardwareAccelerationStatus{}, VideoTranscodeHLS)
	nvenc := PlanVideoTranscode(plan, metadata, VideoHardwarePreference{Mode: "NVENC", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeHLS)
	if ProgressiveVideoProfileHash(metadata, plan, "7.1", software, 4, 2) == ProgressiveVideoProfileHash(metadata, plan, "7.1", nvenc, 4, 2) {
		t.Fatal("software and NVENC HLS profiles must differ")
	}
	if ProgressiveVideoProfileHash(metadata, plan, "7.1", software, 4, 2) == ProgressiveVideoProfileHashForSchedule(metadata, plan, "7.1", software, ProgressiveSegmentSchedule{FirstSeconds: 2, FollowingSeconds: 4}, 2) {
		t.Fatal("HA-05 segment schedule must receive a new profile")
	}
	vaapi := PlanVideoTranscode(plan, metadata, VideoHardwarePreference{Mode: "VAAPI", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeHLS)
	if ProgressiveVideoProfileHashForSchedule(metadata, plan, "7.1", vaapi, ProgressiveSegmentSchedule{FirstSeconds: 2, FollowingSeconds: 4}, 2) == ProgressiveVideoProfileHashForSchedule(metadata, plan, "7.1", nvenc, ProgressiveSegmentSchedule{FirstSeconds: 2, FollowingSeconds: 4}, 2) {
		t.Fatal("VAAPI and NVENC HLS profiles must differ")
	}
}

func TestProgressiveVideoVAAPIExternal(t *testing.T) {
	device := os.Getenv("CGM_TEST_VAAPI_DEVICE")
	if device == "" {
		t.Skip("set CGM_TEST_VAAPI_DEVICE to a render node such as renderD128")
	}
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	if output, err := exec.Command(path, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "7", "-an", "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error:pools=1:frame-threads=1", "-pix_fmt", "yuv420p", source).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, output)
	}
	plan := VideoPlaybackPlan{Mode: PlaybackTranscode, SelectVideoTrack: 0, SelectAudioTrack: -1, MaximumWidth: 1920, MaximumHeight: 1080}
	execution := VideoTranscodeExecutionPlan{Workload: VideoTranscodeHLS, EffectiveBackend: "VAAPI", Device: device, Decoder: "hevc", FilterStrategy: "VAAPI_CPU_SCALE", Encoder: "h264_vaapi", RateControl: vaapiRateControl, Executable: true}
	schedule := ProgressiveSegmentSchedule{FirstSeconds: 2, FollowingSeconds: 4}
	for _, start := range []int{0, 1} {
		directory := filepath.Join(root, "segments-"+strconv.Itoa(start))
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		args, err := ProgressiveVideoArgsForSchedule(source, directory, plan, execution, start, schedule)
		if err != nil {
			t.Fatal(err)
		}
		if err := RunVideoCommand(context.Background(), ffmpeg.NewEncoder(path), args, "vaapi-test", "HLS_ENCODE", 7, nil); err != nil {
			t.Fatal(err)
		}
		if info, err := os.Stat(filepath.Join(directory, "segment-"+formatSegment(start)+".ts")); err != nil || info.Size() == 0 {
			t.Fatalf("VAAPI segment %d unavailable: %v", start, err)
		}
		if start == 0 {
			manifest, err := os.ReadFile(filepath.Join(directory, "internal.m3u8"))
			if err != nil || !strings.Contains(string(manifest), "#EXTINF:2.000000") || !strings.Contains(string(manifest), "#EXTINF:4.000000") {
				t.Fatalf("VAAPI schedule manifest = %q, %v", manifest, err)
			}
		}
	}
}

func TestProgressiveVideoNVENCExternal(t *testing.T) {
	if os.Getenv("CGM_TEST_NVENC") != "1" {
		t.Skip("set CGM_TEST_NVENC=1 on an NVIDIA test host")
	}
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	if output, err := exec.Command(path, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "7", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", source).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, output)
	}
	plan := VideoPlaybackPlan{Mode: PlaybackTranscode, SelectVideoTrack: 0, SelectAudioTrack: -1, MaximumWidth: 1920, MaximumHeight: 1080}
	execution := VideoTranscodeExecutionPlan{Workload: VideoTranscodeHLS, EffectiveBackend: "NVENC", Device: "nvidia0", Decoder: "h264_cuvid", FilterStrategy: "CUDA", Encoder: "h264_nvenc", RateControl: nvencHLSRateControl, Executable: true}
	schedule := ProgressiveSegmentSchedule{FirstSeconds: 2, FollowingSeconds: 4}
	for _, start := range []int{0, 1} {
		directory := filepath.Join(root, "segments-"+strconv.Itoa(start))
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		args, err := ProgressiveVideoArgsForSchedule(source, directory, plan, execution, start, schedule)
		if err != nil {
			t.Fatal(err)
		}
		if err := RunVideoCommand(context.Background(), ffmpeg.NewEncoder(path), args, "nvenc-test", "HLS_ENCODE", 7, nil); err != nil {
			t.Fatal(err)
		}
		if info, err := os.Stat(filepath.Join(directory, "segment-"+formatSegment(start)+".ts")); err != nil || info.Size() == 0 {
			t.Fatalf("NVENC segment %d unavailable: %v", start, err)
		}
		if start == 0 {
			manifest, err := os.ReadFile(filepath.Join(directory, "internal.m3u8"))
			if err != nil || !strings.Contains(string(manifest), "#EXTINF:2.000000") || !strings.Contains(string(manifest), "#EXTINF:4.000000") {
				t.Fatalf("NVENC schedule manifest = %q, %v", manifest, err)
			}
		}
	}
}
