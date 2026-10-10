package productdb

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
)

func TestManifestBatchPreviewAndOverwriteAreFailClosed(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 18, 14, 0, 0, 0, time.UTC)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Original", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: t.TempDir(), Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := db.Galleries().Find(ctx, created.ID)
	state, err := db.Manifests().PushGallery(ctx, created.ID, current.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := db.Manifests().CheckGallery(ctx, created.ID, now)
	if err != nil || checked.Status != ManifestClean {
		t.Fatalf("initial check = %#v, %v", checked, err)
	}
	var observed string
	if err := db.QueryRowContext(ctx, `SELECT status FROM gallery_manifest_inspections WHERE gallery_id=?`, created.ID).Scan(&observed); err != nil || observed != "CLEAN" {
		t.Fatalf("persisted inspection = %q, %v", observed, err)
	}
	current, err = db.Galleries().SetState(ctx, created.ID, current.MetadataRevision, gallery.StateDraft, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := db.Manifests().PreviewGalleryBatchPush(ctx, created.ID, now.Add(time.Minute))
	if err != nil || preview.Status != ManifestDBDirty || preview.DatabaseContentChanged {
		t.Fatalf("revision-only preview = %#v, %v", preview, err)
	}
	result, err := db.Manifests().PushGalleryBatchItem(ctx, GalleryManifestBatchDecision{GalleryID: created.ID, ExpectedRevision: preview.MetadataRevision, ExpectedPath: preview.Path, ExpectedFileHash: preview.FileHash}, false, now.Add(time.Minute))
	if err != nil || result.Reason != "NO_CONTENT_CHANGE" {
		t.Fatalf("revision-only Push = %#v, %v", result, err)
	}
	clean, err := db.Manifests().CheckGallery(ctx, created.ID, now.Add(time.Minute))
	if err != nil || clean.Status != ManifestClean {
		t.Fatalf("revision-only acknowledgement = %#v, %v", clean, err)
	}
	if clean.FileHash != state.FileHash || clean.ManifestRevision != state.ManifestRevision {
		t.Fatalf("revision-only acknowledgement rewrote Manifest: before=%#v after=%#v", state, clean)
	}
	current, err = db.Galleries().UpdateMetadata(ctx, created.ID, current.MetadataRevision, UpdateGalleryMetadataInput{Title: "Database", ContentRating: gallery.ContentRatingNonAdult}, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	preview, err = db.Manifests().PreviewGalleryBatchPush(ctx, created.ID, now.Add(2*time.Minute))
	if err != nil || !preview.DatabaseContentChanged || preview.LocalFileChanged {
		t.Fatalf("database-only preview = %#v, %v", preview, err)
	}
	data, _, err := manifest.ReadFile(state.Path, manifest.MaxGalleryBytes)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	document["title"] = "Local"
	changed, _ := json.MarshalIndent(document, "", "  ")
	if err := os.WriteFile(state.Path, append(changed, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	stale, err := db.Manifests().PushGalleryBatchItem(ctx, GalleryManifestBatchDecision{GalleryID: created.ID, ExpectedRevision: preview.MetadataRevision, ExpectedPath: preview.Path, ExpectedFileHash: preview.FileHash}, true, now.Add(3*time.Minute))
	if err != nil || stale.Reason != "PREVIEW_STALE" {
		t.Fatalf("changed-after-preview = %#v, %v", stale, err)
	}
	preview, err = db.Manifests().PreviewGalleryBatchPush(ctx, created.ID, now.Add(3*time.Minute))
	if err != nil || !preview.LocalFileChanged || preview.Status != ManifestConflict {
		t.Fatalf("two-sided preview = %#v, %v", preview, err)
	}
	decision := GalleryManifestBatchDecision{GalleryID: created.ID, ExpectedRevision: preview.MetadataRevision, ExpectedPath: preview.Path, ExpectedFileHash: preview.FileHash}
	skipped, err := db.Manifests().PushGalleryBatchItem(ctx, decision, false, now.Add(3*time.Minute))
	if err != nil || skipped.Reason != "LOCAL_FILE_CHANGED" {
		t.Fatalf("skip local changes = %#v, %v", skipped, err)
	}
	pushed, err := db.Manifests().PushGalleryBatchItem(ctx, decision, true, now.Add(3*time.Minute))
	if err != nil || pushed.Outcome != "PUSHED" {
		t.Fatalf("explicit overwrite = %#v, %v", pushed, err)
	}
	updated, _, err := manifest.ReadFile(filepath.Join(filepath.Dir(state.Path), ".cosplay.json"), manifest.MaxGalleryBytes)
	if err != nil {
		t.Fatal(err)
	}
	var final map[string]any
	if err := json.Unmarshal(updated, &final); err != nil || final["title"] != "Database" {
		t.Fatalf("overwritten file = %v, %v", final["title"], err)
	}
}
