package processingworker

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

type lifecycleFixture struct {
	service CacheLifecycleService
	gallery gallery.Gallery
	item    gallery.Item
	now     time.Time
}

func newLifecycleFixture(t *testing.T) lifecycleFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	g, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Cache lifecycle"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, g.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: t.TempDir(), Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, g.ID, source.ID, productdb.CreateItemInput{RelativePath: "photo.jpg", MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage, ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingPending}, now)
	if err != nil {
		t.Fatal(err)
	}
	return lifecycleFixture{service: CacheLifecycleService{Database: db, Cache: mediaprocessing.CacheWriter{Root: t.TempDir()}}, gallery: g, item: item, now: now}
}

func (f lifecycleFixture) generate(t *testing.T, revision int64, variant, profile string, publish bool) string {
	t.Helper()
	relative, err := f.service.Cache.RelativePath(f.item.UUID, revision, variant, profile, "jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, size, err := f.service.Cache.WriteAtomic(relative, func(w io.Writer) error { _, err := w.Write([]byte("generated")); return err })
	if err != nil {
		t.Fatal(err)
	}
	if publish {
		tier, _ := mediaprocessing.RequiredCacheTier(variant)
		_, err = f.service.Database.Derivatives().Publish(context.Background(), productdb.PublishDerivativeInput{ItemUUID: f.item.UUID, Variant: variant, ContentRevision: revision, ProfileHash: profile, CacheTier: tier, CacheRelativePath: relative, MIMEType: "image/jpeg", ByteSize: size}, f.now)
		if err != nil {
			t.Fatal(err)
		}
	}
	return relative
}
func (f lifecycleFixture) exists(relative string) bool {
	_, err := os.Stat(filepath.Join(f.service.Cache.Root, relative))
	return err == nil
}

func TestCacheLifecycleContentReplacementReclaimsOldTiersBeforeNewBase(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	oldBase := f.generate(t, 1, mediaprocessing.VariantCard480, "old", true)
	oldEnhanced := f.generate(t, 1, mediaprocessing.VariantLightbox4096, "old", true)
	if _, err := f.service.Database.ExecContext(ctx, `UPDATE gallery_items SET content_revision=2 WHERE item_uuid=?; UPDATE media_derivatives SET state='HARD_INVALID',is_current=0 WHERE item_uuid=?`, f.item.UUID, f.item.UUID); err != nil {
		t.Fatal(err)
	}
	result, err := f.service.Maintain(ctx, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 2 || f.exists(oldBase) || f.exists(oldEnhanced) {
		t.Fatalf("old content cache not reclaimed before new base: %+v", result)
	}
	newBase := f.generate(t, 2, mediaprocessing.VariantCard480, "new", true)
	result, err = f.service.Maintain(ctx, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 0 || f.exists(oldBase) || f.exists(oldEnhanced) || !f.exists(newBase) {
		t.Fatalf("replacement cleanup=%+v", result)
	}
}

func TestSourceScanContentReplacementReclaimsOldGeneratedFiles(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	var sourceID int64
	var sourcePath string
	if err := f.service.Database.QueryRowContext(ctx, `SELECT id,source_path FROM gallery_sources WHERE gallery_id=?`, f.gallery.ID).Scan(&sourceID, &sourcePath); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sourcePath, "photo.jpg")
	writeSource := func(body string, modified time.Time) {
		t.Helper()
		if err := os.WriteFile(file, []byte("\xff\xd8\xff "+body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(file, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	writeSource("original", f.now.Add(-time.Hour))
	if err := f.service.Database.Scans().Run(ctx, sourceID, archivecheck.DefaultLimits(), f.now); err != nil {
		t.Fatal(err)
	}
	oldBase := f.generate(t, 1, mediaprocessing.VariantCard480, "old", true)
	oldEnhanced := f.generate(t, 1, mediaprocessing.VariantLightbox4096, "old", true)
	writeSource("replacement-with-different-bytes", f.now.Add(time.Minute))
	if err := f.service.Database.Scans().Run(ctx, sourceID, archivecheck.DefaultLimits(), f.now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := f.service.Database.QueryRowContext(ctx, `SELECT content_revision FROM gallery_items WHERE item_uuid=?`, f.item.UUID).Scan(&revision); err != nil || revision != 2 {
		t.Fatalf("source scan did not advance content revision: %d, %v", revision, err)
	}
	result, err := f.service.Maintain(ctx, f.now.Add(3*time.Minute))
	if err != nil || result.Removed != 2 || f.exists(oldBase) || f.exists(oldEnhanced) {
		t.Fatalf("old generated files remained after changed-source scan: %+v, %v", result, err)
	}
}

func TestCacheLifecycleSameRevisionHardInvalidWaitsForReplacement(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	old := f.generate(t, 1, mediaprocessing.VariantCard480, "old", true)
	if _, err := f.service.Database.ExecContext(ctx, `UPDATE gallery_items SET availability_state='MISSING' WHERE item_uuid=?;
		UPDATE media_derivatives SET state='HARD_INVALID',is_current=0 WHERE item_uuid=?`, f.item.UUID, f.item.UUID); err != nil {
		t.Fatal(err)
	}
	result, err := f.service.Maintain(ctx, f.now)
	if err != nil || result.Removed != 0 || !f.exists(old) {
		t.Fatalf("same-revision missing source should retain cache: %+v err=%v", result, err)
	}
}

func TestCacheLifecycleProfileReplacementButMissingAndExcludedKeepCurrentBase(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	old := f.generate(t, 1, mediaprocessing.VariantCard480, "old", true)
	if err := f.service.Database.Derivatives().MarkProfileStale(ctx, f.item.UUID, mediaprocessing.VariantCard480, "new"); err != nil {
		t.Fatal(err)
	}
	if result, err := f.service.Maintain(ctx, f.now); err != nil || result.Removed != 0 {
		t.Fatalf("profile premature cleanup=%+v %v", result, err)
	}
	current := f.generate(t, 1, mediaprocessing.VariantCard480, "new", true)
	if _, err := f.service.Database.ExecContext(ctx, `UPDATE gallery_items SET availability_state='MISSING',excluded=1 WHERE item_uuid=?`, f.item.UUID); err != nil {
		t.Fatal(err)
	}
	result, err := f.service.Maintain(ctx, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || f.exists(old) || !f.exists(current) {
		t.Fatalf("profile cleanup=%+v", result)
	}
	result, err = f.service.Maintain(ctx, f.now)
	if err != nil || result.Removed != 0 || !f.exists(current) {
		t.Fatalf("missing current removed=%+v %v", result, err)
	}
}

func TestCacheLifecycleForgetPersistsOutboxAndRetriesAfterReopen(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	relative := f.generate(t, 1, mediaprocessing.VariantCard480, "primary", true)
	if _, err := f.service.Database.ExecContext(ctx, `UPDATE gallery_items SET excluded=1 WHERE item_uuid=?`, f.item.UUID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Database.Galleries().ForgetItem(ctx, f.item.ID, f.gallery.MetadataRevision, f.now); err != nil {
		t.Fatal(err)
	}
	entries, err := f.service.Database.Derivatives().CleanupEntries(ctx, 100)
	if err != nil || len(entries) != 1 {
		t.Fatalf("outbox=%+v %v", entries, err)
	}
	removed, failed, err := f.service.Database.Derivatives().CleanupCacheEntry(ctx, entries[0], true, f.now, func() error { return errors.New("simulated file permission failure") })
	if err != nil || removed || !failed {
		t.Fatalf("failure=%v %v %v", removed, failed, err)
	}
	path := f.service.Database.Path()
	f.service.Database.Close()
	reopened, err := productdb.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	f.service.Database = reopened
	result, err := f.service.Maintain(ctx, f.now.Add(time.Minute))
	if err != nil || result.Removed != 0 {
		t.Fatalf("backoff=%+v %v", result, err)
	}
	result, err = f.service.Maintain(ctx, f.now.Add(16*time.Minute))
	if err != nil || result.Removed != 1 || f.exists(relative) {
		t.Fatalf("retry=%+v %v", result, err)
	}
	stats, err := reopened.Derivatives().LifecycleSummary(ctx)
	if err != nil || stats.PendingFiles != 0 {
		t.Fatalf("pending=%+v %v", stats, err)
	}
}

func TestCacheLifecycleOrphanGraceGenerationProtectionAndReadonlyReview(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	orphan := f.generate(t, 1, mediaprocessing.VariantCard960, "orphan", false)
	review, err := f.service.Review(ctx, f.now)
	if err != nil || len(review.Candidates) != 0 {
		t.Fatalf("new orphan=%+v %v", review, err)
	}
	old := f.now.Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(f.service.Cache.Root, orphan), old, old); err != nil {
		t.Fatal(err)
	}
	var before, after int64
	f.service.Database.QueryRow(`SELECT total_changes()`).Scan(&before)
	review, err = f.service.Review(ctx, f.now)
	if err != nil || len(review.Candidates) != 1 || review.OrphanBytes != 9 {
		t.Fatalf("orphan=%+v %v", review, err)
	}
	f.service.Database.QueryRow(`SELECT total_changes()`).Scan(&after)
	if before != after {
		t.Fatal("preview mutated database")
	}
	revision := int64(1)
	job, err := f.service.Database.ProcessingJobs().Enqueue(ctx, productdb.EnqueueJobInput{Key: productdb.ItemDerivativeJobKey(f.item.UUID, mediaprocessing.VariantCard960, 1, "orphan"), Kind: mediaprocessing.JobItemDerivative, GalleryID: &f.gallery.ID, ItemUUID: f.item.UUID, Variant: mediaprocessing.VariantCard960, ContentRevision: &revision, ProfileHash: "orphan"}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.CleanupSelected(ctx, []string{review.Candidates[0].ID}, f.now); !errors.Is(err, productdb.ErrCacheReviewStale) {
		t.Fatalf("generation not protected: %v", err)
	}
	if _, err := f.service.Database.ExecContext(ctx, `UPDATE processing_jobs SET status='CANCELLED' WHERE id=?`, job.ID); err != nil {
		t.Fatal(err)
	}
	shard, _ := strconv.ParseInt(f.item.UUID[:2], 16, 32)
	if err := f.service.Database.Derivatives().SaveCacheScanCursor(ctx, int(shard), ""); err != nil {
		t.Fatal(err)
	}
	result, err := f.service.Maintain(ctx, f.now)
	if err != nil || result.Removed != 1 || f.exists(orphan) {
		t.Fatalf("orphan auto=%+v %v", result, err)
	}
}

func TestCacheLifecycleRejectsChangedPreviewAndKeepsSourceFiles(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	old := f.generate(t, 1, mediaprocessing.VariantCard480, "old", true)
	f.generate(t, 1, mediaprocessing.VariantCard480, "new", true)
	review, err := f.service.Review(ctx, f.now)
	if err != nil || len(review.Candidates) != 1 {
		t.Fatalf("review=%+v %v", review, err)
	}
	if _, _, err := f.service.Cache.WriteAtomic(old, func(w io.Writer) error { _, e := w.Write([]byte("changed")); return e }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.CleanupSelected(ctx, []string{review.Candidates[0].ID}, f.now); !errors.Is(err, productdb.ErrCacheReviewStale) {
		t.Fatalf("stale=%v", err)
	}
	if !f.exists(old) {
		t.Fatal("changed file was deleted")
	}
}

func TestCacheLifecycleAggregateDeleteQueuesCacheButPreservesMediaAndManifest(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	relative := f.generate(t, 1, mediaprocessing.VariantCard480, "primary", true)
	if _, err := f.service.Database.ExecContext(ctx, `UPDATE galleries SET state='ARCHIVED' WHERE id=?`, f.gallery.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Database.Galleries().Delete(ctx, f.gallery.SetID, f.gallery.MetadataRevision, f.now); err != nil {
		t.Fatal(err)
	}
	stats, err := f.service.Database.Derivatives().LifecycleSummary(ctx)
	if err != nil || stats.PendingFiles != 1 {
		t.Fatalf("cascade outbox=%+v %v", stats, err)
	}
	result, err := f.service.Maintain(ctx, f.now)
	if err != nil || result.Removed != 1 || f.exists(relative) {
		t.Fatalf("cascade cleanup=%+v %v", result, err)
	}
}

func TestCacheLifecycleRetirementIsDurableBeforeUnlinkAndRepublishCancelsDeletion(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	old := f.generate(t, 1, mediaprocessing.VariantCard480, "old", true)
	f.generate(t, 1, mediaprocessing.VariantCard480, "new", true)
	entries, err := f.service.Database.Derivatives().CleanupEntries(ctx, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%+v %v", entries, err)
	}
	// A separate reader sees the committed retirement even while the deletion
	// transaction is open; a crash cannot resurrect a served reference.
	reader, err := sql.Open("sqlite3", f.service.Database.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	_, failed, err := f.service.Database.Derivatives().CleanupCacheEntry(ctx, entries[0], false, f.now, func() error {
		var references, pending int
		if err := reader.QueryRow(`SELECT count(*) FROM media_derivatives WHERE cache_relative_path=?`, old).Scan(&references); err != nil {
			return err
		}
		if err := reader.QueryRow(`SELECT count(*) FROM cache_cleanup_outbox WHERE cache_relative_path=?`, old).Scan(&pending); err != nil {
			return err
		}
		if references != 0 || pending != 1 {
			t.Errorf("retirement not committed before filesystem action: references=%d pending=%d", references, pending)
		}
		return errors.New("simulated deletion failure")
	})
	if err != nil || !failed {
		t.Fatalf("failed cleanup=%v %v", failed, err)
	}
	f.generate(t, 1, mediaprocessing.VariantCard480, "old", true)
	var pending int
	if err := f.service.Database.QueryRow(`SELECT count(*) FROM cache_cleanup_outbox WHERE cache_relative_path=?`, old).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("republish did not cancel pending deletion: %d %v", pending, err)
	}
	result, err := f.service.Maintain(ctx, f.now.Add(16*time.Minute))
	if err != nil || !f.exists(old) || result.Failed != 0 {
		t.Fatalf("republished file not protected: %+v %v", result, err)
	}
}
