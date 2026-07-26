package sourcescan

import (
	"archive/zip"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
)

func TestScanDirectoryHashesAndClassifiesActualContent(t *testing.T) {
	root := t.TempDir()
	imageFile, err := os.Create(filepath.Join(root, "10.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: 200, A: 255})
	if err := jpeg.Encode(imageFile, picture, nil); err != nil {
		t.Fatal(err)
	}
	if err := imageFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "2.gif"), []byte("GIF89a content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fake.jpg"), append([]byte("\x00\x00\x00\x18ftypisom"), make([]byte, 20)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "capture.dng"), append([]byte("II*\x00"), make([]byte, 32)...), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ScanDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 4 {
		t.Fatalf("observations = %#v", result.Observations)
	}
	byPath := make(map[string]Observation)
	for _, observation := range result.Observations {
		byPath[observation.RelativePath] = observation
		if len(observation.FullFingerprint) != len("blake3-v1:")+64 {
			t.Fatalf("full fingerprint = %q", observation.FullFingerprint)
		}
	}
	if byPath["10.jpg"].MediaKind != gallery.MediaKindStaticImage ||
		byPath["2.gif"].MediaKind != gallery.MediaKindAnimatedImage ||
		byPath["fake.jpg"].MediaKind != gallery.MediaKindVideo ||
		byPath["capture.dng"].MediaKind != gallery.MediaKindStaticImage ||
		byPath["capture.dng"].ContentFormat != gallery.ContentFormatRAW {
		t.Fatalf("content classification = %#v", byPath)
	}
	if !hasIssue(result.Issues, "CONTENT_EXTENSION_MISMATCH") {
		t.Fatalf("format mismatch issue missing: %#v", result.Issues)
	}
}

func TestScanDirectoryNeverFollowsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.jpg")
	if err := os.WriteFile(outside, []byte("\xff\xd8\xff content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.jpg")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	result, err := ScanDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 0 || !hasIssue(result.Issues, "SYMLINK_IGNORED") {
		t.Fatalf("symlink scan result = %#v", result)
	}
}

func TestScanArchiveUsesCompressedSizeAndBlocksUnsafeMedia(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "set.cbz")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	part, err := writer.Create("photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("\xff\xd8\xff image image image")); err != nil {
		t.Fatal(err)
	}
	video, err := writer.Create("clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := video.Write(append([]byte("\x00\x00\x00\x18ftypisom"), make([]byte, 20)...)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := ScanArchive(context.Background(), filename, archivecheck.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 2 || !hasIssue(result.Issues, "UNSUPPORTED_ARCHIVE_MEDIA") {
		t.Fatalf("archive scan result = %#v", result)
	}
	for _, observation := range result.Observations {
		if observation.ByteSize <= 0 {
			t.Fatalf("archive member did not use compressed size: %#v", observation)
		}
	}
}

func hasIssue(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
