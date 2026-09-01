package productdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestManageGalleryItemResolutionNeverCrossesAggregateBoundary(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 1, 0, 0, 0, time.UTC)
	first, _ := createEmptySourceFixture(t, db, now)
	second, secondSource := createEmptySourceFixture(t, db, now.Add(time.Minute))
	item, err := db.Galleries().AddItem(ctx, second.ID, secondSource.ID, CreateItemInput{
		RelativePath: "member.jpg", MediaKind: gallery.MediaKindStaticImage,
		ContentFormat: gallery.ContentFormatImage, ImageCategory: gallery.ImageCategoryPhoto,
		Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Manage().GalleryItemID(ctx, first.SetID, item.UUID); !errors.Is(err, ErrGalleryItemNotFound) {
		t.Fatalf("cross-Gallery member resolution error = %v", err)
	}
	if _, err := db.Manage().GalleryItemID(ctx, second.SetID, item.UUID); err != nil {
		t.Fatalf("same-Gallery member resolution failed: %v", err)
	}
}

func TestManageCoreEntitiesIncludeStandaloneCoserAndOrderedAccounts(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Standalone"}, ProfileSummary: "Profile"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CoreEntities().AddSocialAccount(ctx, coser.UUID, "example", "Example", "name", "https://example.test/name", "INACTIVE", true, 2048, coser.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	detail, err := db.CoreEntities().ManageFind(ctx, "COSER", coser.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.SocialAccounts) != 1 || detail.SocialAccounts[0].Position != 2048 || detail.SocialAccounts[0].Status != "INACTIVE" {
		t.Fatalf("Manage Coser = %#v", detail)
	}
	page, err := db.CoreEntities().ManagePage(ctx, "COSER", 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalItems != 1 || len(page.Items) != 1 || page.Items[0].UUID != coser.UUID {
		t.Fatalf("standalone Coser page = %#v", page)
	}
}

func TestManageCoreEntityOptionsSearchStandaloneNamesAndAliases(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 3, 30, 0, 0, time.UTC)
	created, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{
		Name: "Standalone Search Target", Aliases: []string{"独立别名"},
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	byName, err := db.CoreEntities().ManageOptions(ctx, "COSER", "Search Target", 20)
	if err != nil || len(byName) != 1 || byName[0].UUID != created.UUID {
		t.Fatalf("Manage Coser name options = %#v, %v", byName, err)
	}
	byAlias, err := db.CoreEntities().ManageOptions(ctx, "COSER", "独立", 20)
	if err != nil || len(byAlias) != 1 || byAlias[0].UUID != created.UUID {
		t.Fatalf("Manage Coser alias options = %#v, %v", byAlias, err)
	}
	if _, err := db.CoreEntities().ManageOptions(ctx, "COSER", "", 51); err == nil {
		t.Fatal("Manage entity options accepted an unbounded limit")
	}
}

func TestManageCoserPageSearchAssetFiltersAndPageSize(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 1, 0, 30, 0, 0, time.UTC)
	complete, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alice Complete", Aliases: []string{"Alicia"}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	missingBanner, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alice Portrait"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Unrelated"}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE cosers SET avatar_path='avatar.webp',banner_path='banner.webp' WHERE uuid=?`, complete.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE cosers SET avatar_path='portrait.webp' WHERE uuid=?`, missingBanner.UUID); err != nil {
		t.Fatal(err)
	}
	byAlias, err := db.CoreEntities().ManagePageWithOptions(ctx, "COSER", ManageCoreEntityPageOptions{Page: 1, PageSize: 100, Query: "lici", CoserAssetFilter: "COMPLETE"})
	if err != nil || byAlias.PageSize != 100 || byAlias.TotalItems != 1 || len(byAlias.Items) != 1 || byAlias.Items[0].UUID != complete.UUID {
		t.Fatalf("filtered Coser Alias page = %#v, %v", byAlias, err)
	}
	incomplete, err := db.CoreEntities().ManagePageWithOptions(ctx, "COSER", ManageCoreEntityPageOptions{Page: 1, PageSize: 60, Query: "Alice", CoserAssetFilter: "INCOMPLETE"})
	if err != nil || incomplete.TotalItems != 1 || len(incomplete.Items) != 1 || incomplete.Items[0].UUID != missingBanner.UUID {
		t.Fatalf("incomplete Coser page = %#v, %v", incomplete, err)
	}
	if _, err := db.CoreEntities().ManagePageWithOptions(ctx, "COSER", ManageCoreEntityPageOptions{Page: 1, PageSize: 31}); err == nil {
		t.Fatal("Manage Coser page accepted unsupported page size")
	}
	if _, err := db.CoreEntities().ManagePageWithOptions(ctx, "WORK", ManageCoreEntityPageOptions{Page: 1, CoserAssetFilter: "MISSING_AVATAR"}); err == nil {
		t.Fatal("Manage Work page accepted a Coser asset filter")
	}
}

