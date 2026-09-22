package productdb

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
)

func TestGalleryAggregateActivationAndAddedAt(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Galleries()
	now := time.Date(2026, 7, 22, 13, 0, 0, 0, time.UTC)

	created, err := store.Create(ctx, CreateGalleryInput{
		Title:         "Album One",
		ContentRating: gallery.ContentRatingNonAdult,
	}, now)
	if err != nil {
		t.Fatalf("creating Gallery: %v", err)
	}
	if created.State != gallery.StateDraft || created.AddedAtUTC != nil || created.SetID == "" {
		t.Fatalf("created Gallery = %#v", created)
	}
	setRecord, err := db.UUIDRegistry().Lookup(ctx, created.SetID)
	if err != nil || setRecord.Kind != portableid.KindGallery {
		t.Fatalf("Gallery set_id registry record = %#v, error = %v", setRecord, err)
	}

	_, err = store.SetState(ctx, created.ID, created.MetadataRevision, gallery.StateActive, now.Add(time.Minute))
	var activationErr *ActivationError
	if !errors.As(err, &activationErr) {
		t.Fatalf("empty Gallery activation error = %v, want ActivationError", err)
	}

	source, err := store.AddSource(ctx, created.ID, CreateSourceInput{
		Type:         gallery.SourceTypeDirectory,
		Path:         filepath.Join(t.TempDir(), "album-one"),
		Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatalf("adding source: %v", err)
	}
	if _, err := store.AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath:    "001.jpg",
		MediaKind:       gallery.MediaKindStaticImage,
		ImageCategory:   gallery.ImageCategoryPhoto,
		Position:        1024,
		Availability:    gallery.AvailabilityAvailable,
		ProcessingState: gallery.ProcessingReady,
	}, now); err != nil {
		t.Fatalf("adding item: %v", err)
	}

	coserUUID := registerTestPortableUUID(t, db, portableid.KindCoser, now)
	if _, err := store.AddCredit(ctx, created.ID, coserUUID, 1024, created.MetadataRevision, now); err != nil {
		t.Fatalf("adding Album credit: %v", err)
	}

	activeAt := now.Add(2 * time.Minute)
	active, err := store.SetState(ctx, created.ID, created.MetadataRevision+1, gallery.StateActive, activeAt)
	if err != nil {
		t.Fatalf("activating complete Gallery: %v", err)
	}
	if active.AddedAtUTC == nil || !active.AddedAtUTC.Equal(activeAt) || !active.Browsable {
		t.Fatalf("active Gallery = %#v", active)
	}

	archived, err := store.SetState(ctx, active.ID, active.MetadataRevision, gallery.StateArchived, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("archiving Gallery: %v", err)
	}
	restored, err := store.SetState(ctx, archived.ID, archived.MetadataRevision, gallery.StateActive, now.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("restoring Gallery: %v", err)
	}
	if restored.AddedAtUTC == nil || !restored.AddedAtUTC.Equal(activeAt) {
		t.Fatalf("added_at changed after restore: got %v, want %v", restored.AddedAtUTC, activeAt)
	}
}

