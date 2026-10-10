package mediaprocessing

import (
	"context"
	"errors"
	"os"

	"github.com/disintegration/imaging"
	"github.com/stashapp/stash/internal/gallery"
	_ "golang.org/x/image/webp"
)

// ImageGenerator reuses the image stack already shipped by Stash for common
// JPEG/PNG/WebP and first-frame GIF output. AVIF/JXL/RAW are deliberately left
// to dependency-backed generators instead of silently falling back to the
// original file.
type ImageGenerator struct{}

func (ImageGenerator) Supports(request GenerateRequest) bool {
	if request.ContentFormat != gallery.ContentFormatImage {
		return false
	}
	if request.MediaKind == gallery.MediaKindStaticImage {
		return request.Variant == VariantCard480 || request.Variant == VariantCard960 ||
			request.Variant == VariantCard1600 || request.Variant == VariantLightbox4096
	}
	return request.MediaKind == gallery.MediaKindAnimatedImage && request.Variant == VariantStaticPoster
}

func (ImageGenerator) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	if !(ImageGenerator{}).Supports(request) {
		return GenerateResult{}, errors.New("image generator does not support request")
	}
	if err := ctx.Err(); err != nil {
		return GenerateResult{}, err
	}
	image, err := imaging.Open(request.SourcePath, imaging.AutoOrientation(true))
	if err != nil {
		return GenerateResult{}, err
	}
	maximum := variantMaximum(request.Variant)
	if image.Bounds().Dx() > maximum || image.Bounds().Dy() > maximum {
		image = imaging.Fit(image, maximum, maximum, imaging.Lanczos)
	}
	if err := ctx.Err(); err != nil {
		return GenerateResult{}, err
	}
	if err := imaging.Save(image, request.DestinationPath, imaging.JPEGQuality(90)); err != nil {
		return GenerateResult{}, err
	}
	info, err := os.Stat(request.DestinationPath)
	if err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{MIMEType: "image/jpeg", Width: image.Bounds().Dx(), Height: image.Bounds().Dy(), ByteSize: info.Size()}, nil
}

func variantMaximum(variant string) int {
	switch variant {
	case VariantCard480:
		return 480
	case VariantCard960:
		return 960
	case VariantCard1600:
		return 1600
	default:
		return 4096
	}
}
