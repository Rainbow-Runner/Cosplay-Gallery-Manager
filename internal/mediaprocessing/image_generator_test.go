package mediaprocessing

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/gallery"
)

func TestImageGeneratorCreatesBoundedStaticJPEG(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.jpg")
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	picture := image.NewRGBA(image.Rect(0, 0, 1200, 600))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := jpeg.Encode(file, picture, nil); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	destination := filepath.Join(root, "card.jpg")
	result, err := (ImageGenerator{}).Generate(context.Background(), GenerateRequest{MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage, Variant: VariantCard480, SourcePath: source, DestinationPath: destination})
	if err != nil {
		t.Fatal(err)
	}
	if result.MIMEType != "image/jpeg" || result.Width != 480 || result.Height != 240 || result.ByteSize == 0 {
		t.Fatalf("generated image = %#v", result)
	}
	generated, err := os.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer generated.Close()
	decoded, format, err := image.Decode(generated)
	if err != nil || format != "jpeg" || decoded.Bounds().Dx() != 480 {
		t.Fatalf("decoded generated image = %s %v %v", format, decoded.Bounds(), err)
	}
}
