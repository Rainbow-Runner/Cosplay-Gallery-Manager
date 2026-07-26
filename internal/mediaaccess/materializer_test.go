package mediaaccess

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
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
