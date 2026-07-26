package mediaprocessing

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/sourcescan"
	stashffmpeg "github.com/stashapp/stash/pkg/ffmpeg"
)

func TestExternalMediaMatrix(t *testing.T) {
	ffmpegPath := requireExternalTool(t, "CGM_TEST_FFMPEG")
	libRawPath := requireExternalTool(t, "CGM_TEST_LIBRAW")
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	outputRoot := filepath.Join(root, "output")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	writeJPEGFixture(t, filepath.Join(sourceRoot, "photo.jpg"))
	writeAnimatedGIFFixture(t, filepath.Join(sourceRoot, "animation.gif"))
	writeDNGFixture(t, filepath.Join(sourceRoot, "capture.dng"))
	runExternal(t, ffmpegPath, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "1",
		"-an", "-c:v", "mpeg4", "-q:v", "5", filepath.Join(sourceRoot, "clip.mp4"))
	scan, err := sourcescan.ScanDirectory(context.Background(), sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	observations := make(map[string]sourcescan.Observation, len(scan.Observations))
	for _, observation := range scan.Observations {
		observations[observation.RelativePath] = observation
	}
	if !scan.Complete || len(observations) != 4 ||
		observations["photo.jpg"].MediaKind != gallery.MediaKindStaticImage ||
		observations["animation.gif"].MediaKind != gallery.MediaKindAnimatedImage ||
		observations["clip.mp4"].MediaKind != gallery.MediaKindVideo ||
		observations["capture.dng"].ContentFormat != gallery.ContentFormatRAW {
		t.Fatalf("real media classification = complete %v observations %#v issues %#v", scan.Complete, observations, scan.Issues)
	}

	cases := []struct {
		name        string
		generator   Generator
		request     GenerateRequest
		wantMaximum int
	}{
		{
			name:      "animated GIF first-frame poster",
			generator: ImageGenerator{},
			request: GenerateRequest{
				MediaKind: gallery.MediaKindAnimatedImage, ContentFormat: gallery.ContentFormatImage,
				Variant: VariantStaticPoster, SourcePath: filepath.Join(sourceRoot, "animation.gif"),
			},
			wantMaximum: 4096,
		},
		{
			name:      "FFmpeg video poster",
			generator: FFmpegPosterGenerator{Encoder: stashffmpeg.NewEncoder(ffmpegPath)},
			request: GenerateRequest{
				MediaKind: gallery.MediaKindVideo, ContentFormat: gallery.ContentFormatVideo,
				Variant: VariantStaticPoster, SourcePath: filepath.Join(sourceRoot, "clip.mp4"),
			},
			wantMaximum: 4096,
		},
		{
			name:      "LibRaw DNG proxy",
			generator: LibRawGenerator{Executable: libRawPath},
			request: GenerateRequest{
				MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatRAW,
				Variant: VariantCard480, SourcePath: filepath.Join(sourceRoot, "capture.dng"),
			},
			wantMaximum: 480,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			test.request.ItemUUID = "12345678-1234-4123-8123-123456789abc"
			test.request.ContentRevision = 1
			test.request.ProfileHash = DefaultProfileHash()
			test.request.DestinationPath = filepath.Join(outputRoot, safeFixtureName(test.name)+".jpg")
			if !test.generator.Supports(test.request) {
				t.Fatal("real media request was not supported")
			}
			result, err := test.generator.Generate(context.Background(), test.request)
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(test.request.DestinationPath)
			if err != nil {
				t.Fatal(err)
			}
			config, format, decodeErr := image.DecodeConfig(file)
			_ = file.Close()
			if decodeErr != nil || format != "jpeg" || result.MIMEType != "image/jpeg" ||
				config.Width <= 0 || config.Height <= 0 || config.Width > test.wantMaximum || config.Height > test.wantMaximum ||
				result.ByteSize <= 0 {
				t.Fatalf("generated media = format %q config %#v result %#v error %v", format, config, result, decodeErr)
			}
		})
	}
}

func requireExternalTool(t *testing.T, environment string) string {
	t.Helper()
	value := os.Getenv(environment)
	if value == "" {
		t.Skipf("%s is required for the external media release gate", environment)
	}
	path, err := exec.LookPath(value)
	if err != nil {
		t.Fatalf("%s: %v", environment, err)
	}
	return path
}

func runExternal(t *testing.T, executable string, arguments ...string) {
	t.Helper()
	if output, err := exec.Command(executable, arguments...).CombinedOutput(); err != nil {
		t.Fatalf("%s failed: %v\n%s", filepath.Base(executable), err, output)
	}
}

func safeFixtureName(value string) string {
	result := make([]byte, 0, len(value))
	for index := 0; index < len(value); index++ {
		if value[index] >= 'a' && value[index] <= 'z' {
			result = append(result, value[index])
		} else {
			result = append(result, '-')
		}
	}
	return string(result)
}

func writeJPEGFixture(t *testing.T, filename string) {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 96, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 96; x++ {
			picture.SetRGBA(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 3), B: 96, A: 255})
		}
	}
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, picture, &jpeg.Options{Quality: 90}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeAnimatedGIFFixture(t *testing.T, filename string) {
	t.Helper()
	palette := color.Palette{color.Black, color.RGBA{R: 230, G: 80, B: 120, A: 255}}
	first := image.NewPaletted(image.Rect(0, 0, 64, 48), palette)
	second := image.NewPaletted(image.Rect(0, 0, 64, 48), palette)
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			first.SetColorIndex(x, y, uint8((x/8+y/8)%2))
			second.SetColorIndex(x, y, uint8(1-(x/8+y/8)%2))
		}
	}
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(file, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{5, 5}, LoopCount: 0}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

