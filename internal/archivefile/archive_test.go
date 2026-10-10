package archivefile

import (
	"archive/tar"
	"compress/gzip"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWalkSupportedArchiveFormats(t *testing.T) {
	gif := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x01L\x00;")
	for _, test := range []struct {
		name      string
		path      string
		entryName string
	}{
		{name: "tar", path: writeTARFixture(t, gif, false), entryName: "photos/one.gif"},
		{name: "tar gzip", path: writeTARFixture(t, gif, true), entryName: "photos/one.gif"},
		{name: "7z", path: decodeSevenZIPFixture(t, "gallery.7z.b64"), entryName: "one.gif"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var names []string
			var contents [][]byte
			if err := Walk(test.path, func(entry Entry) error {
				if entry.IsDir() {
					return nil
				}
				part, err := entry.Open()
				if err != nil {
					return err
				}
				body, err := io.ReadAll(part)
				closeErr := part.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
				names = append(names, entry.Name)
				contents = append(contents, body)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(names) != 1 || names[0] != test.entryName || string(contents[0]) != string(gif) {
				t.Fatalf("entries=%q contents=%d", names, len(contents))
			}
		})
	}
}

func TestEncryptedSevenZIPIsIdentifiable(t *testing.T) {
	err := Walk(decodeSevenZIPFixture(t, "encrypted.7z.b64"), func(entry Entry) error {
		part, err := entry.Open()
		if err != nil {
			return err
		}
		defer part.Close()
		_, err = io.ReadAll(part)
		return err
	})
	if err == nil || !IsEncryptedError(err) {
		t.Fatalf("encrypted 7z error = %v", err)
	}
}

func TestDetectAndBaseName(t *testing.T) {
	for _, value := range []string{"set.zip", "set.cbz", "set.tar", "set.tar.gz", "set.tgz", "set.7z"} {
		if !IsSupportedPath(value) || BaseName(value) != "set" {
			t.Fatalf("archive identity failed for %q", value)
		}
	}
	if IsSupportedPath("set.rar") || !IsArchiveLikePath("set.rar") {
		t.Fatal("RAR must remain reportable but unsupported")
	}
}

func writeTARFixture(t *testing.T, body []byte, compressed bool) string {
	t.Helper()
	extension := ".tar"
	if compressed {
		extension = ".tar.gz"
	}
	filename := filepath.Join(t.TempDir(), "gallery"+extension)
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	var output io.Writer = file
	var gzipWriter *gzip.Writer
	if compressed {
		gzipWriter = gzip.NewWriter(file)
		output = gzipWriter
	}
	writer := tar.NewWriter(output)
	if err := writer.WriteHeader(&tar.Header{Name: "photos/one.gif", Mode: 0o600, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if gzipWriter != nil {
		if err := gzipWriter.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

func decodeSevenZIPFixture(t *testing.T, name string) string {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	body, err := base64.StdEncoding.DecodeString(string(encoded[:len(encoded)-1]))
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), name[:len(name)-4])
	if err := os.WriteFile(filename, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}
