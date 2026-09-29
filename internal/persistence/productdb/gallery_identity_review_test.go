package productdb

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestManualRelationsConfirmOnlyUniqueMatchingIdentitySuggestions(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 30, 1, 0, 0, 0, time.UTC)
	value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Manual review"}, now)
	if err != nil {
		t.Fatal(err)
	}
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alice", Aliases: []string{"Alicia"}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	addAutomationIdentitySuggestion(t, db, value.ID, "COSER", "Alicia", now)
	addAutomationIdentitySuggestion(t, db, value.ID, "COSER", "Unknown", now)
	input := ReplaceGalleryRelationsInput{Credits: []ReplaceGalleryCreditInput{{CoserUUID: coser.UUID, Position: 1024}}}
	if err := db.Galleries().ReplaceRelations(ctx, value.ID, value.MetadataRevision, input, now); err != nil {
		t.Fatal(err)
	}
	detail, err := db.Manage().GalleryDetail(ctx, value.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Review.IdentitySuggestions) != 2 || len(detail.Review.Blockers) == 0 {
		t.Fatalf("review = %#v", detail.Review)
	}
	statuses := map[string]string{}
	for _, item := range detail.Review.IdentitySuggestions {
		statuses[item.Value] = item.Status
	}
	if statuses["Alicia"] != "ACCEPTED" || statuses["Unknown"] != "PENDING" {
		t.Fatalf("statuses = %#v", statuses)
	}
	if err := db.Galleries().ReplaceRelations(ctx, value.ID, value.MetadataRevision, input, now); !errors.Is(err, ErrMetadataRevisionConflict) {
		t.Fatalf("stale save = %v", err)
	}
}

func TestAmbiguousIdentityNeedsExplicitLinkedChoiceAndKeepsHistory(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 30, 1, 0, 0, 0, time.UTC)
	value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Ambiguous"}, now)
	if err != nil {
		t.Fatal(err)
	}
	a, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alice", Aliases: []string{"Same"}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Bob", Aliases: []string{"Same"}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	addAutomationIdentitySuggestion(t, db, value.ID, "COSER", "Same", now)
	addAutomationIdentitySuggestion(t, db, value.ID, "COSER", "Wrong", now)
	input := ReplaceGalleryRelationsInput{Credits: []ReplaceGalleryCreditInput{{CoserUUID: a.UUID, Position: 1024}}}
	if err := db.Galleries().ReplaceRelations(ctx, value.ID, value.MetadataRevision, input, now); err != nil {
		t.Fatal(err)
	}
	updated, err := db.Galleries().Find(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := db.Manage().GalleryDetail(ctx, value.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Review.IdentitySuggestions) != 2 || detail.Review.IdentitySuggestions[0].Status != "PENDING" {
		t.Fatalf("review = %#v", detail.Review)
	}
	var ambiguousID, wrongID int64
	for _, item := range detail.Review.IdentitySuggestions {
		if item.Value == "Same" {
			if len(item.Options) != 1 || item.Options[0].UUID != a.UUID {
				t.Fatalf("options = %#v", item.Options)
			}
			_, _ = fmt.Sscan(item.ID, &ambiguousID)
		} else {
			_, _ = fmt.Sscan(item.ID, &wrongID)
		}
	}
	if err := db.Galleries().ResolveIdentitySuggestion(ctx, value.ID, ambiguousID, value.MetadataRevision, true, a.UUID, now); !errors.Is(err, ErrMetadataRevisionConflict) {
		t.Fatalf("stale review = %v", err)
	}
	if err := db.Galleries().ResolveIdentitySuggestion(ctx, value.ID, ambiguousID, updated.MetadataRevision, true, b.UUID, now); !errors.Is(err, ErrIdentitySuggestionNotLinked) {
		t.Fatalf("unlinked = %v", err)
	}
	other, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Other Gallery"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Galleries().ResolveIdentitySuggestion(ctx, other.ID, ambiguousID, other.MetadataRevision, false, "", now); !errors.Is(err, ErrIdentitySuggestionNotPending) {
		t.Fatalf("cross-gallery review = %v", err)
	}
	if err := db.Galleries().ResolveIdentitySuggestion(ctx, value.ID, ambiguousID, updated.MetadataRevision, true, a.UUID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Galleries().ResolveIdentitySuggestion(ctx, value.ID, ambiguousID, updated.MetadataRevision, true, a.UUID, now); !errors.Is(err, ErrIdentitySuggestionNotPending) {
		t.Fatalf("replayed review = %v", err)
	}
	if err := db.Galleries().ResolveIdentitySuggestion(ctx, value.ID, wrongID, updated.MetadataRevision, false, "", now); err != nil {
		t.Fatal(err)
	}
	detail, err = db.Manage().GalleryDetail(ctx, value.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Review.IdentitySuggestions) != 2 || len(detail.Review.Blockers) == 0 || detail.Review.IdentitySuggestions[0].Status != "REJECTED" {
		t.Fatalf("resolved review = %#v", detail.Review)
	}
	for _, code := range detail.Review.Blockers {
		if code == "IDENTITY_SUGGESTION_UNRESOLVED" {
			t.Fatal("resolved suggestion still blocks activation")
		}
	}
	if detail.Row.MetadataRevision != updated.MetadataRevision || len(detail.Credits) != 1 || detail.Credits[0].CoserUUID != a.UUID {
		t.Fatalf("review changed relations/revision: %#v", detail)
	}
}

