package mediaprocessing

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/gallery"
)

func TestPrimaryPlansKeepAnimationAndVideoScrubbersStatic(t *testing.T) {
	cases := []struct {
		kind    gallery.MediaKind
		format  gallery.ContentFormat
		variant string
		tier    CacheTier
	}{
		{gallery.MediaKindStaticImage, gallery.ContentFormatImage, VariantCard480, CacheBase},
		{gallery.MediaKindStaticImage, gallery.ContentFormatRAW, VariantCard480, CacheBase},
		{gallery.MediaKindAnimatedImage, gallery.ContentFormatImage, VariantStaticPoster, CacheBase},
		{gallery.MediaKindVideo, gallery.ContentFormatVideo, VariantStaticPoster, CacheBase},
	}
	for _, test := range cases {
		plan, err := PlanPrimary(test.kind, test.format)
		if err != nil || plan.Variant != test.variant || plan.CacheTier != test.tier || !plan.Static {
			t.Fatalf("primary plan for %s/%s = %#v, %v", test.kind, test.format, plan, err)
		}
		if (test.kind == gallery.MediaKindAnimatedImage || test.kind == gallery.MediaKindVideo) && !IsScrubberVariant(test.kind, plan.Variant) {
			t.Fatalf("%s primary Poster not accepted for Scrubber", test.kind)
		}
	}
	if IsScrubberVariant(gallery.MediaKindAnimatedImage, VariantAnimatedPreview) || IsScrubberVariant(gallery.MediaKindVideo, VariantVideoPlayback) {
		t.Fatal("playing derivative was accepted by static Scrubber contract")
	}
}

func TestCacheWriterUsesOpaqueIdentityPathAndRejectsTraversalAndSymlinks(t *testing.T) {
	root := t.TempDir()
	writer := CacheWriter{Root: root}
	uuid := "12345678-1234-4123-8123-123456789abc"
	relative, err := writer.RelativePath(uuid, 2, VariantCard480, "profile-hash", "webp")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(relative), []byte(uuid)) || bytes.Contains([]byte(relative), []byte("physical")) {
		t.Fatalf("cache identity path = %q", relative)
	}
	filename, size, err := writer.WriteAtomic(relative, func(destination io.Writer) error {
		_, err := destination.Write([]byte("generated"))
		return err
	})
	if err != nil || size != 9 {
		t.Fatalf("atomic cache write = %q, %d, %v", filename, size, err)
	}
	if data, err := os.ReadFile(filename); err != nil || string(data) != "generated" {
		t.Fatalf("generated cache = %q, %v", data, err)
	}
	if _, _, err := writer.WriteAtomic("../escape", func(io.Writer) error { return nil }); err == nil {
		t.Fatal("cache traversal was accepted")
	}
	symlinkRoot := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(symlinkRoot, "items")); err == nil {
		symlinkWriter := CacheWriter{Root: symlinkRoot}
		if _, _, err := symlinkWriter.WriteAtomic("items/a/file.webp", func(io.Writer) error { return nil }); err == nil {
			t.Fatal("cache symlink path was accepted")
		}
	}
}

func TestCacheWriterPreservesExtensionForPathGenerators(t *testing.T) {
	writer := CacheWriter{Root: t.TempDir()}
	const relative = "items/12/example/r1/profile/card-480.jpg"
	filename, size, err := writer.WriteAtomicPath(relative, func(destination string) error {
		if filepath.Ext(destination) != ".jpg" {
			t.Fatalf("temporary path extension = %q", filepath.Ext(destination))
		}
		return os.WriteFile(destination, []byte("generated jpeg"), 0o600)
	})
	if err != nil || size != int64(len("generated jpeg")) {
		t.Fatalf("atomic path write = %q, %d, %v", filename, size, err)
	}
	if filepath.Ext(filename) != ".jpg" {
		t.Fatalf("published path extension = %q", filepath.Ext(filename))
	}
}
