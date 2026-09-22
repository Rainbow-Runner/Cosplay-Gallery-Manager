package productdb

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
)

func TestGalleryManifestPushDirtyConflictAndMissingStates(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	root := t.TempDir()
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{
		Title: "Baseline", Description: "## Notes", ContentRating: gallery.ContentRatingNonAdult,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: root, Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "photo.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now); err != nil {
		t.Fatal(err)
	}
	coser := registerTestPortableUUID(t, db, "COSER", now)
	if _, err := db.Galleries().AddCredit(ctx, created.ID, coser, 1024, created.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	current, _ := db.Galleries().Find(ctx, created.ID)
	state, err := db.Manifests().PushGallery(ctx, created.ID, current.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != ManifestClean || state.ManifestRevision != 1 {
		t.Fatalf("first Push state = %#v", state)
	}
	data, _, err := manifest.ReadFile(state.Path, manifest.MaxGalleryBytes)
	if err != nil {
		t.Fatal(err)
	}
	document, err := manifest.ParseGallery(bytesReader(data))
	if err != nil {
		t.Fatalf("system Push did not produce valid strict Manifest: %v\n%s", err, data)
	}
	if document.SetID != created.SetID || len(document.Credits.Value) != 1 || len(document.Items.Value) != 1 {
		t.Fatalf("pushed Manifest = %#v", document)
	}

	current, _ = db.Galleries().Find(ctx, created.ID)
	updated, err := db.Galleries().UpdateMetadata(ctx, created.ID, current.MetadataRevision, UpdateGalleryMetadataInput{
		Title: "Database title", Description: current.Description,
		ContentRating: current.ContentRating,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	checked, err := db.Manifests().CheckGallery(ctx, created.ID, now.Add(time.Minute))
	if err != nil || checked.Status != ManifestDBDirty {
		t.Fatalf("DB dirty Check = %#v, %v", checked, err)
	}

	var external map[string]any
	if err := json.Unmarshal(data, &external); err != nil {
		t.Fatal(err)
	}
	external["title"] = "File title"
	externalData, _ := json.MarshalIndent(external, "", "  ")
	externalData = append(externalData, '\n')
	if err := os.WriteFile(state.Path, externalData, 0o600); err != nil {
		t.Fatal(err)
	}
	checked, err = db.Manifests().CheckGallery(ctx, created.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if checked.Status != ManifestConflict || len(checked.Conflicts) == 0 || checked.Conflicts[0].Path != "/title" {
		t.Fatalf("three-way conflict Check = %#v", checked)
	}
	if _, err := db.Manifests().PushGallery(ctx, created.ID, updated.MetadataRevision, now); !errors.Is(err, ErrManifestFileDirty) {
		t.Fatalf("Push over externally modified file error = %v", err)
	}

	if err := os.Rename(state.Path, state.Path+".externally-moved"); err != nil {
		t.Fatal(err)
	}
	checked, err = db.Manifests().CheckGallery(ctx, created.ID, now.Add(3*time.Minute))
	if err != nil || checked.Status != ManifestMissing {
		t.Fatalf("missing Manifest Check = %#v, %v", checked, err)
	}
}

func TestGalleryManifestPushPreviewCountsForgottenMembers(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 10, 14, 0, 0, 0, time.UTC)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Preview", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: t.TempDir(), Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "missing.jpg", MediaKind: gallery.MediaKindStaticImage, ImageCategory: gallery.ImageCategoryPhoto,
		Position: 1024, Availability: gallery.AvailabilityMissing, ProcessingState: gallery.ProcessingReady,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := db.Manifests().PreviewGalleryPush(ctx, created.ID)
	if err != nil || preview.Added != 1 || preview.Removed != 0 || preview.Retained != 0 {
		t.Fatalf("initial push preview=%#v err=%v", preview, err)
	}
	current, _ := db.Galleries().Find(ctx, created.ID)
	if _, err := db.Manifests().PushGallery(ctx, created.ID, current.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	preview, err = db.Manifests().PreviewGalleryPush(ctx, created.ID)
	if err != nil || preview.Added != 0 || preview.Removed != 0 || preview.Retained != 1 || preview.Updated != 0 {
		t.Fatalf("clean push preview=%#v err=%v", preview, err)
	}
	current, _ = db.Galleries().Find(ctx, created.ID)
	if err := db.Galleries().ForgetItem(ctx, item.ID, current.MetadataRevision, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	preview, err = db.Manifests().PreviewGalleryPush(ctx, created.ID)
	if err != nil || preview.Added != 0 || preview.Removed != 1 || preview.Retained != 0 {
		t.Fatalf("forgotten member push preview=%#v err=%v", preview, err)
	}
}

func TestGalleryManifestNeverOverwritesUntrackedOrDisabledWriteback(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	writebackDisabled := false
	mediaLibrary, err := db.Libraries().Create(ctx, CreateLibraryInput{
		Name: "Writeback disabled", RootPath: t.TempDir(), Enabled: true, MetadataWritebackEnabled: &writebackDisabled,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Read only"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &mediaLibrary.ID, Type: gallery.SourceTypeDirectory,
		Path: mediaLibrary.RootPath, Availability: gallery.AvailabilityAvailable,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Manifests().PushGallery(ctx, created.ID, created.MetadataRevision, now); err == nil {
		t.Fatal("Manifest Push wrote to a media library with metadata writeback disabled")
	}

	root := t.TempDir()
	untracked, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Untracked"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddSource(ctx, untracked.ID, CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: root, Availability: gallery.AvailabilityAvailable,
	}, now); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, ".cosplay.json")
	if err := os.WriteFile(manifestPath, []byte(`{"external":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Manifests().PushGallery(ctx, untracked.ID, untracked.MetadataRevision, now); !errors.Is(err, ErrUntrackedManifest) {
		t.Fatalf("Push over untracked Manifest error = %v", err)
	}
}

func TestGalleryManifestConflictRequiresAndAppliesExplicitChoice(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 0, 30, 0, 0, time.UTC)
	root := t.TempDir()
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Baseline"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: root, Availability: gallery.AvailabilityAvailable}, now); err != nil {
		t.Fatal(err)
	}
	pushed, err := db.Manifests().PushGallery(ctx, created.ID, created.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	baselineData, _, err := manifest.ReadFile(pushed.Path, manifest.MaxGalleryBytes)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := db.Galleries().UpdateMetadata(ctx, created.ID, created.MetadataRevision, UpdateGalleryMetadataInput{Title: "Database"}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	var external map[string]any
	if err := json.Unmarshal(baselineData, &external); err != nil {
		t.Fatal(err)
	}
	external["title"] = "Manifest"
	externalData, _ := json.Marshal(external)
	if err := os.WriteFile(pushed.Path, externalData, 0o600); err != nil {
		t.Fatal(err)
	}
	conflicted, err := db.Manifests().PullGallery(ctx, created.ID, updated.MetadataRevision, now.Add(2*time.Minute))
	if err != nil || conflicted.Status != ManifestConflict {
		t.Fatalf("conflicting Pull = %#v, %v", conflicted, err)
	}
	if _, err := db.Manifests().ResolveGalleryConflicts(ctx, created.ID, updated.MetadataRevision, nil, now); err == nil {
		t.Fatal("partial conflict resolution was accepted")
	}
	resolved, err := db.Manifests().ResolveGalleryConflicts(ctx, created.ID, updated.MetadataRevision,
		map[string]manifest.ConflictChoice{"/title": manifest.ChooseFile}, now.Add(3*time.Minute))
	if err != nil || resolved.Status != ManifestClean {
		t.Fatalf("resolved Pull = %#v, %v", resolved, err)
	}
	galleryRecord, err := db.Galleries().Find(ctx, created.ID)
	if err != nil || galleryRecord.Title != "Manifest" {
		t.Fatalf("resolved Gallery = %#v, %v", galleryRecord, err)
	}
}

func TestGalleryManifestFirstPullCreatesStrictEntitiesAndBindsItems(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 1, 0, 0, 0, time.UTC)
	root := t.TempDir()
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Draft"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: root, Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "photos/01.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	coserUUID := "52345678-1234-4123-8123-123456789abc"
	workUUID := "62345678-1234-4123-8123-123456789abc"
	characterUUID := "72345678-1234-4123-8123-123456789abc"
	tagUUID := "82345678-1234-4123-8123-123456789abc"
	raw := `{
		"schema_version":1,"revision":0,"set_id":"` + created.SetID + `",
		"updated_at":"2026-07-23T01:00:00Z","title":"Imported set",
		"description":"## Imported","content_rating":"NON_ADULT","rating":4.5,
		"credits":[{"coser":{"uuid":"` + coserUUID + `","name_hint":"New Coser"},"position":1024}],
		"cast":[{"coser":{"uuid":"` + coserUUID + `","name_hint":"New Coser"},
			"work":{"uuid":"` + workUUID + `","name_hint":"New Work"},
			"character":{"uuid":"` + characterUUID + `","name_hint":"New Character"},"position":1024}],
		"tags":[{"uuid":"` + tagUUID + `","name_hint":"New Tag"}],
		"external_links":[{"type":"SOURCE","label":"Origin","url":"https://example.test/source","position":1024}],
		"items":[{"path":"photos/01.jpg","category":"SELFIE","caption":"Imported caption","rating":3.5}],
		"excluded_items":[{"path":"photos/01.jpg"}]
	}`
	manifestPath := filepath.Join(root, ".cosplay.json")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, "photos", "01.jpg")), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := db.Manifests().PullGallery(ctx, created.ID, created.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != ManifestClean {
		t.Fatalf("first Pull state = %#v", state)
	}
	updatedGallery, _ := db.Galleries().Find(ctx, created.ID)
	if updatedGallery.Title != "Imported set" || updatedGallery.ContentRating != gallery.ContentRatingNonAdult || updatedGallery.MetadataRevision != 2 {
		t.Fatalf("pulled Gallery = %#v", updatedGallery)
	}
	updatedItem, err := db.Galleries().FindItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedItem.UUID != item.UUID || updatedItem.ImageCategory != gallery.ImageCategorySelfie ||
		updatedItem.Caption != "Imported caption" || !updatedItem.Excluded {
		t.Fatalf("pulled Item = %#v", updatedItem)
	}
	var cosers, works, characters, tags, links, credits, cast int
	if err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM cosers WHERE uuid = ?),
			(SELECT COUNT(*) FROM works WHERE uuid = ?),
			(SELECT COUNT(*) FROM characters WHERE uuid = ? AND work_uuid = ?),
			(SELECT COUNT(*) FROM tags WHERE uuid = ?),
			(SELECT COUNT(*) FROM gallery_external_links WHERE gallery_id = ?),
			(SELECT COUNT(*) FROM gallery_credits WHERE gallery_id = ?),
			(SELECT COUNT(*) FROM gallery_cast WHERE gallery_id = ?)
	`, coserUUID, workUUID, characterUUID, workUUID, tagUUID,
		created.ID, created.ID, created.ID).Scan(&cosers, &works, &characters, &tags, &links, &credits, &cast); err != nil {
		t.Fatal(err)
	}
	if cosers != 1 || works != 1 || characters != 1 || tags != 1 || links != 1 || credits != 1 || cast != 1 {
		t.Fatalf("pulled relation counts = %d %d %d %d %d %d %d", cosers, works, characters, tags, links, credits, cast)
	}

	state, err = db.Manifests().PushGallery(ctx, created.ID, updatedGallery.MetadataRevision, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := manifest.ReadFile(state.Path, manifest.MaxGalleryBytes)
	if err != nil {
		t.Fatal(err)
	}
	systemDocument, err := manifest.ParseGallery(bytesReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(systemDocument.ExternalLinks.Value) != 1 || systemDocument.ExternalLinks.Value[0].LinkUUID == "" ||
		len(systemDocument.Items.Value) != 1 || systemDocument.Items.Value[0].ItemUUID != item.UUID {
		t.Fatalf("system Push did not complete local UUIDs: %#v", systemDocument)
	}
}
