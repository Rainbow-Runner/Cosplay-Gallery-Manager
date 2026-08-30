package archivecheck

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFileAcceptsOrdinaryCBZ(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "gallery.cbz")
	writeArchive(t, filename, []archiveEntry{{name: "01.jpg", body: "image"}, {name: "sub/02.png", body: "image"}})
	result, err := ValidateFile(filename, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Safe() || result.EntryCount != 2 {
		t.Fatalf("ordinary archive result = %#v", result)
	}
}

func TestValidateFileAcceptsTARAndSevenZIP(t *testing.T) {
	tarPath := filepath.Join(t.TempDir(), "gallery.tar")
	file, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	if err := writer.WriteHeader(&tar.Header{Name: "photos/one.jpg", Mode: 0o600, Size: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("image")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{tarPath, decodeSevenZIP(t, "gallery.7z.b64")} {
		result, err := ValidateFile(filename, DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if !result.Safe() || result.EntryCount != 1 {
			t.Fatalf("archive %q result = %#v", filename, result)
		}
	}
}

func TestValidateFileReportsEncryptedSevenZIP(t *testing.T) {
	result, err := ValidateFile(decodeSevenZIP(t, "encrypted.7z.b64"), DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if result.Safe() || !hasIssue(result, "ENCRYPTED_ARCHIVE") {
		t.Fatalf("encrypted 7z result = %#v", result)
	}
}

func TestValidateFileRejectsStructuralArchiveEntries(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "unsafe.zip")
	writeArchive(t, filename, []archiveEntry{
		{name: "../escape.jpg", body: "bad"},
		{name: "nested.cbz", body: "bad"},
		{name: "Photo.jpg", body: "one"},
		{name: "photo.jpg", body: "two"},
		{name: "link", body: "target", mode: os.ModeSymlink | 0o777},
	})
	result, err := ValidateFile(filename, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	wantCodes := map[string]bool{
		"UNSAFE_ENTRY_PATH": false, "NESTED_ARCHIVE": false,
		"AMBIGUOUS_ENTRY_PATH": false, "SYMLINK_ENTRY": false,
	}
	for _, issue := range result.Issues {
		if _, wanted := wantCodes[issue.Code]; wanted {
			wantCodes[issue.Code] = true
		}
	}
	for code, found := range wantCodes {
		if !found {
			t.Fatalf("missing %s in issues %#v", code, result.Issues)
		}
	}
}

func TestValidateFileEnforcesLimitsAndArchiveMediaPolicy(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "limited.zip")
	writeArchive(t, filename, []archiveEntry{{name: "clip.mp4", body: "0123456789"}})
	limits := DefaultLimits()
	limits.MaxEntryUncompressed = 5
	result, err := ValidateFile(filename, limits)
	if err != nil {
		t.Fatal(err)
	}
	wantCodes := map[string]bool{"ENTRY_SIZE_LIMIT": false, "UNSUPPORTED_ARCHIVE_MEDIA": false}
	for _, issue := range result.Issues {
		if _, wanted := wantCodes[issue.Code]; wanted {
			wantCodes[issue.Code] = true
		}
	}
	for code, found := range wantCodes {
		if !found {
			t.Fatalf("missing %s in issues %#v", code, result.Issues)
		}
	}
}

func TestValidateFileEnforcesImagePixelLimit(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "pixels.cbz")
	picture := image.NewRGBA(image.Rect(0, 0, 11, 10))
	picture.Set(0, 0, color.White)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	writeArchive(t, filename, []archiveEntry{{name: "large.png", body: encoded.String()}})
	limits := DefaultLimits()
	limits.MaxImagePixels = 100
	result, err := ValidateFile(filename, limits)
	if err != nil {
		t.Fatal(err)
	}
	if !containsIssue(result.Issues, "IMAGE_PIXEL_LIMIT") {
		t.Fatalf("pixel limit issue missing: %#v", result.Issues)
	}
}

func TestValidateFileRejectsDangerousArchiveMatrix(t *testing.T) {
	cases := []struct {
		name       string
		entries    []archiveEntry
		limits     func(Limits) Limits
		wantCode   string
		structural bool
	}{
		{name: "absolute path", entries: []archiveEntry{{name: "/escape.jpg", body: "x"}}, wantCode: "UNSAFE_ENTRY_PATH", structural: true},
		{name: "Windows path", entries: []archiveEntry{{name: `C:\escape.jpg`, body: "x"}}, wantCode: "UNSAFE_ENTRY_PATH", structural: true},
		{name: "non NFC path", entries: []archiveEntry{{name: "e\u0301.jpg", body: "x"}}, wantCode: "UNSAFE_ENTRY_PATH", structural: true},
		{name: "special device", entries: []archiveEntry{{name: "device.jpg", body: "x", mode: os.ModeDevice | 0o600}}, wantCode: "SPECIAL_ENTRY", structural: true},
		{name: "encrypted entry", entries: []archiveEntry{{name: "secret.jpg", body: "x", flags: 0x1}}, wantCode: "ENCRYPTED_ENTRY", structural: true},
		{name: "entry count", entries: []archiveEntry{{name: "1.jpg", body: "x"}, {name: "2.jpg", body: "x"}},
			limits: func(value Limits) Limits { value.MaxEntries = 1; return value }, wantCode: "ENTRY_COUNT_LIMIT"},
		{name: "total size", entries: []archiveEntry{{name: "1.jpg", body: "12345"}, {name: "2.jpg", body: "67890"}},
			limits: func(value Limits) Limits { value.MaxTotalUncompressed = 9; return value }, wantCode: "TOTAL_SIZE_LIMIT"},
		{name: "compression ratio", entries: []archiveEntry{{name: "bomb.jpg", body: strings.Repeat("0", 128*1024)}},
			limits: func(value Limits) Limits { value.MaxCompressionRatio = 2; return value }, wantCode: "COMPRESSION_RATIO_LIMIT"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "dangerous.zip")
			writeArchive(t, filename, test.entries)
			limits := DefaultLimits()
			if test.limits != nil {
				limits = test.limits(limits)
			}
			result, err := ValidateFile(filename, limits)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, issue := range result.Issues {
				if issue.Code == test.wantCode {
					found = true
					if issue.Structural != test.structural {
						t.Fatalf("%s structural = %v", issue.Code, issue.Structural)
					}
				}
			}
			if !found {
				t.Fatalf("missing %s in %#v", test.wantCode, result.Issues)
			}
		})
	}
}

type archiveEntry struct {
	name  string
	body  string
	mode  os.FileMode
	flags uint16
}

func writeArchive(t *testing.T, filename string, entries []archiveEntry) {
	t.Helper()
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate, Flags: entry.flags}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		part, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func decodeSevenZIP(t *testing.T, name string) string {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("..", "archivefile", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	body, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), strings.TrimSuffix(name, ".b64"))
	if err := os.WriteFile(filename, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func hasIssue(result Result, code string) bool {
	for _, issue := range result.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func containsIssue(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
