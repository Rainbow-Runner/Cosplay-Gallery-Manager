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
