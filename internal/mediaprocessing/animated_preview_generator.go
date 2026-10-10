package mediaprocessing

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/pkg/ffmpeg"
	_ "golang.org/x/image/webp"
)

const (
	AnimatedPreviewMaximum = 480
	AnimatedPreviewFPS     = 15
)

// AnimatedPreviewGenerator creates a complete-duration, bounded animated WebP.
// There is deliberately no -t argument: frame-rate reduction must not shorten
// the source animation.
type AnimatedPreviewGenerator struct{ Encoder *ffmpeg.FFMpeg }

func (generator AnimatedPreviewGenerator) Supports(request GenerateRequest) bool {
	return generator.Encoder != nil && request.MediaKind == gallery.MediaKindAnimatedImage &&
		request.ContentFormat == gallery.ContentFormatImage && request.Variant == VariantAnimatedPreview
}

func (generator AnimatedPreviewGenerator) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	if !generator.Supports(request) {
		return GenerateResult{}, errors.New("animated preview generator does not support request")
	}
	args := animatedPreviewArgs(request)
	if output, err := generator.Encoder.Command(ctx, args).CombinedOutput(); err != nil {
		return GenerateResult{}, fmt.Errorf("animated preview generation failed: %w: %s", err, string(output))
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
	return GenerateResult{MIMEType: "image/webp", Width: config.Width, Height: config.Height, ByteSize: info.Size()}, nil
}

func animatedPreviewArgs(request GenerateRequest) ffmpeg.Args {
	filter := fmt.Sprintf("fps=%d,scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:flags=lanczos", AnimatedPreviewFPS, AnimatedPreviewMaximum, AnimatedPreviewMaximum)
	return ffmpeg.Args{"-hide_banner", "-loglevel", "error", "-y", "-i", request.SourcePath,
		"-map_metadata", "-1", "-an", "-sn", "-dn", "-vf", filter, "-c:v", "libwebp_anim",
		"-quality", "80", "-compression_level", "4", "-loop", "0", "-f", "webp", request.DestinationPath}
}
