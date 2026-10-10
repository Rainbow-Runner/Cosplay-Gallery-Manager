package mediaprocessing

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/pkg/ffmpeg"
)

func TestAnimatedPreviewArgsKeepCompleteDurationAndBoundAnimation(t *testing.T) {
	request := GenerateRequest{MediaKind: gallery.MediaKindAnimatedImage, ContentFormat: gallery.ContentFormatImage,
		Variant: VariantAnimatedPreview, SourcePath: "source.gif", DestinationPath: "preview.webp"}
	args := animatedPreviewArgs(request)
	joined := " " + strings.Join(args, " ") + " "
	if strings.Contains(joined, " -t ") || strings.Contains(joined, " -to ") || strings.Contains(joined, " -frames:v ") {
		t.Fatalf("animated preview unexpectedly truncates duration: %v", args)
	}
	for _, expected := range []string{"fps=15", "min(480,iw)", "libwebp_anim", " -loop 0 ", "preview.webp"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("animated preview args missing %q: %v", expected, args)
		}
	}
}

func TestAnimatedPreviewGeneratorCreatesAnimatedWebPWithFFmpeg(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source.gif")
	destination := filepath.Join(root, "preview.webp")
	palette := color.Palette{color.Black, color.White}
	frames := make([]*image.Paletted, 20)
	delays := make([]int, len(frames))
	for index := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, 600, 300), palette)
		frame.Pix[index%len(frame.Pix)] = 1
		frames[index], delays[index] = frame, 10
	}
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(file, &gif.GIF{Image: frames, Delay: delays, LoopCount: 0}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := (AnimatedPreviewGenerator{Encoder: ffmpeg.NewEncoder(ffmpegPath)}).Generate(context.Background(), GenerateRequest{
		MediaKind: gallery.MediaKindAnimatedImage, ContentFormat: gallery.ContentFormatImage, Variant: VariantAnimatedPreview,
		SourcePath: source, DestinationPath: destination,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if result.MIMEType != "image/webp" || result.Width > AnimatedPreviewMaximum || result.Height > AnimatedPreviewMaximum || !bytes.Contains(data, []byte("ANIM")) {
		t.Fatalf("animated preview result = %#v, bytes=%d", result, len(data))
	}
	sourceDuration := 0.0
	for _, delay := range delays {
		sourceDuration += float64(delay) / 100
	}
	previewDuration := float64(webpAnimationDurationMilliseconds(data)) / 1000
	if previewDuration <= 0 || math.Abs(sourceDuration-previewDuration) > 0.11 {
		t.Fatalf("preview duration %.3fs does not retain source duration %.3fs", previewDuration, sourceDuration)
	}
}

func webpAnimationDurationMilliseconds(value []byte) int {
	if len(value) < 12 || !bytes.Equal(value[:4], []byte("RIFF")) || !bytes.Equal(value[8:12], []byte("WEBP")) {
		return 0
	}
	total := 0
	for offset := 12; offset+8 <= len(value); {
		size := int(binary.LittleEndian.Uint32(value[offset+4 : offset+8]))
		payload := offset + 8
		if size < 0 || payload+size > len(value) {
			return 0
		}
		if string(value[offset:offset+4]) == "ANMF" && size >= 16 {
			total += int(value[payload+12]) | int(value[payload+13])<<8 | int(value[payload+14])<<16
		}
		offset = payload + size + size%2
	}
	return total
}
