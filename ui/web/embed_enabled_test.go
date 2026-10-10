//go:build cgm_web_embed

package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedFileSystemContainsOnlyProductBuild(t *testing.T) {
	fileSystem, ok := FileSystem()
	if !ok {
		t.Fatal("product web build is not embedded")
	}
	index, err := fs.ReadFile(fileSystem, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "<title>Cosplay Gallery Manager</title>") {
		t.Fatal("embedded index is not the product UI")
	}
	assetCount := 0
	if err := fs.WalkDir(fileSystem, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(path, "v2.5") {
			t.Fatalf("legacy UI path embedded in product UI: %s", path)
		}
		if !entry.IsDir() && strings.HasPrefix(path, "assets/") {
			assetCount++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if assetCount == 0 {
		t.Fatal("embedded product UI contains no built assets")
	}
}
