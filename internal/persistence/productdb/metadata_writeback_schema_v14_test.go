package productdb

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestSchemaV13MigratesExistingLibraryWritebackPolicy(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "product.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	oldReadOnly, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Old read-only", RootPath: t.TempDir(), Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	oldWritable, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Old writable", RootPath: t.TempDir(), Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `UPDATE media_libraries SET read_only=1 WHERE id=?`, oldReadOnly.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `UPDATE media_libraries SET read_only=0 WHERE id=?`, oldWritable.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `DROP TABLE item_capture_dates; DROP TABLE gallery_capture_date_reviews; ALTER TABLE galleries DROP COLUMN shoot_date_origin;
		ALTER TABLE media_libraries DROP COLUMN metadata_writeback_enabled;
		UPDATE cgm_product_identity SET database_schema_version=13 WHERE singleton_id=1`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	if migrated.Identity().DatabaseSchemaVersion != 15 {
		t.Fatalf("schema=%d", migrated.Identity().DatabaseSchemaVersion)
	}
	for _, check := range []struct {
		id       int64
		expected bool
	}{{oldReadOnly.ID, false}, {oldWritable.ID, true}} {
		value, err := migrated.Libraries().Find(ctx, check.id)
		if err != nil || value.MetadataWritebackEnabled != check.expected {
			t.Fatalf("library %d writeback=%v, err=%v", check.id, value.MetadataWritebackEnabled, err)
		}
	}
	newLibrary, err := migrated.Libraries().Create(ctx, CreateLibraryInput{Name: "New default", RootPath: t.TempDir(), Enabled: true}, now)
	if err != nil || !newLibrary.MetadataWritebackEnabled {
		t.Fatalf("new library default=%#v, err=%v", newLibrary, err)
	}
	if _, err := migrated.Libraries().SetMetadataWriteback(ctx, oldReadOnly.ID, true, "stale", now.Add(time.Minute)); err == nil {
		t.Fatal("stale writeback update succeeded")
	}
	updated, err := migrated.Libraries().SetMetadataWriteback(ctx, oldReadOnly.ID, true, oldReadOnly.UpdatedAtUTC.Format(time.RFC3339Nano), now.Add(time.Minute))
	if err != nil || !updated.MetadataWritebackEnabled {
		t.Fatalf("enabled writeback=%#v, err=%v", updated, err)
	}
}

func TestMetadataWritebackGateDoesNotModifySourceMedia(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "source.jpg")
	original := []byte("original source media")
	if err := os.WriteFile(mediaPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Media read-only", RootPath: root, Enabled: true}, now)
	if err != nil || !library.MetadataWritebackEnabled {
		t.Fatalf("new library = %#v, %v", library, err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Sidecar"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{LibraryID: &library.ID, Type: gallery.SourceTypeDirectory, Path: root, Availability: gallery.AvailabilityAvailable}, now); err != nil {
		t.Fatal(err)
	}
	disabled, err := db.Libraries().SetMetadataWriteback(ctx, library.ID, false, library.UpdatedAtUTC.Format(time.RFC3339Nano), now.Add(time.Second))
	if err != nil || disabled.MetadataWritebackEnabled {
		t.Fatalf("disabled = %#v, %v", disabled, err)
	}
	preview, err := db.Manifests().PreviewGalleryBatchPush(ctx, created.ID, now)
	if err != nil || preview.BlockReason != "METADATA_WRITEBACK_DISABLED" {
		t.Fatalf("blocked preview = %#v, %v", preview, err)
	}
	if _, err := db.Manifests().PushGallery(ctx, created.ID, created.MetadataRevision, now); err == nil {
		t.Fatal("single Push bypassed disabled metadata writeback")
	}
	enabled, err := db.Libraries().SetMetadataWriteback(ctx, library.ID, true, disabled.UpdatedAtUTC.Format(time.RFC3339Nano), now.Add(2*time.Second))
	if err != nil || !enabled.MetadataWritebackEnabled {
		t.Fatalf("enabled = %#v, %v", enabled, err)
	}
	preview, err = db.Manifests().PreviewGalleryBatchPush(ctx, created.ID, now)
	if err != nil || preview.BlockReason != "" {
		t.Fatalf("allowed preview = %#v, %v", preview, err)
	}
	if _, err := db.Manifests().PushGallery(ctx, created.ID, created.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	if current, err := os.ReadFile(mediaPath); err != nil || string(current) != string(original) {
		t.Fatalf("source media changed: %q, %v", current, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".cosplay.json")); err != nil {
		t.Fatalf("sidecar missing: %v", err)
	}
}
