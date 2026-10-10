package productserver

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/productapi"
)

func TestPortableExportOwnerContinuityIsExplicitAndExcludesRatingAndHistory(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	if _, err := server.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, filepath.Join(root, "cosers"), filepath.Join(root, "backups"), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 11, 0, 0, 0, time.UTC)
	gallery, _, item := createPortableOwnerFixture(t, server, now)
	if err := server.Database.PersonalStates().SetGalleryFavorite(ctx, gallery.ID, true, now); err != nil {
		t.Fatal(err)
	}
	if err := server.Database.PersonalStates().SetGalleryHidden(ctx, gallery.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := server.Database.PersonalStates().SetItemFavorite(ctx, item.ID, true, now); err != nil {
		t.Fatal(err)
	}
	if err := server.Database.PersonalStates().RecordGalleryView(ctx, gallery.ID, &item.ID, now); err != nil {
		t.Fatal(err)
	}
	without := filepath.Join(root, "without.zip")
	if _, err := server.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: without, AllowIncompleteGallery: true}); err != nil {
		t.Fatal(err)
	}
	plain, err := portablecatalog.InspectFile(ctx, without)
	if err != nil || plain.Bundle.Owner != nil {
		t.Fatalf("default export owner = %#v, err=%v", plain.Bundle.Owner, err)
	}
	with := filepath.Join(root, "with.zip")
	if _, err := server.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: with, AllowIncompleteGallery: true, IncludePersonalFlags: true}); err != nil {
		t.Fatal(err)
	}
	inspection, err := portablecatalog.InspectFile(ctx, with)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := portablecatalog.ReadPackageManifest(ctx, with)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.OwnerContinuity || manifest.OwnerGalleryLifecycle || !manifest.OwnerPersonalFlags || manifest.OwnerGalleryCount != 1 || manifest.OwnerItemCount != 1 {
		t.Fatalf("owner continuity package summary = %#v", manifest)
	}
	owner := inspection.Bundle.Owner
	if owner == nil || owner.IncludesGalleryLifecycle || !owner.IncludesPersonalFlags || len(owner.Galleries) != 1 || !owner.Galleries[0].Favorite || !owner.Galleries[0].Hidden || len(owner.Galleries[0].Items) != 1 {
		t.Fatalf("owner continuity = %#v", owner)
	}
	encoded, err := json.Marshal(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{[]byte(`"last_viewed_at"`), []byte(`"last_item"`), []byte(`"rating_half_steps"`), []byte(`"slug"`)} {
		if bytes.Contains(encoded, forbidden) {
			t.Fatalf("portable owner entry contains forbidden field %q", forbidden)
		}
	}
}

func TestPortableWorkbenchReturnsExportPreflightSummary(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	if _, err := server.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, filepath.Join(root, "cosers"), filepath.Join(root, "backups"), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	createPortableOwnerFixture(t, server, time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC))
	result, err := server.RunPortableMigration(ctx, productapi.PortableMigrationRequest{Action: "PREFLIGHT_EXPORT", Confirmation: "PREFLIGHT"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Code != "PORTABLE_PREFLIGHT_EXPORT_COMPLETED" || result.Preflight == nil || result.Preflight.GalleryCount != 1 || result.Snapshot.Owner != nil {
		t.Fatalf("portable workbench preflight result = %#v", result)
	}
}

func createPortableOwnerFixture(t *testing.T, server *Server, now time.Time) (gallery.Gallery, gallery.Source, gallery.Item) {
	t.Helper()
	created, err := server.Database.Galleries().Create(context.Background(), productdb.CreateGalleryInput{Title: "Owner continuity"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := server.Database.Galleries().AddSource(context.Background(), created.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: filepath.Join(t.TempDir(), "gallery"), Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := server.Database.Galleries().AddItem(context.Background(), created.ID, source.ID, productdb.CreateItemInput{RelativePath: "one.jpg", MediaKind: gallery.MediaKindStaticImage, ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady}, now)
	if err != nil {
		t.Fatal(err)
	}
	return created, source, item
}
