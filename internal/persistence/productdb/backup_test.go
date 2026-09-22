package productdb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/portableid"
)

func TestBackupCreatesConsistentProductSnapshot(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	source, err := Open(ctx, filepath.Join(directory, "source.sqlite"))
	if err != nil {
		t.Fatalf("opening source database: %v", err)
	}
	defer source.Close()

	createdAt := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	if _, err := source.UUIDRegistry().Register(
		ctx,
		testGalleryUUIDA,
		portableid.KindGallery,
		createdAt,
	); err != nil {
		t.Fatalf("registering source UUID: %v", err)
	}

	targetPath := filepath.Join(directory, "snapshot.sqlite")
	if err := source.Backup(ctx, targetPath); err != nil {
		t.Fatalf("creating backup: %v", err)
	}

	snapshot, err := Open(ctx, targetPath)
	if err != nil {
		t.Fatalf("opening backup: %v", err)
	}
	defer snapshot.Close()

	record, err := snapshot.UUIDRegistry().Lookup(ctx, testGalleryUUIDA)
	if err != nil {
		t.Fatalf("looking up backed-up UUID: %v", err)
	}
	if record.Kind != portableid.KindGallery || !record.CreatedAtUTC.Equal(createdAt) {
		t.Fatalf("backed-up UUID = %#v", record)
	}
}

func TestBackupNeverOverwritesExistingFile(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	source, err := Open(ctx, filepath.Join(directory, "source.sqlite"))
	if err != nil {
		t.Fatalf("opening source database: %v", err)
	}
	defer source.Close()

	targetPath := filepath.Join(directory, "existing.sqlite")
	contents := []byte("keep me")
	if err := os.WriteFile(targetPath, contents, 0o600); err != nil {
		t.Fatalf("creating existing target: %v", err)
	}

	err = source.Backup(ctx, targetPath)
	if !errors.Is(err, ErrBackupTargetExists) {
		t.Fatalf("backup error = %v, want ErrBackupTargetExists", err)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("reading existing target: %v", err)
	}
	if string(got) != string(contents) {
		t.Fatalf("existing target changed: got %q, want %q", got, contents)
	}
}
