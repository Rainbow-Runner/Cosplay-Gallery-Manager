package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/coversimilarity"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func insertCoverSignatureForTest(t *testing.T, db *Database, uuid string, revision int64, sig coversimilarity.Signature, now time.Time) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `INSERT INTO cover_image_signatures
		(item_uuid,content_revision,profile_hash,signature_version,dhash_h,dhash_v,phash,width,height,created_at_utc)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid, revision, "test-profile", coversimilarity.Version,
		coversimilarity.Hex(sig.DHashH), coversimilarity.Hex(sig.DHashV), coversimilarity.Hex(sig.PHash),
		sig.Width, sig.Height, formatTime(now))
	if err != nil {
		t.Fatal(err)
	}
}

func TestCoverSimilarityAcceptsSeveralNearDuplicatesWithoutRunnerUpGap(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	first, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Stage(ctx, first, scanPhoto("old.jpg", "old")); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Commit(ctx, first, now); err != nil {
		t.Fatal(err)
	}
	old := loadGalleryItemsForTest(t, db, created.ID)[0]
	if _, err := db.Covers().Initialize(ctx, created.ID, now); err != nil {
		t.Fatal(err)
	}
	base := coversimilarity.Signature{DHashH: 0x111, DHashV: 0x222, PHash: 0x333, Width: 480, Height: 320}
	insertCoverSignatureForTest(t, db, old.UUID, old.ContentRevision, base, now)
	second, err := db.Scans().Begin(ctx, source.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, observation := range []ScanObservation{scanPhoto("a.jpg", "a"), scanPhoto("b.jpg", "b"), scanPhoto("c.jpg", "c")} {
		if err := db.Scans().Stage(ctx, second, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().Commit(ctx, second, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	for _, item := range items {
		if item.RelativePath == "old.jpg" {
			continue
		}
		sig := base
		switch item.RelativePath {
		case "a.jpg":
			sig.PHash ^= 1
		case "b.jpg":
			sig.PHash ^= 2
		case "c.jpg":
			sig.PHash = ^base.PHash
		}
		path, err := (mediaprocessing.CacheWriter{}).RelativePath(item.UUID, item.ContentRevision,
			mediaprocessing.VariantCard480, "test-profile", "jpg")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Derivatives().Publish(ctx, PublishDerivativeInput{ItemUUID: item.UUID,
			Variant: mediaprocessing.VariantCard480, CacheTier: mediaprocessing.CacheBase,
			ContentRevision: item.ContentRevision, ProfileHash: "test-profile", CacheRelativePath: path,
			MIMEType: "image/jpeg", Width: 480, Height: 320}, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err := db.CoverSimilarity().SaveSignature(ctx, CoverSignatureArtifact{ItemUUID: item.UUID,
			ContentRevision: item.ContentRevision, ProfileHash: "test-profile", CacheRelativePath: path}, sig, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := db.CoverSimilarity().ResolveOne(ctx, now.Add(2*time.Minute))
	if err != nil || !resolved {
		t.Fatalf("resolve=%v err=%v", resolved, err)
	}
	var status string
	var qualified int
	var selected string
	if err := db.QueryRowContext(ctx, `SELECT status,qualified_count,selected_item_uuid FROM cover_similarity_intents WHERE gallery_id=?`, created.ID).
		Scan(&status, &qualified, &selected); err != nil {
		t.Fatal(err)
	}
	if status != "MULTIPLE_MATCHED" || qualified != 2 {
		t.Fatalf("status=%s qualified=%d selected=%s", status, qualified, selected)
	}
	cover, err := db.Covers().Find(ctx, created.ID)
	if err != nil || cover.PreferredKind != gallery.CoverItem || cover.PreferredItemUUID != selected {
		t.Fatalf("cover=%+v err=%v", cover, err)
	}
}

func TestCoverSimilarityDoesNotOverwriteManualCover(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 30, 13, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	first, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Stage(ctx, first, scanPhoto("old.jpg", "old")); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Commit(ctx, first, now); err != nil {
		t.Fatal(err)
	}
	old := loadGalleryItemsForTest(t, db, created.ID)[0]
	if _, err := db.Covers().Initialize(ctx, created.ID, now); err != nil {
		t.Fatal(err)
	}
	insertCoverSignatureForTest(t, db, old.UUID, old.ContentRevision,
		coversimilarity.Signature{Width: 480, Height: 320}, now)
	second, err := db.Scans().Begin(ctx, source.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Stage(ctx, second, scanPhoto("new.jpg", "new")); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Commit(ctx, second, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var newUUID string
	if err := db.QueryRowContext(ctx, `SELECT item_uuid FROM gallery_items WHERE gallery_id=? AND relative_path='new.jpg'`, created.ID).Scan(&newUUID); err != nil {
		t.Fatal(err)
	}
	var metadataRevision int64
	if err := db.QueryRowContext(ctx, `SELECT metadata_revision FROM galleries WHERE id=?`, created.ID).Scan(&metadataRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Covers().SetItem(ctx, created.ID, newUUID, metadataRevision, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CoverSimilarity().ResolveOne(ctx, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM cover_similarity_intents WHERE scan_run_id=?`, second).Scan(&status); err != nil {
		t.Fatal(err)
	}
	cover, err := db.Covers().Find(ctx, created.ID)
	if err != nil || status != "STALE" || cover.PreferredItemUUID != newUUID || cover.PreferredKind != gallery.CoverItem {
		t.Fatalf("status=%s cover=%+v err=%v", status, cover, err)
	}
}