func TestGalleryMetadataOptimisticLockAndActiveDemotion(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Galleries()
	now := time.Date(2026, 7, 22, 13, 0, 0, 0, time.UTC)

	created, source, _ := createCompleteAlbumFixture(t, db, now)
	active, err := store.SetState(ctx, created.ID, created.MetadataRevision, gallery.StateActive, now)
	if err != nil {
		t.Fatalf("activating Gallery: %v", err)
	}

	updated, err := store.UpdateMetadata(ctx, active.ID, active.MetadataRevision, UpdateGalleryMetadataInput{
		Title:         "",
		ContentRating: gallery.ContentRatingNonAdult,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("updating active Gallery: %v", err)
	}
	if updated.State != gallery.StateDraft {
		t.Fatalf("invalid active metadata left state %s, want DRAFT", updated.State)
	}
	if updated.AddedAtUTC == nil {
		t.Fatal("demotion must preserve first activation time")
	}

	_, err = store.UpdateMetadata(ctx, active.ID, active.MetadataRevision, UpdateGalleryMetadataInput{
		Title:         "stale update",
		ContentRating: gallery.ContentRatingNonAdult,
	}, now.Add(2*time.Minute))
	if !errors.Is(err, ErrMetadataRevisionConflict) {
		t.Fatalf("stale update error = %v, want ErrMetadataRevisionConflict", err)
	}

	before, err := store.Find(ctx, created.ID)
	if err != nil {
		t.Fatalf("reading Gallery before source health update: %v", err)
	}
	if err := store.SetSourceHealth(
		ctx,
		source.ID,
		gallery.AvailabilityMissing,
		gallery.ReconcileNeedsRescan,
		false,
		now.Add(3*time.Minute),
	); err != nil {
		t.Fatalf("updating source health: %v", err)
	}
	after, err := store.Find(ctx, created.ID)
	if err != nil {
		t.Fatalf("reading Gallery after source health update: %v", err)
	}
	if after.MetadataRevision != before.MetadataRevision || after.ScanRevision != before.ScanRevision+1 {
		t.Fatalf("revisions after technical update = metadata %d scan %d; before %d/%d",
			after.MetadataRevision, after.ScanRevision, before.MetadataRevision, before.ScanRevision)
	}
}

func TestGallerySourceHealthPausesBrowseWithoutChangingItems(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Galleries()
	now := time.Date(2026, 7, 22, 13, 0, 0, 0, time.UTC)
	created, source, item := createCompleteAlbumFixture(t, db, now)
	active, err := store.SetState(ctx, created.ID, created.MetadataRevision, gallery.StateActive, now)
	if err != nil {
		t.Fatalf("activating Gallery: %v", err)
	}
	if !active.Browsable {
		t.Fatal("fixture Gallery should be browsable")
	}

	if err := store.SetSourceHealth(
		ctx,
		source.ID,
		gallery.AvailabilityUnreadable,
		gallery.ReconcileError,
		false,
		now.Add(time.Minute),
	); err != nil {
		t.Fatalf("marking source unreadable: %v", err)
	}
	paused, err := store.Find(ctx, created.ID)
	if err != nil {
		t.Fatalf("reading paused Gallery: %v", err)
	}
	if paused.State != gallery.StateActive || paused.Browsable {
		t.Fatalf("paused Gallery = %#v", paused)
	}
	persistedItem, err := findItem(ctx, db.DB, item.ID)
	if err != nil {
		t.Fatalf("reading Item: %v", err)
	}
	if persistedItem.Availability != gallery.AvailabilityAvailable {
		t.Fatalf("source failure changed Item availability to %s", persistedItem.Availability)
	}
}

func TestItemsRemainIndependentAcrossGallerySources(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Galleries()
	now := time.Date(2026, 7, 22, 13, 0, 0, 0, time.UTC)

	first, err := store.Create(ctx, CreateGalleryInput{Title: "A"}, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(ctx, CreateGalleryInput{Title: "B"}, now)
	if err != nil {
		t.Fatal(err)
	}
	firstSource, err := store.AddSource(ctx, first.ID, CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: filepath.Join(t.TempDir(), "a"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	secondSource, err := store.AddSource(ctx, second.ID, CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: filepath.Join(t.TempDir(), "b"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	input := CreateItemInput{
		RelativePath: "same.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}
	firstItem, err := store.AddItem(ctx, first.ID, firstSource.ID, input, now)
	if err != nil {
		t.Fatal(err)
	}
	secondItem, err := store.AddItem(ctx, second.ID, secondSource.ID, input, now)
	if err != nil {
		t.Fatal(err)
	}
	if firstItem.UUID == secondItem.UUID {
		t.Fatal("different physical files in different Galleries shared an item UUID")
	}
	if _, err := store.AddItem(ctx, first.ID, secondSource.ID, CreateItemInput{
		RelativePath: "wrong-owner.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 2048,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now); err == nil {
		t.Fatal("Item bound to a Source owned by another Gallery")
	}
}

func TestCosplayActivationRequiresCastForEveryCredit(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Galleries()
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	created, _, _ := createCompleteAlbumFixture(t, db, now)

	secondCoser := registerTestPortableUUID(t, db, portableid.KindCoser, now)
	if _, err := store.AddCredit(
		ctx,
		created.ID,
		secondCoser,
		2048,
		created.MetadataRevision,
		now,
	); err != nil {
		t.Fatalf("adding second Credit: %v", err)
	}
	characterA := registerTestPortableUUID(t, db, portableid.KindCharacter, now)
	characterB := registerTestPortableUUID(t, db, portableid.KindCharacter, now)

	var firstCreditID, secondCreditID int64
	rows, err := db.QueryContext(ctx, `
		SELECT id FROM gallery_credits WHERE gallery_id = ? ORDER BY position
	`, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() || rows.Scan(&firstCreditID) != nil || !rows.Next() || rows.Scan(&secondCreditID) != nil {
		rows.Close()
		t.Fatal("reading fixture Credits")
	}
	rows.Close()

	if err := store.AddCast(
		ctx,
		created.ID,
		firstCreditID,
		characterA,
		1024,
		created.MetadataRevision+1,
		now,
	); err != nil {
		t.Fatalf("adding first Cast: %v", err)
	}
	_, err = store.SetState(
		ctx,
		created.ID,
		created.MetadataRevision+2,
		gallery.StateActive,
		now,
	)
	var activationErr *ActivationError
	if !errors.As(err, &activationErr) || !hasActivationBlocker(activationErr, "CAST_REQUIRED_FOR_EACH_CREDIT") {
		t.Fatalf("partial Cast activation error = %#v, want per-Credit blocker", err)
	}

	if err := store.AddCast(
		ctx,
		created.ID,
		secondCreditID,
		characterB,
		1024,
		created.MetadataRevision+2,
		now,
	); err != nil {
		t.Fatalf("adding second Cast: %v", err)
	}
	if _, err := store.SetState(
		ctx,
		created.ID,
		created.MetadataRevision+3,
		gallery.StateActive,
		now,
	); err != nil {
		t.Fatalf("activating complete multi-Coser Cosplay: %v", err)
	}
}

func TestGalleryItemHardLimitAndExcludedRetention(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Galleries()
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	created, err := store.Create(ctx, CreateGalleryInput{Title: "Large"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.AddSource(ctx, created.ID, CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: filepath.Join(t.TempDir(), "large"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for index := 1; index <= 1000; index++ {
		itemUUID := fmt.Sprintf("00000000-0000-4000-8000-%012x", index)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO portable_uuid_registry (uuid, entity_kind, created_at_utc)
			VALUES (?, 'GALLERY_ITEM', ?)
		`, itemUUID, formatTime(now)); err != nil {
			t.Fatalf("registering Item %d: %v", index, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_items (
				item_uuid, gallery_id, source_id, relative_path, media_kind,
				image_category, position, availability_state, processing_state,
				created_at_utc, updated_at_utc
			) VALUES (?, ?, ?, ?, 'STATIC_IMAGE', 'PHOTO', ?, 'AVAILABLE', 'READY', ?, ?)
		`, itemUUID, created.ID, source.ID, fmt.Sprintf("%04d.jpg", index), int64(index)*1024, formatTime(now), formatTime(now)); err != nil {
			t.Fatalf("inserting Item %d: %v", index, err)
		}
	}
	excludedUUID := "00000000-0000-4000-8000-000000001001"
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO portable_uuid_registry (uuid, entity_kind, created_at_utc)
		VALUES (?, 'GALLERY_ITEM', ?)
	`, excludedUUID, formatTime(now)); err != nil {
		t.Fatal(err)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_items (
			item_uuid, gallery_id, source_id, relative_path, media_kind,
			image_category, position, excluded, availability_state,
			processing_state, created_at_utc, updated_at_utc
		) VALUES (?, ?, ?, 'excluded.jpg', 'STATIC_IMAGE', 'PHOTO', ?, 1,
			'AVAILABLE', 'READY', ?, ?)
	`, excludedUUID, created.ID, source.ID, int64(1001)*1024, formatTime(now), formatTime(now))
	if err != nil {
		t.Fatalf("retaining excluded Item beyond active limit: %v", err)
	}
	excludedID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gallery_items SET excluded = 0 WHERE id = ?`, excludedID); err == nil {
		t.Fatal("restoring the 1001st non-excluded Item unexpectedly succeeded")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func hasActivationBlocker(err *ActivationError, code string) bool {
	for _, blocker := range err.Blockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}

func createCompleteAlbumFixture(
	t *testing.T,
	db *Database,
	now time.Time,
) (gallery.Gallery, gallery.Source, gallery.Item) {
	t.Helper()
	ctx := context.Background()
	store := db.Galleries()
	created, err := store.Create(ctx, CreateGalleryInput{
		Title: "Complete Album", ContentRating: gallery.ContentRatingNonAdult,
	}, now)
	if err != nil {
		t.Fatalf("creating fixture Gallery: %v", err)
	}
	source, err := store.AddSource(ctx, created.ID, CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: filepath.Join(t.TempDir(), "fixture"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatalf("creating fixture Source: %v", err)
	}
	item, err := store.AddItem(ctx, created.ID, source.ID, CreateItemInput{
		RelativePath: "one.jpg", MediaKind: gallery.MediaKindStaticImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024,
		Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now)
	if err != nil {
		t.Fatalf("creating fixture Item: %v", err)
	}
	coserUUID := registerTestPortableUUID(t, db, portableid.KindCoser, now)
	if _, err := store.AddCredit(ctx, created.ID, coserUUID, 1024, created.MetadataRevision, now); err != nil {
		t.Fatalf("creating fixture Credit: %v", err)
	}
	created, err = store.Find(ctx, created.ID)
	if err != nil {
		t.Fatalf("reading fixture Gallery: %v", err)
	}
	return created, source, item
}

func registerTestPortableUUID(t *testing.T, db *Database, kind portableid.Kind, now time.Time) string {
	t.Helper()
	store := db.CoreEntities()
	switch kind {
	case portableid.KindCoser:
		created, err := store.CreateCoser(context.Background(), CreateCoserInput{
			CreateNamedEntityInput: CreateNamedEntityInput{Name: "Test Coser " + portableid.New()[:8]},
		}, now)
		if err != nil {
			t.Fatalf("creating Coser fixture: %v", err)
		}
		return created.UUID
	case portableid.KindCharacter:
		work, err := store.CreateWork(context.Background(), CreateNamedEntityInput{Name: "Test Work " + portableid.New()[:8]}, now)
		if err != nil {
			t.Fatalf("creating Work fixture: %v", err)
		}
		created, err := store.CreateCharacter(context.Background(), work.UUID, CreateNamedEntityInput{
			Name: "Test Character " + portableid.New()[:8],
		}, now)
		if err != nil {
			t.Fatalf("creating Character fixture: %v", err)
		}
		return created.UUID
	default:
		record, err := db.UUIDRegistry().New(context.Background(), kind, now)
		if err != nil {
			t.Fatalf("registering %s UUID: %v", kind, err)
		}
		return record.UUID
	}
}
