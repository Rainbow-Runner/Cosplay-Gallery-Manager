package productdb

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestMostSpecificMediaLibraryOwnsPathEvenWhenDisabled(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Libraries()
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	root := t.TempDir()

	parent, err := store.Create(ctx, CreateLibraryInput{
		Name: "Parent", RootPath: root, Enabled: true, CaptureTimezone: "Asia/Shanghai",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.Create(ctx, CreateLibraryInput{
		Name: "Disabled child", RootPath: filepath.Join(root, "cosplay"), Enabled: false,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	owner, err := store.OwnerForPath(ctx, filepath.Join(root, "cosplay", "set-a"))
	if err != nil {
		t.Fatal(err)
	}
	if owner == nil || owner.ID != child.ID {
		t.Fatalf("owner = %#v, want disabled child %d", owner, child.ID)
	}
	boundaries, err := store.ChildBoundaries(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(boundaries) != 1 || boundaries[0].ID != child.ID {
		t.Fatalf("child boundaries = %#v", boundaries)
	}
}

func TestLibraryChangeRequiresExplicitSourceAssignment(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	libraries := db.Libraries()
	galleries := db.Galleries()
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	root := t.TempDir()

	parent, err := libraries.Create(ctx, CreateLibraryInput{Name: "Parent", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	childRoot := filepath.Join(root, "child")
	child, err := libraries.Create(ctx, CreateLibraryInput{Name: "Child", RootPath: childRoot, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := galleries.Create(ctx, CreateGalleryInput{Title: "Set"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := galleries.AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &child.ID, Type: gallery.SourceTypeDirectory,
		Path: filepath.Join(childRoot, "set"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	preview, err := libraries.PreviewChange(ctx, child.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Impacts) != 1 || preview.Impacts[0].SuggestedOwner == nil ||
		*preview.Impacts[0].SuggestedOwner != parent.ID {
		t.Fatalf("change preview = %#v", preview)
	}
	persisted, err := findSource(ctx, db.DB, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.LibraryID == nil || *persisted.LibraryID != child.ID {
		t.Fatal("preview silently reassigned GallerySource")
	}

	if err := libraries.AssignSource(ctx, source.ID, nil); err != nil {
		t.Fatal(err)
	}
	persisted, err = findSource(ctx, db.DB, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.LibraryID != nil {
		t.Fatalf("unassigned Source still has library %v", persisted.LibraryID)
	}
	if err := libraries.AssignSource(ctx, source.ID, &parent.ID); err != nil {
		t.Fatal(err)
	}
}

func TestGallerySourceCannotBindOutsideSelectedLibrary(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	mediaLibrary, err := db.Libraries().Create(ctx, CreateLibraryInput{
		Name: "Library", RootPath: filepath.Join(t.TempDir(), "library"), Enabled: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Set"}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &mediaLibrary.ID, Type: gallery.SourceTypeDirectory,
		Path: filepath.Join(t.TempDir(), "outside"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err == nil {
		t.Fatal("GallerySource outside selected library unexpectedly succeeded")
	}
}