func TestManageCoserNameConflictsUseNormalizedExactNamesAndAliases(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)
	created, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{
		Name: "Straße", Aliases: []string{"Alice"},
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Strasse Extra"}}, now); err != nil {
		t.Fatal(err)
	}
	galleryValue, _ := createEmptySourceFixture(t, db, now)
	if err := db.Galleries().ReplaceRelations(ctx, galleryValue.ID, galleryValue.MetadataRevision,
		ReplaceGalleryRelationsInput{Credits: []ReplaceGalleryCreditInput{{CoserUUID: created.UUID, Position: 1024}}}, now); err != nil {
		t.Fatal(err)
	}
	byName, err := db.CoreEntities().ManageCoserNameConflicts(ctx, "  STRASSE  ", 10)
	if err != nil || len(byName) != 1 || byName[0].Coser.UUID != created.UUID || byName[0].GalleryCount != 1 || len(byName[0].MatchedValues) != 1 || byName[0].MatchedValues[0] != "Straße" {
		t.Fatalf("normalized Coser name conflicts = %#v, %v", byName, err)
	}
	byAlias, err := db.CoreEntities().ManageCoserNameConflicts(ctx, "ALICE", 10)
	if err != nil || len(byAlias) != 1 || byAlias[0].Coser.UUID != created.UUID || len(byAlias[0].MatchedValues) != 1 || byAlias[0].MatchedValues[0] != "Alice" {
		t.Fatalf("normalized Coser Alias conflicts = %#v, %v", byAlias, err)
	}
	partial, err := db.CoreEntities().ManageCoserNameConflicts(ctx, "Strass", 10)
	if err != nil || len(partial) != 0 {
		t.Fatalf("partial Coser name conflicts = %#v, %v", partial, err)
	}
	if _, err := db.CoreEntities().ManageCoserNameConflicts(ctx, "Alice", 21); err == nil {
		t.Fatal("Coser conflict check accepted an unbounded limit")
	}
}

func TestReplaceGalleryRelationsIsAtomicAndRevisionGuarded(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 4, 0, 0, 0, time.UTC)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Relations"}, now)
	if err != nil {
		t.Fatal(err)
	}
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Coser"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	work, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "Character"}, now)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := db.CoreEntities().CreateTag(ctx, CreateTagInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "Tag"}, UseInRecommendation: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	input := ReplaceGalleryRelationsInput{
		Credits: []ReplaceGalleryCreditInput{{CoserUUID: coser.UUID, Position: 1024, Cast: []ReplaceGalleryCastInput{{CharacterUUID: character.UUID, Position: 1024}}}},
		Tags:    []ReplaceGalleryTagInput{{TagUUID: tag.UUID, Position: 1024}},
	}
	if err := db.Galleries().ReplaceRelations(ctx, created.ID, created.MetadataRevision, input, now); err != nil {
		t.Fatal(err)
	}
	detail, err := db.Manage().GalleryDetail(ctx, created.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Row.MetadataRevision != created.MetadataRevision+1 || len(detail.Credits) != 1 || len(detail.Credits[0].Cast) != 1 || len(detail.Tags) != 1 {
		t.Fatalf("Manage Gallery relations = %#v", detail)
	}
	if detail.Credits[0].CoserUUID != coser.UUID || detail.Credits[0].Cast[0].CharacterUUID != character.UUID || detail.Tags[0].UUID != tag.UUID {
		t.Fatalf("Manage Gallery relation identities = %#v %#v", detail.Credits, detail.Tags)
	}
	if err := db.Galleries().ReplaceRelations(ctx, created.ID, created.MetadataRevision, ReplaceGalleryRelationsInput{}, now); !errors.Is(err, ErrMetadataRevisionConflict) {
		t.Fatalf("stale relation save error = %v", err)
	}
	detail, err = db.Manage().GalleryDetail(ctx, created.SetID)
	if err != nil || len(detail.Credits) != 1 || len(detail.Tags) != 1 {
		t.Fatalf("stale relation save changed data: %#v, %v", detail, err)
	}
}
