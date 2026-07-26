package productdb

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestCoverPreferredIntentFallsBackRestoresAndSupportsUndo(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	galleryID, items := createDerivativeTestGallery(t, db, now)
	for index, item := range items[:2] {
		if _, err := db.Derivatives().Publish(ctx, PublishDerivativeInput{ItemUUID: item.UUID,
			Variant: mediaprocessing.VariantCard480, CacheTier: mediaprocessing.CacheBase,
			ContentRevision: 1, ProfileHash: "cover", CacheRelativePath: "cover/item-" + string(rune('a'+index)) + ".webp",
			MIMEType: "image/webp", ByteSize: 10}, now); err != nil {
			t.Fatal(err)
		}
	}
	store := &CoverStore{db: db.DB, random: bytes.NewReader(make([]byte, 128))}
	automatic, err := store.Initialize(ctx, galleryID, now)
	if err != nil {
		t.Fatal(err)
	}
	if automatic.PreferredKind != gallery.CoverAutoRandom || automatic.PreferredItemUUID != items[0].UUID || automatic.EffectiveItemUUID != items[0].UUID {
		t.Fatalf("automatic cover = %#v", automatic)
	}
	manual, err := store.SetItem(ctx, galleryID, items[1].UUID, 2, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if manual.PreferredKind != gallery.CoverItem || manual.EffectiveItemUUID != items[1].UUID || !manual.CanUndo {
		t.Fatalf("manual cover = %#v", manual)
	}
	pushed, err := db.Manifests().PushGallery(ctx, galleryID, 3, now.Add(90*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := manifest.ReadFile(pushed.Path, manifest.MaxGalleryBytes)
	if err != nil {
		t.Fatal(err)
	}
	document, err := manifest.ParseGallery(bytesReader(data))
	if err != nil || !document.Cover.Present || document.Cover.Value.Kind != "ITEM" || document.Cover.Value.ItemUUID != items[1].UUID {
		t.Fatalf("pushed preferred cover = %#v, %v", document.Cover, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET availability_state='MISSING' WHERE item_uuid=?`, items[1].UUID); err != nil {
		t.Fatal(err)
	}
	fallback, err := store.Reconcile(ctx, galleryID, false, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if fallback.PreferredItemUUID != items[1].UUID || fallback.EffectiveItemUUID != items[0].UUID || fallback.WarningCode != "PREFERRED_COVER_MISSING" {
		t.Fatalf("missing preferred fallback = %#v", fallback)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET availability_state='AVAILABLE' WHERE item_uuid=?`, items[1].UUID); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Reconcile(ctx, galleryID, false, now.Add(3*time.Minute))
	if err != nil || restored.EffectiveItemUUID != items[1].UUID || restored.WarningCode != "" {
		t.Fatalf("restored preferred cover = %#v, %v", restored, err)
	}
	undone, err := store.Undo(ctx, galleryID, false, 3, now.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if undone.PreferredKind != gallery.CoverAutoRandom || undone.PreferredItemUUID != items[0].UUID || undone.CanUndo {
		t.Fatalf("undone cover = %#v", undone)
	}
}

func TestPendingRandomStaticIntentCanUseVideoPosterAsEffectiveFallback(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 11, 0, 0, 0, time.UTC)
	galleryID, items := createDerivativeTestGallery(t, db, now)
	if _, err := db.Derivatives().Publish(ctx, PublishDerivativeInput{ItemUUID: items[3].UUID,
		Variant: mediaprocessing.VariantStaticPoster, CacheTier: mediaprocessing.CacheBase,
		ContentRevision: 1, ProfileHash: "poster", CacheRelativePath: "poster/video.webp",
		MIMEType: "image/webp", ByteSize: 10}, now); err != nil {
		t.Fatal(err)
	}
	state, err := db.Covers().Initialize(ctx, galleryID, now)
	if err != nil {
		t.Fatal(err)
	}
	isStaticPreferred := state.PreferredItemUUID == items[0].UUID || state.PreferredItemUUID == items[1].UUID || state.PreferredItemUUID == items[4].UUID
	if state.PreferredKind != gallery.CoverAutoRandom || !isStaticPreferred ||
		state.EffectiveKind != gallery.CoverVideoPoster || state.EffectiveItemUUID != items[3].UUID {
		t.Fatalf("video-only effective cover = %#v", state)
	}
}
