package mediaprocessing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/pkg/ffmpeg"
)

type VideoPlaybackGenerator struct{ Encoder *ffmpeg.FFMpeg }

func (generator VideoPlaybackGenerator) Supports(request GenerateRequest) bool {
	return generator.Encoder != nil && request.MediaKind == gallery.MediaKindVideo && request.ContentFormat == gallery.ContentFormatVideo && request.Variant == VariantVideoPlayback && request.VideoTechnical != nil && request.VideoPlan != nil && request.VideoPlan.Mode != PlaybackDirect
}

func (generator VideoPlaybackGenerator) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	if !generator.Supports(request) {
		return GenerateResult{}, errors.New("video playback generator does not support request")
	}
	plan := *request.VideoPlan
	args := ffmpeg.Args{"-hide_banner", "-loglevel", "error", "-y"}
	if plan.ApplyRotation {
		args = append(args, "-noautorotate")
	}
	args = append(args, "-i", request.SourcePath, "-map", "0:"+strconv.Itoa(plan.SelectVideoTrack))
	if plan.SelectAudioTrack >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(plan.SelectAudioTrack))
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn")
	if plan.Mode == PlaybackRemux {
		args = append(args, "-c:v", "copy")
	} else {
		filters := videoFilters(plan)
		if filters != "" {
			args = append(args, "-vf", filters)
		}
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-profile:v", "high", "-pix_fmt", "yuv420p")
	}
	if plan.SelectAudioTrack >= 0 {
		if plan.CopyAudio {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "192k")
		}
	}
	args = append(args, "-metadata:s:v:0", "rotate=0", "-movflags", "+faststart", "-f", "mp4", request.DestinationPath)
	if _, err := generator.Encoder.Command(ctx, args).CombinedOutput(); err != nil {
		if plan.ToneMapHDRToSDR {
			return GenerateResult{}, &VideoProcessingError{Code: ErrorVideoToneMapUnavailable, Err: errors.New("FFmpeg HDR-to-SDR playback filter failed")}
		}
		return GenerateResult{}, fmt.Errorf("video playback generation failed: %w", err)
	}
	info, err := os.Stat(request.DestinationPath)
	if err != nil {
		return GenerateResult{}, err
	}
	width, height := fitVideoDimensions(request.VideoTechnical.DisplayWidth, request.VideoTechnical.DisplayHeight, plan.MaximumWidth, plan.MaximumHeight)
	return GenerateResult{MIMEType: "video/mp4", Width: width, Height: height, ByteSize: info.Size()}, nil
}

func fitVideoDimensions(width, height, maximumWidth, maximumHeight int) (int, int) {
	if width <= 0 || height <= 0 || maximumWidth <= 0 || maximumHeight <= 0 || (width <= maximumWidth && height <= maximumHeight) {
		return width, height
	}
	if int64(maximumWidth)*int64(height) <= int64(maximumHeight)*int64(width) {
		return maximumWidth, max(1, int(float64(height)*float64(maximumWidth)/float64(width)))
	}
	return max(1, int(float64(width)*float64(maximumHeight)/float64(height))), maximumHeight
}

func videoFilters(plan VideoPlaybackPlan) string {
	filters := []string{}
	if plan.ApplyRotation {
		switch ((plan.Rotation % 360) + 360) % 360 {
		case 90:
			filters = append(filters, "transpose=clock")
		case 180:
			filters = append(filters, "hflip", "vflip")
		case 270:
			filters = append(filters, "transpose=cclock")
		}
	}
	if plan.ToneMapHDRToSDR {
		filters = append(filters, "zscale=t=linear:npl=100", "format=gbrpf32le", "tonemap=mobius", "zscale=p=bt709:t=bt709:m=bt709:r=tv", "format=yuv420p")
	}
	filters = append(filters, fmt.Sprintf("scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease", plan.MaximumWidth, plan.MaximumHeight))
	return strings.Join(filters, ",")
}
