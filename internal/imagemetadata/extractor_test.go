package imagemetadata

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/settings"
)

func TestExtractPathReturnsDimensionsAndSafeDescriptiveCameraFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.jpg")
	base := bytes.NewBuffer(nil)
	value := image.NewRGBA(image.Rect(0, 0, 3, 2))
	value.Set(1, 1, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	if err := jpeg.Encode(base, value, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	encoded := insertEXIF(t, base.Bytes(), []testEXIFTag{
		asciiTag(0x010f, "Canon"), asciiTag(0x0110, "EOS Test"), asciiTag(0x0131, "CGM Test"),
		asciiTag(0x013b, "Alice"), asciiTag(0x8298, "Alice 2026"), undefinedTag(0xa300, 3), rationalTag(0x829a, 1, 125), rationalTag(0x829d, 28, 10),
	})
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ExtractPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Width != 3 || result.Height != 2 {
		t.Fatalf("dimensions = %dx%d", result.Width, result.Height)
	}
	byVisibility := make(map[string][]Entry)
	for _, entry := range result.Entries {
		byVisibility[entry.VisibilityKey] = append(byVisibility[entry.VisibilityKey], entry)
		if entry.Key == "exif.MakerNote" {
			t.Fatal("maker note must never be returned")
		}
	}
	if byVisibility[settings.MetadataAuthor][0].Value != "Alice" || byVisibility[settings.MetadataSoftware][0].Value != "CGM Test" {
		t.Fatalf("descriptive fields = %#v", byVisibility)
	}
	if byVisibility[settings.MetadataSource][0].Value != "Digital still camera (3)" {
		t.Fatalf("source field = %#v", byVisibility[settings.MetadataSource])
	}
	if byVisibility[settings.MetadataExposureTime][0].Value != "1/125 s" || byVisibility[settings.MetadataAperture][0].Value != "f/2.8" {
		t.Fatalf("camera fields = %#v", byVisibility)
	}
}

func TestDefaultVisibilityExcludesSensitiveMetadata(t *testing.T) {
	visible := settings.MediaMetadataVisibleSet(settings.DefaultMediaMetadataVisibleFields)
	if visible[settings.MetadataGPS] || visible[settings.MetadataDeviceIdentifiers] {
		t.Fatal("sensitive metadata must require explicit owner opt-in")
	}
	if !visible[settings.MetadataAuthor] || !visible[settings.MetadataOtherEXIF] {
		t.Fatal("ordinary descriptive and safe EXIF metadata should be enabled by default")
	}
}

type testEXIFTag struct {
	id     uint16
	typeID uint16
	count  uint32
	data   []byte
}

func asciiTag(id uint16, value string) testEXIFTag {
	data := append([]byte(value), 0)
	return testEXIFTag{id: id, typeID: 2, count: uint32(len(data)), data: data}
}

func rationalTag(id uint16, numerator, denominator uint32) testEXIFTag {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data, numerator)
	binary.LittleEndian.PutUint32(data[4:], denominator)
	return testEXIFTag{id: id, typeID: 5, count: 1, data: data}
}

func undefinedTag(id uint16, values ...byte) testEXIFTag {
	return testEXIFTag{id: id, typeID: 7, count: uint32(len(values)), data: values}
}

func insertEXIF(t *testing.T, jpegBytes []byte, tags []testEXIFTag) []byte {
	t.Helper()
	if len(jpegBytes) < 2 || jpegBytes[0] != 0xff || jpegBytes[1] != 0xd8 {
		t.Fatal("test JPEG is missing SOI")
	}
	tiffHeader := []byte{'I', 'I', 0x2a, 0, 8, 0, 0, 0}
	ifd := make([]byte, 2+12*len(tags)+4)
	binary.LittleEndian.PutUint16(ifd, uint16(len(tags)))
	var extra []byte
	for index, tag := range tags {
		offset := 2 + index*12
		binary.LittleEndian.PutUint16(ifd[offset:], tag.id)
		binary.LittleEndian.PutUint16(ifd[offset+2:], tag.typeID)
		binary.LittleEndian.PutUint32(ifd[offset+4:], tag.count)
		if len(tag.data) <= 4 {
			copy(ifd[offset+8:offset+12], tag.data)
		} else {
			binary.LittleEndian.PutUint32(ifd[offset+8:], uint32(len(tiffHeader)+len(ifd)+len(extra)))
			extra = append(extra, tag.data...)
		}
	}
	payload := append([]byte("Exif\x00\x00"), tiffHeader...)
	payload = append(payload, ifd...)
	payload = append(payload, extra...)
	if len(payload)+2 > 65535 {
		t.Fatal("test EXIF segment exceeds JPEG APP1 limit")
	}
	segment := []byte{0xff, 0xe1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	segment = append(segment, payload...)
	result := append([]byte{}, jpegBytes[:2]...)
	result = append(result, segment...)
	return append(result, jpegBytes[2:]...)
}
