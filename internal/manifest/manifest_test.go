package manifest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/gallery"
)

const manifestSetID = "12345678-1234-4123-8123-123456789abc"
const manifestCoserID = "22345678-1234-4123-8123-123456789abc"
const manifestWorkID = "32345678-1234-4123-8123-123456789abc"
const manifestCharacterID = "42345678-1234-4123-8123-123456789abc"

func TestGalleryManifestStrictPartialAndExplicitNull(t *testing.T) {
	raw := `{
		"schema_version":1,"revision":0,"set_id":"` + manifestSetID + `",
		"updated_at":"2026-07-22T12:00:00Z","title":null,
		"extensions":{"future":{"enabled":true}}
	}`
	document, err := ParseGallery(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !document.Title.Present || !document.Title.Null || document.Description.Present {
		t.Fatalf("optional field semantics = %#v %#v", document.Title, document.Description)
	}
	unknown := strings.Replace(raw, `"title":null,`, `"unknown":true,`, 1)
	if _, err := ParseGallery(strings.NewReader(unknown)); err == nil {
		t.Fatal("ordinary unknown Gallery Manifest field was accepted")
	}
}

func TestGalleryManifestValidatesRelationsPathsAndRatings(t *testing.T) {
	raw := `{
		"schema_version":1,"revision":2,"set_id":"` + manifestSetID + `",
		"updated_at":"2026-07-22T12:00:00Z","rating":4.5,
		"credits":[{"coser":{"uuid":"` + manifestCoserID + `"},"position":1024}],
		"cast":[{"coser":{"uuid":"` + manifestCoserID + `"},
			"work":{"uuid":"` + manifestWorkID + `"},
			"character":{"uuid":"` + manifestCharacterID + `"},"position":1024}],
		"items":[{"path":"photos/01.jpg","rating":3.5}]
	}`
	if _, err := ParseGallery(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseGallery(strings.NewReader(strings.Replace(raw, "photos/01.jpg", "../escape.jpg", 1))); err == nil {
		t.Fatal("Manifest traversal path was accepted")
	}
	if _, err := ParseGallery(strings.NewReader(strings.Replace(raw, "4.5", "4.2", 1))); err == nil {
		t.Fatal("non-half-star rating was accepted")
	}
}

func TestCoserManifestCropAndAssetPaths(t *testing.T) {
	raw := `{
		"schema_version":1,"revision":1,"coser_uuid":"` + manifestCoserID + `",
		"updated_at":"2026-07-22T12:00:00Z","name":"Coser",
		"avatar":"assets/avatar.jpg","avatar_crop":{"x":0.1,"y":0.2,"size":0.5}
	}`
	if _, err := ParseCoser(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	invalid := strings.Replace(raw, `"size":0.5`, `"size":0.9`, 1)
	if _, err := ParseCoser(strings.NewReader(invalid)); err == nil {
		t.Fatal("out-of-bounds avatar crop was accepted")
	}
}

func TestThreeWayMergeFieldsCollectionsAndConflicts(t *testing.T) {
	baseline := map[string]any{
		"title": "old", "description": "old description",
		"links": map[string]any{"link-a": map[string]any{"label": "A", "url": "https://a.test"}},
	}
	database := map[string]any{
		"title": "database", "description": "old description",
		"links": map[string]any{
			"link-a": map[string]any{"label": "A", "url": "https://a.test"},
			"link-b": map[string]any{"label": "B", "url": "https://b.test"},
		},
	}
	file := map[string]any{
		"title": "old", "description": "file description",
		"links": map[string]any{
			"link-a": map[string]any{"label": "Changed A", "url": "https://a.test"},
		},
	}
	mergedValue, conflicts := ThreeWayMerge(baseline, database, file)
	if len(conflicts) != 0 {
		t.Fatalf("independent changes conflicted: %#v", conflicts)
	}
	merged := mergedValue.(map[string]any)
	if merged["title"] != "database" || merged["description"] != "file description" {
		t.Fatalf("merged scalar fields = %#v", merged)
	}
	links := merged["links"].(map[string]any)
	if len(links) != 2 || links["link-a"].(map[string]any)["label"] != "Changed A" {
		t.Fatalf("merged keyed collection = %#v", links)
	}

	database["title"] = "database two"
	file["title"] = "file two"
	_, conflicts = ThreeWayMerge(baseline, database, file)
	if len(conflicts) != 1 || conflicts[0].Path != "/title" {
		t.Fatalf("same-field conflict = %#v", conflicts)
	}

	deleted := map[string]any{"links": map[string]any{}}
	modified := map[string]any{"links": map[string]any{"link-a": map[string]any{"label": "modified"}}}
	_, conflicts = ThreeWayMerge(map[string]any{"links": baseline["links"]}, deleted, modified)
	if len(conflicts) == 0 {
		t.Fatal("delete-modify collection conflict was not reported")
	}
	resolvedValue, _, err := ResolveThreeWay(
		map[string]any{"links": baseline["links"]}, deleted, modified,
		map[string]ConflictChoice{"/links/link-a": ChooseDatabase},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolvedLinks := resolvedValue.(map[string]any)["links"].(map[string]any)
	if _, exists := resolvedLinks["link-a"]; exists {
		t.Fatalf("database deletion choice retained member: %#v", resolvedLinks)
	}
	if _, _, err := ResolveThreeWay(baseline, database, file, nil); err == nil {
		t.Fatal("partial conflict choices were accepted")
	}
}

func TestManifestPathAndAtomicSingleBackup(t *testing.T) {
	root := t.TempDir()
	directoryPath, err := GalleryPath(gallery.SourceTypeDirectory, root)
	if err != nil || directoryPath != filepath.Join(root, ".cosplay.json") {
		t.Fatalf("directory Manifest path = %q, %v", directoryPath, err)
	}
	archive := filepath.Join(root, "set.cbz")
	archivePath, _ := GalleryPath(gallery.SourceTypeArchive, archive)
	if archivePath != archive+".cosplay.json" {
		t.Fatalf("archive Manifest path = %q", archivePath)
	}
	directoryCover, err := GalleryManagedCoverPath(gallery.SourceTypeDirectory, root, "png")
	if err != nil || directoryCover != filepath.Join(root, ".cosplay-assets", "cover.png") {
		t.Fatalf("directory cover path = %q, %v", directoryCover, err)
	}
	archiveCover, err := GalleryManagedCoverPath(gallery.SourceTypeArchive, archive, ".webp")
	if err != nil || archiveCover != filepath.Join(archive+".cosplay-assets", "cover.webp") {
		t.Fatalf("archive cover path = %q, %v", archiveCover, err)
	}
	first := []byte(`{"revision":1}`)
	second := []byte(`{"revision":2}`)
	third := []byte(`{"revision":3}`)
	if _, err := WriteAtomic(directoryPath, first, MaxGalleryBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteAtomic(directoryPath, second, MaxGalleryBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteAtomic(directoryPath, third, MaxGalleryBytes); err != nil {
		t.Fatal(err)
	}
	current, _, err := ReadFile(directoryPath, MaxGalleryBytes)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(directoryPath + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(third) || string(backup) != string(second) {
		t.Fatalf("atomic snapshots current=%s backup=%s", current, backup)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("Manifest write retained unexpected files: %#v", entries)
	}
	if _, err := json.Marshal(map[string]any{"hash": Hash(current)}); err != nil {
		t.Fatal(err)
	}
}

func TestCoserMergeRedirectUsesIndependentPortableFile(t *testing.T) {
	root := t.TempDir()
	source := "12345678-1234-4123-8123-123456789abc"
	target := "22345678-1234-4123-8123-123456789abc"
	filename, err := WriteCoserRedirect(root, source, target)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filename) != "redirect_to_uuid" {
		t.Fatalf("redirect filename = %q", filename)
	}
	resolved, err := ReadCoserRedirect(root, source)
	if err != nil || resolved != target {
		t.Fatalf("redirect target = %q, %v", resolved, err)
	}
	if _, err := os.Stat(filepath.Join(root, source, "coser.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("redirect unexpectedly created coser.json: %v", err)
	}
}
