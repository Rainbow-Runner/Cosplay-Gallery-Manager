package mediaprocessing

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/ffmpeg/transcoder"
)

// FFmpegPosterGenerator is the adapter around Stash's existing FFmpeg command
// builder. It generates static JPEG resources only; it never starts animation
// or video playback for a Scrubber request.
type FFmpegPosterGenerator struct{ Encoder *ffmpeg.FFMpeg }

func (generator FFmpegPosterGenerator) Supports(request GenerateRequest) bool {
	if generator.Encoder == nil {
		return false
	}
	if request.Variant != VariantStaticPoster && request.Variant != VariantCard480 && request.Variant != VariantCard960 && request.Variant != VariantCard1600 && request.Variant != VariantLightbox4096 {
		return false
	}
	return request.MediaKind == gallery.MediaKindVideo || request.MediaKind == gallery.MediaKindAnimatedImage || request.ContentFormat == gallery.ContentFormatRAW
}

func (generator FFmpegPosterGenerator) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	if !generator.Supports(request) {
		return GenerateResult{}, errors.New("FFmpeg Poster generator does not support request")
	}
	maximum := variantMaximum(request.Variant)
	if request.MediaKind == gallery.MediaKindVideo && request.Variant == VariantStaticPoster {
		maximum = 960
	}
	var args ffmpeg.Args
	if request.MediaKind == gallery.MediaKindVideo {
		seek := 0.0
		if request.VideoTechnical != nil {
			seek = PosterTimestamp(request.VideoTechnical.DurationSeconds)
		}
		attempts := []struct {
			time float64
			slow bool
		}{{seek, false}, {seek, true}}
		if seek > 0 {
			attempts = append(attempts, struct {
				time float64
				slow bool
			}{0, true})
		}
		var err error
		for _, attempt := range attempts {
			args = videoPosterArgs(request, attempt.time, attempt.slow, maximum)
			_, err = generator.Encoder.Command(ctx, args).CombinedOutput()
			if err == nil {
				break
			}
		}
		if err != nil {
			if request.VideoTechnical != nil && request.VideoTechnical.HDR {
				return GenerateResult{}, &VideoProcessingError{Code: ErrorVideoToneMapUnavailable, Err: errors.New("FFmpeg HDR-to-SDR poster filter failed")}
			}
			return GenerateResult{}, fmt.Errorf("FFmpeg Poster generation failed: %w", err)
		}
	} else {
		args = transcoder.ImageThumbnail(request.SourcePath, transcoder.ImageThumbnailOptions{OutputFormat: ffmpeg.ImageFormatJpeg, OutputPath: request.DestinationPath, MaxDimensions: maximum, Quality: 3})
		if _, err := generator.Encoder.Command(ctx, args).CombinedOutput(); err != nil {
			return GenerateResult{}, fmt.Errorf("FFmpeg Poster generation failed: %w", err)
		}
	}
	file, err := os.Open(request.DestinationPath)
	if err != nil {
		return GenerateResult{}, err
	}
	config, _, decodeErr := image.DecodeConfig(file)
	_ = file.Close()
	if decodeErr != nil {
		return GenerateResult{}, decodeErr
	}
	info, err := os.Stat(request.DestinationPath)
	if err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{MIMEType: "image/jpeg", Width: config.Width, Height: config.Height, ByteSize: info.Size()}, nil
}

func videoPosterArgs(request GenerateRequest, seek float64, accurate bool, maximum int) ffmpeg.Args {
	args := ffmpeg.Args{"-hide_banner", "-loglevel", "error", "-y", "-noautorotate"}
	if !accurate && seek > 0 {
		args = append(args, "-ss", strconv.FormatFloat(seek, 'f', 3, 64))
	}
	args = append(args, "-i", request.SourcePath)
	if accurate && seek > 0 {
		args = append(args, "-ss", strconv.FormatFloat(seek, 'f', 3, 64))
	}
	if request.VideoTechnical != nil {
		args = append(args, "-map", "0:"+strconv.Itoa(request.VideoTechnical.VideoStreamIndex))
	}
	filters := []string{}
	if request.VideoTechnical != nil {
		switch ((request.VideoTechnical.Rotation % 360) + 360) % 360 {
		case 90:
			filters = append(filters, "transpose=clock")
		case 180:
			filters = append(filters, "hflip", "vflip")
		case 270:
			filters = append(filters, "transpose=cclock")
		}
		if request.VideoTechnical.HDR {
			filters = append(filters, "zscale=t=linear:npl=100", "format=gbrpf32le", "tonemap=mobius", "zscale=p=bt709:t=bt709:m=bt709:r=tv", "format=yuv420p")
		}
	}
	filters = append(filters, fmt.Sprintf("scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease", maximum, maximum))
	return append(args, "-vf", strings.Join(filters, ","), "-frames:v", "1", "-an", "-sn", "-dn", "-q:v", "3", "-f", "image2", request.DestinationPath)
}

func PosterTimestamp(duration float64) float64 {
	if duration <= 0 {
		return 0
	}
	value := duration * 0.2
	maximum := duration - 0.05
	if maximum < 0 {
		maximum = 0
	}
	if value > maximum {
		value = maximum
	}
	if value < 0 {
		return 0
	}
	return value
}
