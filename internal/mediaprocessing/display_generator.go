package mediaprocessing

import (
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/disintegration/imaging"
	"github.com/stashapp/stash/internal/gallery"
)

// DisplayGenerator uses the same orientation and Lanczos resize as the JPEG
// generator; only the explicitly named foundation display variant uses WebP.
type DisplayGenerator struct{ Executable string }

func (g DisplayGenerator) Supports(r GenerateRequest) bool {
	return g.Executable != "" && r.MediaKind == gallery.MediaKindStaticImage &&
		r.ContentFormat == gallery.ContentFormatImage && (r.Variant == VariantCardDisplay960 || r.Variant == VariantCardDisplay1600)
}

func (g DisplayGenerator) Generate(ctx context.Context, r GenerateRequest) (GenerateResult, error) {
	if !g.Supports(r) {
		return GenerateResult{}, errors.New("unsupported display image")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return GenerateResult{}, err
	}
	img, err := imaging.Open(r.SourcePath, imaging.AutoOrientation(true))
	if err != nil {
		return GenerateResult{}, err
	}
	return encodeDisplayImage(ctx, g.Executable, img, r.Variant, r.DestinationPath)
}

// RAW and ordinary images share exactly the same resize and WebP contract;
// callers supply oriented pixels decoded by their respective source decoder.
func encodeDisplayImage(ctx context.Context, executable string, img image.Image, variant, destination string) (GenerateResult, error) {
	if err := ctx.Err(); err != nil {
		return GenerateResult{}, err
	}
	maximum := variantMaximum(variant)
	if img.Bounds().Dx() > maximum || img.Bounds().Dy() > maximum {
		img = imaging.Fit(img, maximum, maximum, imaging.Lanczos)
	}
	reader, writer := io.Pipe()
	cmd := exec.CommandContext(ctx, executable, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-threads", "1", "-i", "pipe:0",
		"-frames:v", "1", "-pix_fmt", "bgra", "-c:v", "libwebp", "-threads", "1", "-quality", "80", "-compression_level", "4", "-f", "webp", destination)
	cmd.Stdin = reader
	if err := cmd.Start(); err != nil {
		reader.Close()
		writer.Close()
		return GenerateResult{}, err
	}
	encoded := make(chan error, 1)
	go func() { err := png.Encode(writer, img); _ = writer.CloseWithError(err); encoded <- err }()
	err := cmd.Wait()
	_ = reader.Close()
	encodeErr := <-encoded
	if err != nil {
		return GenerateResult{}, err
	}
	if encodeErr != nil {
		return GenerateResult{}, encodeErr
	}
	if err := ctx.Err(); err != nil {
		return GenerateResult{}, err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{MIMEType: "image/webp", Width: img.Bounds().Dx(), Height: img.Bounds().Dy(), ByteSize: info.Size()}, nil
}
