package productserver

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/settings"
)

func TestMediaEmbeddedMetadataReadsCurrentSourceWithoutPersistence(t *testing.T) {
	ctx := context.Background()
	sourceRoot, cacheRoot := t.TempDir(), t.TempDir()
	sourcePath := filepath.Join(sourceRoot, "photo.jpg")
	writeMetadataTestJPEG(t, sourcePath, 3, 2)
	database, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 8, 18, 18, 0, 0, 0, time.UTC)
	created, err := database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Realtime metadata", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := database.Galleries().AddSource(ctx, created.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := database.Galleries().AddItem(ctx, created.ID, source.ID, productdb.CreateItemInput{RelativePath: "photo.jpg", MediaKind: gallery.MediaKindStaticImage,
		ContentFormat: gallery.ContentFormatImage, ImageCategory: gallery.ImageCategoryPhoto, Position: 1024,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',added_at_utc=? WHERE id=?`, now.Format("2006-01-02T15:04:05Z"), created.ID); err != nil {
		t.Fatal(err)
	}
	server := &Server{Config: Config{CachePath: cacheRoot}, Database: database}
	visible := []string{settings.MetadataFileDimensions}
	first, err := server.MediaEmbeddedMetadata(ctx, item.UUID, visible)
	if err != nil {
		t.Fatal(err)
	}
	if !containsMetadataValue(first, settings.MetadataFileDimensions, "3 × 2") {
		t.Fatalf("first metadata = %#v", first)
	}
	if len(first.Entries) != 1 {
		t.Fatalf("server returned fields outside the requested visibility set: %#v", first.Entries)
	}
	writeMetadataTestJPEG(t, sourcePath, 7, 4)
	second, err := server.MediaEmbeddedMetadata(ctx, item.UUID, visible)
	if err != nil {
		t.Fatal(err)
	}
	if !containsMetadataValue(second, settings.MetadataFileDimensions, "7 × 4") {
		t.Fatalf("second metadata did not reflect source update = %#v", second)
	}
	var persistedTables int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name='image_embedded_metadata'`).Scan(&persistedTables); err != nil || persistedTables != 0 {
		t.Fatalf("image metadata persistence table count=%d error=%v", persistedTables, err)
	}
}

func containsMetadataValue(result browse.MediaInformationSummary, key, value string) bool {
	for _, entry := range result.Entries {
		if entry.VisibilityKey == key && entry.Value == value {
			return true
		}
	}
	return false
}

func writeMetadataTestJPEG(t *testing.T, path string, width, height int) {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value.Set(x, y, color.RGBA{R: 120, G: 80, B: 200, A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, value, nil); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
