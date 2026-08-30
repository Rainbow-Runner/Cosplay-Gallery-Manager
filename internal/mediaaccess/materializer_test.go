package mediaaccess

import (
	"archive/tar"
	"archive/zip"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/gallery"
)

func TestMaterializerReadsDirectoryWithoutFollowingSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "photos"), 0o700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "photos", "one.jpg")
	if err := os.WriteFile(filename, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	materialized, err := (Materializer{}).Open(context.Background(), Source{Type: gallery.SourceTypeDirectory, Path: root, RelativePath: "photos/one.jpg"})
	if err != nil || materialized.Path != filename {
		t.Fatalf("directory materialization = %#v, %v", materialized, err)
	}
	if err := materialized.Close(); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.jpg")
	_ = os.WriteFile(outside, []byte("outside"), 0o600)
	if err := os.Symlink(outside, filepath.Join(root, "linked.jpg")); err == nil {
		if _, err := (Materializer{}).Open(context.Background(), Source{Type: gallery.SourceTypeDirectory, Path: root, RelativePath: "linked.jpg"}); err == nil {
			t.Fatal("directory symlink was followed")
		}
	}
}

func TestMaterializerExtractsOneArchiveMemberToManagedTemporaryFile(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "set.cbz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("photos/one.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("archive image"))
	_ = writer.Close()
	_ = file.Close()
	temporaryRoot := t.TempDir()
	materialized, err := (Materializer{TemporaryRoot: temporaryRoot, MaximumBytes: 100}).Open(context.Background(), Source{Type: gallery.SourceTypeArchive, Path: archive, RelativePath: "photos/one.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(materialized.Path)
	if err != nil || string(data) != "archive image" {
		t.Fatalf("archive materialization = %q, %v", data, err)
	}
	name := materialized.Path
	if err := materialized.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary extraction was not removed: %v", err)
	}
}

func TestMaterializerExtractsTARAndSevenZIPMembers(t *testing.T) {
	body := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x01L\x00;")
	tarPath := filepath.Join(t.TempDir(), "set.tar")
	file, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	if err := writer.WriteHeader(&tar.Header{Name: "photos/one.gif", Mode: 0o600, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ filename, relative string }{
		{filename: tarPath, relative: "photos/one.gif"},
		{filename: decodeMaterializerSevenZIP(t), relative: "one.gif"},
	} {
		materialized, err := (Materializer{TemporaryRoot: t.TempDir(), MaximumBytes: 100}).Open(context.Background(), Source{
			Type: gallery.SourceTypeArchive, Path: test.filename, RelativePath: test.relative,
		})
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(materialized.Path)
		if err != nil || string(data) != string(body) {
			t.Fatalf("materialized %q = %d bytes, %v", test.filename, len(data), err)
		}
		if err := materialized.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func decodeMaterializerSevenZIP(t *testing.T) string {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("..", "archivefile", "testdata", "gallery.7z.b64"))
	if err != nil {
		t.Fatal(err)
	}
	body, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "gallery.7z")
	if err := os.WriteFile(filename, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}
