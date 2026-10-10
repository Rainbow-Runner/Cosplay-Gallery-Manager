package mediaprocessing

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/portableid"
)

func TestGeneratedCacheCleanupNamingAndSafeDeletion(t *testing.T) {
	writer := CacheWriter{Root: t.TempDir()}
	uuid := portableid.New()
	relative, err := writer.RelativePath(uuid, 1, VariantCard480, "primary", "jpg")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../photo.jpg", "items/00/unknown/card-480.jpg", relative + ".bak", filepath.ToSlash(filepath.Join(filepath.Dir(relative), "user-photo.jpg"))} {
		if _, ok := ParseGeneratedCachePath(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
	_, _, err = writer.WriteAtomic(relative, func(w io.Writer) error { _, err := w.Write([]byte("generated")); return err })
	if err != nil {
		t.Fatal(err)
	}
	info, err := writer.InspectForCleanup(relative)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.RemoveReviewedGenerated(relative, info); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(writer.Root, relative)); !os.IsNotExist(err) {
		t.Fatalf("not removed: %v", err)
	}
}

func TestCacheCleanupRejectsParentLinksHardlinksAndStaleIdentity(t *testing.T) {
	writer := CacheWriter{Root: t.TempDir()}
	uuid := portableid.New()
	relative, _ := writer.RelativePath(uuid, 1, VariantCard480, "primary", "jpg")
	_, _, err := writer.WriteAtomic(relative, func(w io.Writer) error { _, err := w.Write([]byte("generated")); return err })
	if err != nil {
		t.Fatal(err)
	}
	info, err := writer.InspectForCleanup(relative)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(writer.Root, relative), filepath.Join(t.TempDir(), "source.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := writer.RemoveReviewedGenerated(relative, info); err == nil {
		t.Fatal("hardlink accepted")
	}
	if _, err := writer.InspectForCleanup(relative); err == nil {
		t.Fatal("hardlink reviewed")
	}
	outside := t.TempDir()
	linked := CacheWriter{Root: t.TempDir()}
	if err := os.Symlink(outside, filepath.Join(linked.Root, "items")); err != nil {
		t.Fatal(err)
	}
	if err := linked.RemoveReviewedGenerated(relative, info); err == nil {
		t.Fatal("parent symlink accepted")
	}
	other := CacheWriter{Root: t.TempDir()}
	_, _, err = other.WriteAtomic(relative, func(w io.Writer) error { _, e := w.Write([]byte("different")); return e })
	if err != nil {
		t.Fatal(err)
	}
	if err := other.RemoveReviewedGenerated(relative, info); err == nil {
		t.Fatal("changed inode accepted")
	}
}

func TestCacheCleanupResumesQuarantinedFile(t *testing.T) {
	writer := CacheWriter{Root: t.TempDir()}
	relative, _ := writer.RelativePath(portableid.New(), 1, VariantCard480, "primary", "jpg")
	_, _, err := writer.WriteAtomic(relative, func(w io.Writer) error { _, e := w.Write([]byte("generated")); return e })
	if err != nil {
		t.Fatal(err)
	}
	quarantine := filepath.Join(writer.Root, filepath.Dir(relative), ".gc-"+filepath.Base(relative))
	if err := os.Rename(filepath.Join(writer.Root, relative), quarantine); err != nil {
		t.Fatal(err)
	}
	info, err := writer.InspectForCleanup(relative)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.RemoveReviewedGenerated(relative, info); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(quarantine); !os.IsNotExist(err) {
		t.Fatal("quarantine leaked")
	}
}

func TestCacheWalkIsBoundedAndResumable(t *testing.T) {
	writer := CacheWriter{Root: t.TempDir()}
	uuid := "00111111-1111-4111-8111-111111111111"
	for _, variant := range []string{VariantCard480, VariantCard960, VariantCard1600} {
		relative, _ := writer.RelativePath(uuid, 1, variant, "primary", "jpg")
		_, _, err := writer.WriteAtomic(relative, func(w io.Writer) error { _, e := w.Write([]byte("generated")); return e })
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for i := 0; i < 5; i++ {
		last, complete, _, err := writer.WalkCleanupShard(0, cursor, 7, time.Now().Add(time.Second), func(path string, _ os.FileInfo) error { seen[path] = true; return nil })
		if err != nil {
			t.Fatal(err)
		}
		cursor = last
		if complete {
			break
		}
	}
	if len(seen) != 3 {
		t.Fatalf("cursor lost files: %v", seen)
	}
}
