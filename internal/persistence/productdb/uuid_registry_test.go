package productdb

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/portableid"
)

const (
	testGalleryUUIDA = "550e8400-e29b-41d4-a716-446655440000"
	testGalleryUUIDB = "01890f5c-7b2a-7cc0-98c4-dc0c0c07398f"
	testGalleryUUIDC = "6ba7b810-9dad-41d1-80b4-00c04fd430c8"
)

func TestPortableUUIDRegistryRejectsCrossKindReuse(t *testing.T) {
	ctx := context.Background()
	registry := openTestRegistry(t)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

	if _, err := registry.Register(ctx, testGalleryUUIDA, portableid.KindGallery, now); err != nil {
		t.Fatalf("registering Gallery UUID: %v", err)
	}
	_, err := registry.Register(ctx, testGalleryUUIDA, portableid.KindCoser, now)
	var conflict *PortableUUIDKindConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("reuse error = %v, want PortableUUIDKindConflictError", err)
	}
	if conflict.Found != portableid.KindGallery || conflict.Requested != portableid.KindCoser {
		t.Fatalf("unexpected kind conflict: %#v", conflict)
	}
}

func TestPortableUUIDAliasResolvesPermanentMergeChain(t *testing.T) {
	ctx := context.Background()
	registry := openTestRegistry(t)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

	for _, value := range []string{testGalleryUUIDA, testGalleryUUIDB, testGalleryUUIDC} {
		if _, err := registry.Register(ctx, value, portableid.KindGallery, now); err != nil {
			t.Fatalf("registering %s: %v", value, err)
		}
	}
	if err := registry.Alias(
		ctx,
		testGalleryUUIDA,
		testGalleryUUIDB,
		portableid.KindGallery,
		now.Add(time.Minute),
	); err != nil {
		t.Fatalf("merging A into B: %v", err)
	}
	if err := registry.Alias(
		ctx,
		testGalleryUUIDB,
		testGalleryUUIDC,
		portableid.KindGallery,
		now.Add(2*time.Minute),
	); err != nil {
		t.Fatalf("merging B into C: %v", err)
	}

	resolved, err := registry.Resolve(ctx, testGalleryUUIDA)
	if err != nil {
		t.Fatalf("resolving merge chain: %v", err)
	}
	if resolved.UUID != testGalleryUUIDC || resolved.State != PortableUUIDActive {
		t.Fatalf("resolved record = %#v, want active C", resolved)
	}

	if _, err := registry.Register(
		ctx,
		testGalleryUUIDA,
		portableid.KindGallery,
		now.Add(3*time.Minute),
	); !errors.Is(err, ErrPortableUUIDOccupied) {
		t.Fatalf("re-registering alias error = %v, want ErrPortableUUIDOccupied", err)
	}
}

func TestPortableUUIDTombstonePreventsRecreation(t *testing.T) {
	ctx := context.Background()
	registry := openTestRegistry(t)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

	if _, err := registry.Register(ctx, testGalleryUUIDA, portableid.KindGallery, now); err != nil {
		t.Fatalf("registering Gallery UUID: %v", err)
	}
	if err := registry.Tombstone(
		ctx,
		testGalleryUUIDA,
		portableid.KindGallery,
		"gallery deleted",
		now.Add(time.Minute),
	); err != nil {
		t.Fatalf("tombstoning Gallery UUID: %v", err)
	}

	record, err := registry.Lookup(ctx, testGalleryUUIDA)
	if err != nil {
		t.Fatalf("looking up Tombstone: %v", err)
	}
	if record.State != PortableUUIDTombstone || record.Reason != "gallery deleted" {
		t.Fatalf("tombstone record = %#v", record)
	}
	if _, err := registry.Resolve(ctx, testGalleryUUIDA); !errors.Is(err, ErrPortableUUIDTombstoned) {
		t.Fatalf("resolving Tombstone error = %v, want ErrPortableUUIDTombstoned", err)
	}
	if _, err := registry.Register(
		ctx,
		testGalleryUUIDA,
		portableid.KindGallery,
		now.Add(2*time.Minute),
	); !errors.Is(err, ErrPortableUUIDOccupied) {
		t.Fatalf("re-registering Tombstone error = %v, want ErrPortableUUIDOccupied", err)
	}
}

func TestPortableUUIDHistoryRowsCannotBeDeleted(t *testing.T) {
	ctx := context.Background()
	db, registry := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

	if _, err := registry.Register(ctx, testGalleryUUIDA, portableid.KindGallery, now); err != nil {
		t.Fatalf("registering Gallery UUID: %v", err)
	}
	if err := registry.Tombstone(
		ctx,
		testGalleryUUIDA,
		portableid.KindGallery,
		"gallery deleted",
		now,
	); err != nil {
		t.Fatalf("tombstoning Gallery UUID: %v", err)
	}

	if _, err := db.ExecContext(
		ctx,
		`DELETE FROM portable_uuid_tombstones WHERE uuid = ?`,
		testGalleryUUIDA,
	); err == nil {
		t.Fatal("deleting a Tombstone unexpectedly succeeded")
	}
	if _, err := db.ExecContext(
		ctx,
		`DELETE FROM portable_uuid_registry WHERE uuid = ?`,
		testGalleryUUIDA,
	); err == nil {
		t.Fatal("deleting a registry allocation unexpectedly succeeded")
	}
}

func openTestRegistry(t *testing.T) *UUIDRegistry {
	t.Helper()
	_, registry := openTestDatabaseAndRegistry(t)
	return registry
}

func openTestDatabaseAndRegistry(t *testing.T) (*Database, *UUIDRegistry) {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatalf("opening test product database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test product database: %v", err)
		}
	})
	return db, db.UUIDRegistry()
}
