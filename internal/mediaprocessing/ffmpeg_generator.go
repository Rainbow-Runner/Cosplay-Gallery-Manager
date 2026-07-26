package mediaprocessing

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"

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
	var args ffmpeg.Args
	if request.MediaKind == gallery.MediaKindVideo {
		args = transcoder.ScreenshotTime(request.SourcePath, 0, transcoder.ScreenshotOptions{OutputPath: request.DestinationPath, OutputType: transcoder.ScreenshotOutputTypeImage2, Quality: 3, Width: maximum})
	} else {
		args = transcoder.ImageThumbnail(request.SourcePath, transcoder.ImageThumbnailOptions{OutputFormat: ffmpeg.ImageFormatJpeg, OutputPath: request.DestinationPath, MaxDimensions: maximum, Quality: 3})
	}
	output, err := generator.Encoder.Command(ctx, args).CombinedOutput()
	if err != nil {
		return GenerateResult{}, fmt.Errorf("FFmpeg Poster generation failed: %w (%s)", err, string(output))
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