func TestManualReviewCompletesBlockedDraftActivationWithoutRemovingHistory(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 30, 1, 0, 0, 0, time.UTC)
	value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Ready after review", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, value.ID, CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: filepath.Join(t.TempDir(), "album"), Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddItem(ctx, value.ID, source.ID, CreateItemInput{RelativePath: "001.jpg", MediaKind: gallery.MediaKindStaticImage, ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady}, now); err != nil {
		t.Fatal(err)
	}
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alice"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	addAutomationIdentitySuggestion(t, db, value.ID, "COSER", "Alice", now)
	current, err := db.Galleries().Find(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().SetState(ctx, value.ID, current.MetadataRevision, gallery.StateActive, now); err == nil {
		t.Fatal("activation bypassed pending suggestion and missing credit")
	}
	if err := db.Galleries().ReplaceRelations(ctx, value.ID, current.MetadataRevision, ReplaceGalleryRelationsInput{Credits: []ReplaceGalleryCreditInput{{CoserUUID: coser.UUID, Position: 1024}}}, now); err != nil {
		t.Fatal(err)
	}
	current, err = db.Galleries().Find(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := db.Galleries().SetState(ctx, value.ID, current.MetadataRevision, gallery.StateActive, now.Add(time.Minute))
	if err != nil || active.State != gallery.StateActive {
		t.Fatalf("activation = %#v, %v", active, err)
	}
	var pending, accepted int
	if err := db.QueryRowContext(ctx, `SELECT SUM(status='PENDING'),SUM(status='ACCEPTED') FROM gallery_identity_suggestions WHERE gallery_id=?`, value.ID).Scan(&pending, &accepted); err != nil || pending != 0 || accepted != 1 {
		t.Fatalf("suggestion history = pending %d accepted %d: %v", pending, accepted, err)
	}
}

func TestManualIdentityReconciliationRollsBackRelationOnSuggestionWriteFailure(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 30, 1, 0, 0, 0, time.UTC)
	value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Rollback"}, now)
	if err != nil {
		t.Fatal(err)
	}
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alice"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	addAutomationIdentitySuggestion(t, db, value.ID, "COSER", "Alice", now)
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_review BEFORE UPDATE ON gallery_identity_suggestions BEGIN SELECT RAISE(ABORT,'injected review failure'); END`); err != nil {
		t.Fatal(err)
	}
	input := ReplaceGalleryRelationsInput{Credits: []ReplaceGalleryCreditInput{{CoserUUID: coser.UUID, Position: 1024}}}
	if err := db.Galleries().ReplaceRelations(ctx, value.ID, value.MetadataRevision, input, now); err == nil {
		t.Fatal("expected review update failure")
	}
	detail, err := db.Manage().GalleryDetail(ctx, value.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Row.MetadataRevision != value.MetadataRevision || len(detail.Credits) != 0 || detail.Review.IdentitySuggestions[0].Status != "PENDING" {
		t.Fatalf("partial save: %#v", detail)
	}
}