type dngEntry struct {
	tag      uint16
	dataType uint16
	count    uint32
	data     []byte
	offset   uint32
}

func writeDNGFixture(t *testing.T, filename string) {
	t.Helper()
	const width, height = 64, 48
	entries := []dngEntry{
		dngLong(254, 0),
		dngLong(256, width),
		dngLong(257, height),
		dngShort(258, 16),
		dngShort(259, 1),
		dngShort(262, 32803),
		dngASCII(271, "Cosplay Gallery Manager"),
		dngASCII(272, "Synthetic DNG Fixture"),
		dngLong(273, 0),
		dngShort(274, 1),
		dngShort(277, 1),
		dngLong(278, height),
		dngLong(279, width*height*2),
		dngShort(284, 1),
		dngASCII(305, "CGM external media gate"),
		dngShorts(33421, 2, 2),
		dngBytes(33422, 0, 1, 1, 2),
		dngBytes(50706, 1, 4, 0, 0),
		dngBytes(50707, 1, 1, 0, 0),
		dngASCII(50708, "CGM Synthetic Bayer"),
		dngBytes(50710, 0, 1, 2),
		dngShort(50711, 1),
		dngShorts(50713, 1, 1),
		dngRationals(50714, [2]int32{0, 1}),
		dngLong(50717, 65535),
		dngShorts(50719, 0, 0),
		dngLongs(50720, width, height),
		dngSRationals(50721,
			[2]int32{1, 1}, [2]int32{0, 1}, [2]int32{0, 1},
			[2]int32{0, 1}, [2]int32{1, 1}, [2]int32{0, 1},
			[2]int32{0, 1}, [2]int32{0, 1}, [2]int32{1, 1}),
		dngRationals(50728, [2]int32{1, 1}, [2]int32{1, 1}, [2]int32{1, 1}),
		dngShort(50778, 21),
		dngLongs(50829, 0, 0, height, width),
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].tag < entries[right].tag })
	nextOffset := uint32(8 + 2 + len(entries)*12 + 4)
	for index := range entries {
		if len(entries[index].data) > 4 {
			entries[index].offset = nextOffset
			nextOffset += uint32(len(entries[index].data))
			if nextOffset%2 != 0 {
				nextOffset++
			}
		}
	}
	stripOffset := nextOffset
	for index := range entries {
		if entries[index].tag == 273 {
			entries[index] = dngLong(273, stripOffset)
		}
	}

	var output bytes.Buffer
	output.WriteString("II")
	writeDNGValue(&output, uint16(42))
	writeDNGValue(&output, uint32(8))
	writeDNGValue(&output, uint16(len(entries)))
	for _, entry := range entries {
		writeDNGValue(&output, entry.tag)
		writeDNGValue(&output, entry.dataType)
		writeDNGValue(&output, entry.count)
		if len(entry.data) <= 4 {
			output.Write(entry.data)
			output.Write(make([]byte, 4-len(entry.data)))
		} else {
			writeDNGValue(&output, entry.offset)
		}
	}
	writeDNGValue(&output, uint32(0))
	for _, entry := range entries {
		if len(entry.data) <= 4 {
			continue
		}
		for output.Len() < int(entry.offset) {
			output.WriteByte(0)
		}
		output.Write(entry.data)
	}
	for output.Len() < int(stripOffset) {
		output.WriteByte(0)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value := uint16(4096 + ((x*701 + y*997) % 56000))
			writeDNGValue(&output, value)
		}
	}
	if err := os.WriteFile(filename, output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func dngBytes(tag uint16, values ...byte) dngEntry {
	return dngEntry{tag: tag, dataType: 1, count: uint32(len(values)), data: append([]byte(nil), values...)}
}

func dngASCII(tag uint16, value string) dngEntry {
	data := append([]byte(value), 0)
	return dngEntry{tag: tag, dataType: 2, count: uint32(len(data)), data: data}
}

func dngShort(tag uint16, value uint16) dngEntry {
	return dngShorts(tag, value)
}

func dngShorts(tag uint16, values ...uint16) dngEntry {
	var data bytes.Buffer
	for _, value := range values {
		writeDNGValue(&data, value)
	}
	return dngEntry{tag: tag, dataType: 3, count: uint32(len(values)), data: data.Bytes()}
}

func dngLong(tag uint16, value uint32) dngEntry {
	return dngLongs(tag, value)
}

func dngLongs(tag uint16, values ...uint32) dngEntry {
	var data bytes.Buffer
	for _, value := range values {
		writeDNGValue(&data, value)
	}
	return dngEntry{tag: tag, dataType: 4, count: uint32(len(values)), data: data.Bytes()}
}

func dngRationals(tag uint16, values ...[2]int32) dngEntry {
	return dngRationalEntry(tag, 5, values...)
}

func dngSRationals(tag uint16, values ...[2]int32) dngEntry {
	return dngRationalEntry(tag, 10, values...)
}

func dngRationalEntry(tag uint16, dataType uint16, values ...[2]int32) dngEntry {
	var data bytes.Buffer
	for _, value := range values {
		writeDNGValue(&data, value[0])
		writeDNGValue(&data, value[1])
	}
	return dngEntry{tag: tag, dataType: dataType, count: uint32(len(values)), data: data.Bytes()}
}

func writeDNGValue[T ~uint16 | ~uint32 | ~int32](destination *bytes.Buffer, value T) {
	if err := binary.Write(destination, binary.LittleEndian, value); err != nil {
		panic(fmt.Sprintf("writing DNG fixture: %v", err))
	}
}
