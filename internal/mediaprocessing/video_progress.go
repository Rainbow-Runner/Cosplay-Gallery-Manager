package mediaprocessing

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/ffmpeg"
)

// These values describe generated output, never the owner's watching position.
type VideoProgress struct {
	Seconds    float64 `json:"seconds"`
	Percent    float64 `json:"percent"`
	Speed      float64 `json:"speed"`
	FPS        float64 `json:"fps"`
	ETASeconds float64 `json:"etaSeconds"`
}

// Both complete-file and progressive playback share one CPU transcode slot.
var playbackSlot = make(chan struct{}, 1)

func AcquirePlaybackSlot(ctx context.Context) (func(), error) {
	select {
	case playbackSlot <- struct{}{}:
		return func() { <-playbackSlot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func VideoStage(item, stage string, started time.Time) {
	if len(item) > 8 {
		item = item[:8]
	}
	slog.Info("CGM_VIDEO_STAGE_COMPLETED", "item", item, "stage", stage, "elapsed_ms", time.Since(started).Milliseconds())
}

func RunVideoCommand(ctx context.Context, encoder *ffmpeg.FFMpeg, args []string, item, stage string, duration float64, update func(VideoProgress)) error {
	started := time.Now()
	defer VideoStage(item, stage, started)
	args = append([]string{"-progress", "pipe:1", "-nostats", "-stats_period", "1"}, args...)
	cmd := encoder.Command(ctx, args)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	// No paths, private input URLs or arbitrary media tags enter application logs.
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		slog.Warn("CGM_VIDEO_FFMPEG_START_FAILED", "item", shortVideoItem(item), "error_type", "START")
		return errors.New("could not start video processor")
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	progress := VideoProgress{ETASeconds: -1}
	lastLog := time.Time{}
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		number, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "x"), 64)
		if math.IsNaN(number) || math.IsInf(number, 0) {
			number = 0
		}
		switch key {
		case "out_time_us":
			progress.Seconds = math.Max(0, number/1e6)
		case "fps":
			progress.FPS = number
		case "speed":
			progress.Speed = number
		case "progress":
			if duration > 0 {
				progress.Percent = math.Min(100, 100*progress.Seconds/duration)
			}
			if progress.Speed > 0 && duration > 0 {
				progress.ETASeconds = math.Max(0, (duration-progress.Seconds)/progress.Speed)
			}
			if update != nil {
				update(progress)
			}
			if time.Since(lastLog) >= 5*time.Second || value == "end" {
				id := item
				if len(id) > 8 {
					id = id[:8]
				}
				slog.Info("CGM_VIDEO_FFMPEG_PROGRESS", "item", id, "stage", stage, "elapsed_ms", time.Since(started).Milliseconds(), "output_seconds", progress.Seconds, "percent", progress.Percent, "speed", progress.Speed, "fps", progress.FPS, "eta_seconds", progress.ETASeconds)
				lastLog = time.Now()
			}
		}
	}
	if scanner.Err() != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		slog.Info("CGM_VIDEO_FFMPEG_CANCELLED", "item", shortVideoItem(item), "stage", stage, "elapsed_ms", time.Since(started).Milliseconds())
		return ctx.Err()
	}
	if scanner.Err() != nil || waitErr != nil {
		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		slog.Warn("CGM_VIDEO_FFMPEG_FAILED", "item", shortVideoItem(item), "stage", stage, "exit_code", exitCode, "progress_seconds", progress.Seconds, "elapsed_ms", time.Since(started).Milliseconds())
		return errors.New("video processor failed")
	}
	return nil
}

func shortVideoItem(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
