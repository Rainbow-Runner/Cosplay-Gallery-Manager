package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
)

func TestStrongAndTagRecommendationsUseConfirmedWeightsAndScope(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 20, 0, 0, 0, time.UTC)
	coserA, _ := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "A"}}, now)
	coserB, _ := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "B"}}, now)
	coserC, _ := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "C"}}, now)
	workA, _ := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Work A"}, now)
	workB, _ := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Work B"}, now)
	characterA, _ := db.CoreEntities().CreateCharacter(ctx, workA.UUID, CreateNamedEntityInput{Name: "Character A"}, now)
	characterB, _ := db.CoreEntities().CreateCharacter(ctx, workA.UUID, CreateNamedEntityInput{Name: "Character B"}, now)
	characterC, _ := db.CoreEntities().CreateCharacter(ctx, workB.UUID, CreateNamedEntityInput{Name: "Character C"}, now)

	source, _ := createBrowseGallery(t, db, "Source", gallery.ContentRatingNonAdult, now)
	sameCharacter, _ := createBrowseGallery(t, db, "Same Character", gallery.ContentRatingNonAdult, now)
	sameWork, _ := createBrowseGallery(t, db, "Same Work", gallery.ContentRatingNonAdult, now)
	typeOnly, _ := createBrowseGallery(t, db, "Type Only", gallery.ContentRatingNonAdult, now)
	adult, _ := createBrowseGallery(t, db, "Adult Match", gallery.ContentRatingAdult, now)
	attachRecommendationCast(t, db, source, coserA.UUID, characterA.UUID, now)
	attachRecommendationCast(t, db, sameCharacter, coserA.UUID, characterA.UUID, now)
	attachRecommendationCast(t, db, sameWork, coserB.UUID, characterB.UUID, now)
	attachRecommendationCast(t, db, typeOnly, coserC.UUID, characterC.UUID, now)
	attachRecommendationCast(t, db, adult, coserA.UUID, characterA.UUID, now)
	for index, value := range []gallery.Gallery{source, sameCharacter, sameWork, typeOnly, adult} {
		activateBrowseFixture(t, db, value.ID, now.Add(time.Duration(index)*time.Minute))
	}

	strong, err := db.Browse().StrongRecommendations(ctx, source.SetID, browse.ScopeList)
	if err != nil {
		t.Fatal(err)
	}
	if len(strong) != 2 || strong[0].Card.SetID != sameCharacter.SetID || strong[0].Score != 195 ||
		strong[1].Card.SetID != sameWork.SetID || strong[1].Score != 55 {
		t.Fatalf("strong recommendations = %#v", strong)
	}
	if len(strong[0].Reasons) != 4 || len(strong[1].Reasons) != 2 {
		t.Fatalf("strong reasons = %#v", strong)
	}

	parent, _ := db.CoreEntities().CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Parent"}, UseInRecommendation: true}, now)
	childA, _ := db.CoreEntities().CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Child A"}, UseInRecommendation: true}, now)
	childB, _ := db.CoreEntities().CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Child B"}, UseInRecommendation: true}, now)
	if err := db.CoreEntities().AddTagParent(ctx, childA.UUID, parent.UUID, 1024, childA.MetadataRevision, parent.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	if err := db.CoreEntities().AddTagParent(ctx, childB.UUID, parent.UUID, 1024, childB.MetadataRevision, parent.MetadataRevision+1, now); err != nil {
		t.Fatal(err)
	}
	currentSource, _ := db.Galleries().Find(ctx, source.ID)
	if err := db.Galleries().AddTag(ctx, source.ID, childA.UUID, 1024, currentSource.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	currentWork, _ := db.Galleries().Find(ctx, sameWork.ID)
	if err := db.Galleries().AddTag(ctx, sameWork.ID, childB.UUID, 1024, currentWork.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	tagMatches, err := db.Browse().TagRecommendations(ctx, source.SetID, browse.ScopeList)
	if err != nil {
		t.Fatal(err)
	}
	if len(tagMatches) != 1 || tagMatches[0].Card.SetID != sameWork.SetID || tagMatches[0].Score < 0.05 ||
		len(tagMatches[0].Reasons) != 1 || tagMatches[0].Reasons[0] != browse.ReasonTags {
		t.Fatalf("Tag recommendations = %#v", tagMatches)
	}
}

func attachRecommendationCast(t *testing.T, db *Database, target gallery.Gallery, coserUUID, characterUUID string, now time.Time) {
	t.Helper()
	credit, err := db.Galleries().AddCredit(context.Background(), target.ID, coserUUID, 1024, target.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Galleries().AddCast(context.Background(), target.ID, credit, characterUUID, 1024, target.MetadataRevision+1, now); err != nil {
		t.Fatal(err)
	}
}
