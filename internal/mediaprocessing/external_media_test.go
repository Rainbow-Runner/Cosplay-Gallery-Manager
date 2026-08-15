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
	ffprobePath := requireExternalFFprobe(t, ffmpegPath)
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
	videoMetadata, err := (ProbeAdapter{Executable: ffprobePath, Version: "external-gate"}).Probe(context.Background(), filepath.Join(sourceRoot, "clip.mp4"))
	if err != nil || videoMetadata.DurationSeconds <= 0 || videoMetadata.VideoStreamIndex < 0 {
		t.Fatalf("real FFprobe result = %#v, %v", videoMetadata, err)
	}
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

	t.Run("FFprobe technical matrix", func(t *testing.T) {
		movPath := filepath.Join(outputRoot, "silent.mov")
		runExternal(t, ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "1", "-an", "-c:v", "libx264", movPath)
		mov, err := (ProbeAdapter{Executable: ffprobePath, Version: "external-gate"}).Probe(context.Background(), movPath)
		if err != nil || mov.Container != "mp4" || mov.VideoCodec != "h264" || mov.AudioStreamIndex != nil {
			t.Fatalf("MOV/silent probe = %#v, %v", mov, err)
		}

		webmPath := filepath.Join(outputRoot, "vp9.webm")
		runExternal(t, ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "1", "-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-c:a", "libopus", webmPath)
		webm, err := (ProbeAdapter{Executable: ffprobePath, Version: "external-gate"}).Probe(context.Background(), webmPath)
		if err != nil || webm.Container != "webm" || webm.VideoCodec != "vp9" || webm.AudioCodec != "opus" || PlaybackPlanFromMetadata(webm).Mode != PlaybackTranscode {
			t.Fatalf("WebM probe = %#v, %v", webm, err)
		}

		dualPath := filepath.Join(outputRoot, "dual-audio.mkv")
		runExternal(t, ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000", "-t", "1", "-map", "0:v:0", "-map", "1:a:0", "-map", "2:a:0", "-c:v", "libx264", "-c:a", "aac", "-disposition:a:0", "0", "-disposition:a:1", "default", dualPath)
		dual, err := (ProbeAdapter{Executable: ffprobePath, Version: "external-gate"}).Probe(context.Background(), dualPath)
		if err != nil || dual.AudioStreamIndex == nil || *dual.AudioStreamIndex != 2 {
			t.Fatalf("dual-audio default selection = %#v, %v", dual, err)
		}

		rotationBase := filepath.Join(outputRoot, "rotation-base.mp4")
		rotationPath := filepath.Join(outputRoot, "rotated.mp4")
		runExternal(t, ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "1", "-an", "-c:v", "libx264", rotationBase)
		runExternal(t, ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-display_rotation", "90", "-i", rotationBase, "-c", "copy", rotationPath)
		rotated, err := (ProbeAdapter{Executable: ffprobePath, Version: "external-gate"}).Probe(context.Background(), rotationPath)
		if err != nil || rotated.Rotation == 0 || rotated.DisplayWidth != 180 || rotated.DisplayHeight != 320 || PlaybackPlanFromMetadata(rotated).Mode != PlaybackTranscode {
			t.Fatalf("rotation probe = %#v, %v", rotated, err)
		}

		hdrPath := filepath.Join(outputRoot, "hdr-tagged.mkv")
		runExternal(t, ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "1", "-an", "-vf", "format=yuv420p10le", "-c:v", "ffv1", "-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc", hdrPath)
		hdr, err := (ProbeAdapter{Executable: ffprobePath, Version: "external-gate"}).Probe(context.Background(), hdrPath)
		if err != nil || !hdr.HDR || PlaybackPlanFromMetadata(hdr).Mode != PlaybackTranscode {
			t.Fatalf("HDR probe = %#v, %v", hdr, err)
		}
		hdrPlan := PlaybackPlanFromMetadata(hdr)
		hdrOutput := filepath.Join(outputRoot, "hdr-sdr-proxy.mp4")
		if _, err := (VideoPlaybackGenerator{Encoder: stashffmpeg.NewEncoder(ffmpegPath)}).Generate(context.Background(), GenerateRequest{MediaKind: gallery.MediaKindVideo,
			ContentFormat: gallery.ContentFormatVideo, Variant: VariantVideoPlayback, SourcePath: hdrPath, DestinationPath: hdrOutput, VideoTechnical: &hdr, VideoPlan: &hdrPlan}); err != nil {
			t.Fatal(err)
		}
	})

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
				Variant: VariantStaticPoster, SourcePath: filepath.Join(sourceRoot, "clip.mp4"), VideoTechnical: &videoMetadata,
			},
			wantMaximum: 960,
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

	remuxSource := filepath.Join(sourceRoot, "remux-source.mkv")
	runExternal(t, ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "1", "-an", "-c:v", "libx264", remuxSource)
	remuxMetadata, err := (ProbeAdapter{Executable: ffprobePath, Version: "external-gate"}).Probe(context.Background(), remuxSource)
	if err != nil {
		t.Fatal(err)
	}
	remuxPlan := PlaybackPlanFromMetadata(remuxMetadata)
	if remuxPlan.Mode != PlaybackRemux {
		t.Fatalf("real remux plan = %#v from %#v", remuxPlan, remuxMetadata)
	}
	remuxOutput := filepath.Join(outputRoot, "video-remux.mp4")
	remuxRequest := GenerateRequest{MediaKind: gallery.MediaKindVideo, ContentFormat: gallery.ContentFormatVideo, Variant: VariantVideoPlayback,
		SourcePath: remuxSource, DestinationPath: remuxOutput, VideoTechnical: &remuxMetadata, VideoPlan: &remuxPlan}
	if _, err := (VideoPlaybackGenerator{Encoder: stashffmpeg.NewEncoder(ffmpegPath)}).Generate(context.Background(), remuxRequest); err != nil {
		t.Fatal(err)
	}
	remuxResult, err := (ProbeAdapter{Executable: ffprobePath, Version: "external-gate"}).Probe(context.Background(), remuxOutput)
	if err != nil || remuxResult.Container != "mp4" || remuxResult.VideoCodec != "h264" {
		t.Fatalf("real remux output = %#v, %v", remuxResult, err)
	}

	transcodePlan := PlaybackPlanFromMetadata(videoMetadata)
	if transcodePlan.Mode != PlaybackTranscode {
		t.Fatalf("real transcode plan = %#v", transcodePlan)
	}
	transcodeOutput := filepath.Join(outputRoot, "video-transcode.mp4")
	transcodeRequest := GenerateRequest{MediaKind: gallery.MediaKindVideo, ContentFormat: gallery.ContentFormatVideo, Variant: VariantVideoPlayback,
		SourcePath: filepath.Join(sourceRoot, "clip.mp4"), DestinationPath: transcodeOutput, VideoTechnical: &videoMetadata, VideoPlan: &transcodePlan}
	if _, err := (VideoPlaybackGenerator{Encoder: stashffmpeg.NewEncoder(ffmpegPath)}).Generate(context.Background(), transcodeRequest); err != nil {
		t.Fatal(err)
	}
	transcodeResult, err := (ProbeAdapter{Executable: ffprobePath, Version: "external-gate"}).Probe(context.Background(), transcodeOutput)
	if err != nil || transcodeResult.Container != "mp4" || transcodeResult.VideoCodec != "h264" {
		t.Fatalf("real transcode output = %#v, %v", transcodeResult, err)
	}
	if directPlan := PlaybackPlanFromMetadata(transcodeResult); directPlan.Mode != PlaybackDirect {
		t.Fatalf("generated compatible MP4 was not direct-playable: %#v", directPlan)
	}
}

func requireExternalFFprobe(t *testing.T, ffmpegPath string) string {
	t.Helper()
	if configured := os.Getenv("CGM_TEST_FFPROBE"); configured != "" {
		path, err := exec.LookPath(configured)
		if err != nil {
			t.Fatalf("CGM_TEST_FFPROBE: %v", err)
		}
		return path
	}
	candidate := filepath.Join(filepath.Dir(ffmpegPath), "ffprobe")
	if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
		return candidate
	}
	t.Skip("CGM_TEST_FFPROBE or an ffprobe sibling is required for the external video release gate")
	return ""
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
