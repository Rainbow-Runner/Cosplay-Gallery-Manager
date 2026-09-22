package productdb

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
)

func TestGalleryItemExclusionSurvivesScanAndAllowsEffectiveLimit(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 19, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	item, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "excluded.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	current, err := db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().SetItemExcluded(ctx, item.ID, true, current.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}

	runID, err := db.Scans().Begin(ctx, source.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 1001; index++ {
		path := fmt.Sprintf("%04d.jpg", index)
		fingerprintValue := fingerprint(path)
		if index == 0 {
			path = "excluded.jpg"
			fingerprintValue = fingerprint("excluded")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_scan_observations (
				scan_run_id, relative_path, media_kind, image_category,
				full_fingerprint, processing_state
			) VALUES (?, ?, 'STATIC_IMAGE', 'PHOTO', ?, 'READY')
		`, runID, path, fingerprintValue); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Commit(ctx, runID, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("effective 1000-member scan failed: %v", err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	if len(items) != 1001 {
		t.Fatalf("stored members = %d, want 1001 including excluded", len(items))
	}
	persisted, err := db.Galleries().FindItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.Excluded {
		t.Fatal("rescan silently restored excluded GalleryItem")
	}
}

func TestGalleryItemRestoreDoesNotResumeManuallyCancelledJob(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	runID, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Stage(ctx, runID, scanPendingPhoto("manual-cancel.jpg", "manual-cancel")); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Commit(ctx, runID, now); err != nil {
		t.Fatal(err)
	}
	item := loadGalleryItemsForTest(t, db, created.ID)[0]
	var jobID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM processing_jobs WHERE item_uuid=?`, item.UUID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if err := db.ProcessingJobs().Cancel(ctx, jobID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	current, err := db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().SetItemExcluded(ctx, item.ID, true, current.MetadataRevision, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	current, err = db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().SetItemExcluded(ctx, item.ID, false, current.MetadataRevision, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM processing_jobs WHERE id=?`, jobID).Scan(&status); err != nil || status != "CANCELLED" {
		t.Fatalf("manually cancelled job status=%q err=%v", status, err)
	}
}

func TestGalleryItemForgetRequiresSafeStateAndTombstonesUUID(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 19, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	item, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "keep.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := db.Galleries().Find(ctx, created.ID)
	if err := db.Galleries().ForgetItem(ctx, item.ID, current.MetadataRevision, now); !errors.Is(err, ErrGalleryItemNotForgettable) {
		t.Fatalf("forget available Item error = %v", err)
	}
	if _, err := db.Galleries().SetItemExcluded(ctx, item.ID, true, current.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gallery_covers(gallery_id,preferred_kind,preferred_item_uuid,
		fallback_item_uuid,effective_kind,effective_item_uuid,warning_code,updated_at_utc)
		VALUES(?,'ITEM',?,?,'ITEM',?,'',?)`, created.ID, item.UUID, item.UUID, item.UUID, formatTime(now)); err != nil {
		t.Fatal(err)
	}
	current, _ = db.Galleries().Find(ctx, created.ID)
	if err := db.Galleries().ForgetItem(ctx, item.ID, current.MetadataRevision, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().FindItem(ctx, item.ID); !errors.Is(err, ErrGalleryItemNotFound) {
		t.Fatalf("finding forgotten Item error = %v", err)
	}
	record, err := db.UUIDRegistry().Lookup(ctx, item.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != PortableUUIDTombstone || record.Kind != portableid.KindGalleryItem {
		t.Fatalf("forgotten UUID record = %#v", record)
	}
	var preferredKind, effectiveKind string
	var coverReferences int
	if err := db.QueryRowContext(ctx, `SELECT preferred_kind,effective_kind,
		(preferred_item_uuid IS NOT NULL)+(fallback_item_uuid IS NOT NULL)+(effective_item_uuid IS NOT NULL)
		FROM gallery_covers WHERE gallery_id=?`, created.ID).Scan(&preferredKind, &effectiveKind, &coverReferences); err != nil ||
		preferredKind != "NONE" || effectiveKind != "NONE" || coverReferences != 0 {
		t.Fatalf("forgotten cover state=%s/%s/%d err=%v", preferredKind, effectiveKind, coverReferences, err)
	}
}

func TestForgetMissingGalleryItemsIsAtomicAndLeavesAvailableMembers(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 10, 11, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	var missingUUIDs []string
	for index, availability := range []gallery.AvailabilityState{gallery.AvailabilityMissing, gallery.AvailabilityAvailable, gallery.AvailabilityMissing} {
		item, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
			RelativePath: fmt.Sprintf("%d.jpg", index), MediaKind: gallery.MediaKindStaticImage,
			ImageCategory: gallery.ImageCategoryPhoto, Position: int64(index+1) * 1024,
			Availability: availability, ProcessingState: gallery.ProcessingReady,
		}, now)
		if err != nil {
			t.Fatal(err)
		}
		if availability == gallery.AvailabilityMissing {
			missingUUIDs = append(missingUUIDs, item.UUID)
		}
	}
	current, err := db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	forgotten, err := db.Galleries().ForgetMissingItems(ctx, created.ID, current.MetadataRevision, now.Add(time.Minute))
	if err != nil || forgotten != 2 {
		t.Fatalf("forgotten=%d err=%v", forgotten, err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	if len(items) != 1 || items[0].Availability != gallery.AvailabilityAvailable {
		t.Fatalf("remaining items=%#v", items)
	}
	for _, uuid := range missingUUIDs {
		record, err := db.UUIDRegistry().Lookup(ctx, uuid)
		if err != nil || record.State != PortableUUIDTombstone {
			t.Fatalf("missing UUID lifecycle=%#v err=%v", record, err)
		}
	}
	updated, err := db.Galleries().Find(ctx, created.ID)
	if err != nil || updated.MetadataRevision != current.MetadataRevision+1 || updated.ScanRevision != current.ScanRevision+1 {
		t.Fatalf("updated Gallery=%#v err=%v", updated, err)
	}
}

func TestReplaceMissingGalleryItemPreservesOldIdentityAndBusinessState(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	missing, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "old/portrait.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Caption: "retained caption",
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "new/portrait-hq.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 2048,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingPending,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET availability_state='MISSING',excluded=1,
		byte_size=100,quick_fingerprint='old-quick',full_fingerprint='old-full' WHERE id=?;
		UPDATE gallery_items SET byte_size=400,quick_fingerprint='new-quick',full_fingerprint='new-full' WHERE id=?;
		INSERT INTO gallery_item_personal_states(gallery_item_id,favorite,favorited_at_utc) VALUES(?,1,?)`,
		missing.ID, replacement.ID, missing.ID, formatTime(now)); err != nil {
		t.Fatal(err)
	}
	current, err := db.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Galleries().ReplaceMissingItem(ctx, missing.ID, replacement.ID, current.MetadataRevision, current.ScanRevision, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	adopted, err := db.Galleries().FindItem(ctx, missing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if adopted.UUID != missing.UUID || adopted.RelativePath != replacement.RelativePath || adopted.Caption != "retained caption" ||
		!adopted.Excluded || adopted.Position != missing.Position || adopted.Availability != gallery.AvailabilityAvailable ||
		adopted.ProcessingState != gallery.ProcessingPending || adopted.ByteSize != 400 || adopted.FullFingerprint != "new-full" ||
		adopted.ContentRevision != missing.ContentRevision+1 {
		t.Fatalf("adopted GalleryItem=%#v", adopted)
	}
	if _, err := db.Galleries().FindItem(ctx, replacement.ID); !errors.Is(err, ErrGalleryItemNotFound) {
		t.Fatalf("temporary replacement still exists: %v", err)
	}
	record, err := db.UUIDRegistry().Lookup(ctx, replacement.UUID)
	if err != nil || record.State != PortableUUIDTombstone {
		t.Fatalf("replacement UUID lifecycle=%#v err=%v", record, err)
	}
	var favorite int
	if err := db.QueryRowContext(ctx, `SELECT favorite FROM gallery_item_personal_states WHERE gallery_item_id=?`, missing.ID).Scan(&favorite); err != nil || favorite != 1 {
		t.Fatalf("retained personal state=%d err=%v", favorite, err)
	}
	updated, err := db.Galleries().Find(ctx, created.ID)
	if err != nil || updated.MetadataRevision != current.MetadataRevision+1 || updated.ScanRevision != current.ScanRevision+1 {
		t.Fatalf("updated Gallery=%#v err=%v", updated, err)
	}
}

func TestReplaceMissingGalleryItemRejectsEditedReplacement(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 10, 12, 30, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	missing, _ := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "old.jpg", MediaKind: gallery.MediaKindStaticImage, ImageCategory: gallery.ImageCategoryPhoto,
		Position: 1024, Availability: gallery.AvailabilityMissing, ProcessingState: gallery.ProcessingReady,
	}, now)
	replacement, _ := db.Galleries().AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "new.jpg", MediaKind: gallery.MediaKindStaticImage, ImageCategory: gallery.ImageCategoryPhoto,
		Position: 2048, Caption: "already reviewed", Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now)
	current, _ := db.Galleries().Find(ctx, created.ID)
	err := db.Galleries().ReplaceMissingItem(ctx, missing.ID, replacement.ID, current.MetadataRevision, current.ScanRevision, now)
	if !errors.Is(err, ErrGalleryItemReplacementInvalid) {
		t.Fatalf("edited replacement error=%v", err)
	}
	if _, err := db.Galleries().FindItem(ctx, replacement.ID); err != nil {
		t.Fatalf("rejected replacement was modified: %v", err)
	}
}

func TestGalleryItemMoveUsesGapThenRebalancesOnlyCurrentGroup(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 19, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	store := db.Galleries()
	photoA := addPositionedItem(t, store, created.ID, source.ID, "a.jpg", gallery.MediaKindStaticImage, 1, now)
	photoB := addPositionedItem(t, store, created.ID, source.ID, "b.jpg", gallery.MediaKindStaticImage, 2, now)
	photoC := addPositionedItem(t, store, created.ID, source.ID, "c.jpg", gallery.MediaKindStaticImage, 3, now)
	video := addPositionedItem(t, store, created.ID, source.ID, "clip.mp4", gallery.MediaKindVideo, 4, now)

	current, _ := store.Find(ctx, created.ID)
	if _, err := store.MoveItemWithinGroup(ctx, photoC.ID, &photoB.ID, current.MetadataRevision, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	after := loadGalleryItemsForTest(t, db, created.ID)
	if after[0].ID != photoA.ID || after[1].ID != photoC.ID || after[2].ID != photoB.ID {
		t.Fatalf("photo order after rebalance = %#v", after[:3])
	}
	persistedVideo, err := store.FindItem(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedVideo.Position != 4 {
		t.Fatalf("rebalance changed other media group position to %d", persistedVideo.Position)
	}
	if after[0].Position <= 4 || after[1].Position-after[0].Position != 1024 {
		t.Fatalf("rebalance did not move group above high-water mark: %#v", after[:3])
	}

	current, _ = store.Find(ctx, created.ID)
	if _, err := store.MoveItemWithinGroup(ctx, photoA.ID, &video.ID, current.MetadataRevision, now); !errors.Is(err, ErrMoveAcrossMediaGroups) {
		t.Fatalf("cross-group move error = %v", err)
	}
}

func TestGalleryItemGroupReorderIsAtomicCompleteAndKeepsOtherGroups(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 8, 11, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	store := db.Galleries()
	root := addPositionedItem(t, store, created.ID, source.ID, "root.jpg", gallery.MediaKindStaticImage, 1024, now)
	chapterA := addPositionedItem(t, store, created.ID, source.ID, "disc-a/1.jpg", gallery.MediaKindStaticImage, 2048, now)
	chapterB := addPositionedItem(t, store, created.ID, source.ID, "disc-b/1.jpg", gallery.MediaKindStaticImage, 3072, now)
	video := addPositionedItem(t, store, created.ID, source.ID, "disc-a/clip.mp4", gallery.MediaKindVideo, 4096, now)

	current, err := store.Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReorderItemsWithinGroup(ctx, []int64{root.ID, chapterB.ID, chapterA.ID}, current.MetadataRevision, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	if items[0].ID != root.ID || items[1].ID != chapterB.ID || items[2].ID != chapterA.ID {
		t.Fatalf("photo order = %#v", items[:3])
	}
	persistedVideo, err := store.FindItem(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedVideo.Position != 4096 {
		t.Fatalf("group reorder changed video position to %d", persistedVideo.Position)
	}
	after, err := store.Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.MetadataRevision != current.MetadataRevision+1 {
		t.Fatalf("metadata revision = %d, want %d", after.MetadataRevision, current.MetadataRevision+1)
	}
	if err := store.ReorderItemsWithinGroup(ctx, []int64{root.ID, chapterB.ID}, after.MetadataRevision, now.Add(2*time.Minute)); !errors.Is(err, ErrInvalidGalleryItemOrder) {
		t.Fatalf("incomplete reorder error = %v", err)
	}
	if err := store.ReorderItemsWithinGroup(ctx, []int64{root.ID, chapterB.ID, chapterB.ID}, after.MetadataRevision, now.Add(2*time.Minute)); !errors.Is(err, ErrInvalidGalleryItemOrder) {
		t.Fatalf("duplicate reorder error = %v", err)
	}
	if err := store.ReorderItemsWithinGroup(ctx, []int64{root.ID, chapterB.ID, video.ID}, after.MetadataRevision, now.Add(2*time.Minute)); !errors.Is(err, ErrInvalidGalleryItemOrder) {
		t.Fatalf("cross-group reorder error = %v", err)
	}
}

func addPositionedItem(
	t *testing.T,
	store *GalleryStore,
	galleryID int64,
	sourceID int64,
	relativePath string,
	kind gallery.MediaKind,
	position int64,
	now time.Time,
) gallery.Item {
	t.Helper()
	category := gallery.ImageCategory("")
	if kind == gallery.MediaKindStaticImage {
		category = gallery.ImageCategoryPhoto
	}
	item, err := store.AddItem(context.Background(), galleryID, sourceID, CreateItemInput{
		RelativePath: relativePath, MediaKind: kind, ImageCategory: category,
		Position: position, Availability: gallery.AvailabilityAvailable,
		ProcessingState: gallery.ProcessingReady,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return item
}
