package mediaprocessing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/disintegration/imaging"
	"github.com/stashapp/stash/internal/gallery"
)

// LibRawGenerator invokes the configured LibRaw dcraw-compatible executable
// directly (never through a shell). RAW originals remain read-only and only a
// bounded sRGB JPEG proxy is published.
type LibRawGenerator struct{ Executable string }

func (generator LibRawGenerator) Supports(request GenerateRequest) bool {
	return generator.Executable != "" && request.MediaKind == gallery.MediaKindStaticImage && request.ContentFormat == gallery.ContentFormatRAW &&
		(request.Variant == VariantLightbox4096 || request.Variant == VariantCard480 || request.Variant == VariantCard960 || request.Variant == VariantCard1600)
}

func (generator LibRawGenerator) Validate() error {
	if generator.Executable == "" {
		return errors.New("LibRaw decoder executable is not configured")
	}
	path, err := exec.LookPath(generator.Executable)
	if err != nil {
		return fmt.Errorf("LibRaw decoder unavailable: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("LibRaw decoder is not a regular executable")
	}
	return nil
}

func (generator LibRawGenerator) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	if !generator.Supports(request) {
		return GenerateResult{}, errors.New("LibRaw generator does not support request")
	}
	command := exec.CommandContext(ctx, generator.Executable, "-T", "-c", "-w", "-W", "-o", "1", "-q", "3", request.SourcePath)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return GenerateResult{}, err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return GenerateResult{}, err
	}
	decoded, decodeErr := imaging.Decode(stdout, imaging.AutoOrientation(true))
	if decodeErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return GenerateResult{}, fmt.Errorf("LibRaw output decode failed: %w", decodeErr)
	}
	if err := command.Wait(); err != nil {
		return GenerateResult{}, fmt.Errorf("LibRaw decode failed: %w (%s)", err, stderr.String())
	}
	maximum := variantMaximum(request.Variant)
	if decoded.Bounds().Dx() > maximum || decoded.Bounds().Dy() > maximum {
		decoded = imaging.Fit(decoded, maximum, maximum, imaging.Lanczos)
	}
	if err := imaging.Save(decoded, request.DestinationPath, imaging.JPEGQuality(90)); err != nil {
		return GenerateResult{}, err
	}
	info, err := os.Stat(request.DestinationPath)
	if err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{MIMEType: "image/jpeg", Width: decoded.Bounds().Dx(), Height: decoded.Bounds().Dy(), ByteSize: info.Size()}, nil
}
