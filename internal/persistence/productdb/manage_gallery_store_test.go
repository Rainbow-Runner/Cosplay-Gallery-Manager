package productdb

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
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

func TestManageGalleryPageSearchAcrossDatabaseAndIssueFilters(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	first, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Alice COS", Aliases: []string{"Moonlight"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	firstSource, err := db.Galleries().AddSource(ctx, first.ID, CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: filepath.Join(t.TempDir(), "Special Source"), Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "100% Hero"}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddSource(ctx, second.ID, CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: filepath.Join(t.TempDir(), "other"), Availability: gallery.AvailabilityAvailable}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddItem(ctx, first.ID, firstSource.ID, CreateItemInput{RelativePath: "missing.jpg", MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage, ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityMissing, ProcessingState: gallery.ProcessingReady}, now); err != nil {
		t.Fatal(err)
	}
	for _, search := range []string{"Alice", "moon", "Special Source", first.SetID[:8]} {
		page, err := db.Manage().GalleryPage(ctx, 1, "ALL", search)
		if err != nil || page.TotalItems != 1 || len(page.Items) != 1 || page.Items[0].SetID != first.SetID || page.Summary.All != 2 {
			t.Fatalf("search %q: page=%#v err=%v", search, page, err)
		}
	}
	page, err := db.Manage().GalleryPage(ctx, 1, "MISSING", "Moon")
	if err != nil || page.TotalItems != 1 || page.Items[0].SetID != first.SetID {
		t.Fatalf("combined filter: %#v %v", page, err)
	}
	page, err = db.Manage().GalleryPage(ctx, 1, "MISSING", "Hero")
	if err != nil || page.TotalItems != 0 || len(page.Items) != 0 || page.Summary.All != 2 {
		t.Fatalf("empty combined filter: %#v %v", page, err)
	}
	page, err = db.Manage().GalleryPage(ctx, 1, "ALL", "%")
	if err != nil || page.TotalItems != 1 || page.Items[0].SetID != second.SetID {
		t.Fatalf("literal wildcard: %#v %v", page, err)
	}
	if _, err := db.Manage().GalleryPage(ctx, 1, "ALL", strings.Repeat("x", 301)); err == nil {
		t.Fatal("long query was accepted")
	}
}

func TestManageGalleryPageFiltersMissingMembersAcrossTheWholeDatabase(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	missingGallery, missingSource := createEmptySourceFixture(t, db, now)
	if _, err := db.Galleries().AddItem(ctx, missingGallery.ID, missingSource.ID, CreateItemInput{
		RelativePath: "old-name.jpg", MediaKind: gallery.MediaKindStaticImage,
		ContentFormat: gallery.ContentFormatImage, ImageCategory: gallery.ImageCategoryPhoto,
		Position: 1024, Availability: gallery.AvailabilityMissing, ProcessingState: gallery.ProcessingReady,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddItem(ctx, missingGallery.ID, missingSource.ID, CreateItemInput{
		RelativePath: "other-missing.jpg", MediaKind: gallery.MediaKindStaticImage,
		ContentFormat: gallery.ContentFormatImage, ImageCategory: gallery.ImageCategoryPhoto,
		Position: 2048, Availability: gallery.AvailabilityMissing, ProcessingState: gallery.ProcessingReady,
	}, now); err != nil {
		t.Fatal(err)
	}
	errorGallery, errorSource := createEmptySourceFixture(t, db, now.Add(time.Minute))
	if _, err := db.Galleries().AddItem(ctx, errorGallery.ID, errorSource.ID, CreateItemInput{
		RelativePath: "failed.jpg", MediaKind: gallery.MediaKindStaticImage,
		ContentFormat: gallery.ContentFormatImage, ImageCategory: gallery.ImageCategoryPhoto,
		Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingError,
	}, now); err != nil {
		t.Fatal(err)
	}
	page, err := db.Manage().GalleryPage(ctx, 1, "MISSING", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalItems != 1 || len(page.Items) != 1 || page.Items[0].SetID != missingGallery.SetID || page.Items[0].MissingCount != 2 {
		t.Fatalf("missing Gallery page = %#v", page)
	}
	if page.Summary.All != 2 || page.Summary.MissingGallery != 1 || page.Summary.MissingItem != 2 || page.Summary.ProcessingError != 1 {
		t.Fatalf("global summary must count Galleries, except legacy missingItem: %#v", page.Summary)
	}
	errorPage, err := db.Manage().GalleryPage(ctx, 1, "PROCESSING_ERROR", "")
	if err != nil || errorPage.TotalItems != 1 || len(errorPage.Items) != 1 || errorPage.Items[0].SetID != errorGallery.SetID || errorPage.Summary.All != 2 {
		t.Fatalf("processing error page = %#v, err=%v", errorPage, err)
	}
	if _, err := db.Manage().GalleryPage(ctx, 1, "NOT_A_FILTER", ""); err == nil {
		t.Fatal("unsupported issue filter was accepted")
	}
	runID, err := db.Scans().Begin(ctx, missingSource.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Abort(ctx, runID, false, "SOURCE_NOT_FOUND", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	detail, err := db.Manage().GalleryDetail(ctx, missingGallery.SetID)
	if err != nil || len(detail.ScanRuns) != 1 || detail.ScanRuns[0].Status != "FAILED" || detail.ScanRuns[0].ErrorCode != "SOURCE_NOT_FOUND" {
		t.Fatalf("Manage scan history=%#v err=%v", detail.ScanRuns, err)
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

func TestManageCoserPageOrdersEnglishAndChineseByPinyin(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 1, 12, 30, 0, 0, time.UTC)
	inputs := []CreateNamedEntityInput{
		{Name: "张三"},
		{Name: "bob"},
		{Name: "李四"},
		{Name: "Alice"},
		{Name: "小丁"},
		{Name: "Manual Override", SortName: "Aardvark"},
	}
	for _, input := range inputs {
		if _, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: input}, now); err != nil {
			t.Fatal(err)
		}
	}
	page, err := db.CoreEntities().ManagePage(ctx, "COSER", 1)
	if err != nil {
		t.Fatal(err)
	}
	wanted := []string{"Manual Override", "Alice", "bob", "李四", "小丁", "张三"}
	if len(page.Items) != len(wanted) {
		t.Fatalf("Pinyin Coser page length = %d, want %d", len(page.Items), len(wanted))
	}
	for index, name := range wanted {
		if page.Items[index].Name != name {
			t.Fatalf("Pinyin Coser order[%d] = %q, want %q; page = %#v", index, page.Items[index].Name, name, page.Items)
		}
	}
}

func TestManageWorkCharacterAndTagPagesSearchSizeAndPinyinOrder(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)
	workInputs := []CreateNamedEntityInput{
		{Name: "张三", Aliases: []string{"Work Search Alias"}}, {Name: "bob"}, {Name: "Alice"}, {Name: "Manual Work", SortName: "Aardvark"},
	}
	var workUUID string
	for _, input := range workInputs {
		created, err := db.CoreEntities().CreateWork(ctx, input, now)
		if err != nil {
			t.Fatal(err)
		}
		if workUUID == "" {
			workUUID = created.UUID
		}
	}
	characterInputs := []CreateNamedEntityInput{
		{Name: "张角", Aliases: []string{"Character Search Alias"}}, {Name: "bob"}, {Name: "Alice"}, {Name: "Manual Character", SortName: "Aardvark"},
	}
	for _, input := range characterInputs {
		if _, err := db.CoreEntities().CreateCharacter(ctx, workUUID, input, now); err != nil {
			t.Fatal(err)
		}
	}
	tagInputs := []CreateNamedEntityInput{
		{Name: "张贴", Aliases: []string{"Tag Search Alias"}}, {Name: "bob"}, {Name: "Alice"}, {Name: "Manual Tag", SortName: "Aardvark"},
	}
	for _, input := range tagInputs {
		if _, err := db.CoreEntities().CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: input, UseInRecommendation: true}, now); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		kind       string
		query      string
		wantSearch string
		wantOrder  []string
	}{
		{kind: "WORK", query: "Work Search", wantSearch: "张三", wantOrder: []string{"Manual Work", "Alice", "bob", "张三"}},
		{kind: "CHARACTER", query: "Character Search", wantSearch: "张角", wantOrder: []string{"Manual Character", "Alice", "bob", "张角"}},
		{kind: "TAG", query: "Tag Search", wantSearch: "张贴", wantOrder: []string{"Manual Tag", "Alice", "bob", "张贴"}},
	} {
		t.Run(test.kind, func(t *testing.T) {
			searched, err := db.CoreEntities().ManagePageWithOptions(ctx, test.kind, ManageCoreEntityPageOptions{Page: 1, PageSize: 100, Query: test.query})
			if err != nil || searched.PageSize != 100 || searched.TotalItems != 1 || len(searched.Items) != 1 || searched.Items[0].Name != test.wantSearch {
				t.Fatalf("searched %s page = %#v, %v", test.kind, searched, err)
			}
			page, err := db.CoreEntities().ManagePageWithOptions(ctx, test.kind, ManageCoreEntityPageOptions{Page: 1, PageSize: 30})
			if err != nil || len(page.Items) != len(test.wantOrder) {
				t.Fatalf("ordered %s page = %#v, %v", test.kind, page, err)
			}
			for index, name := range test.wantOrder {
				if page.Items[index].Name != name {
					t.Fatalf("ordered %s page[%d] = %q, want %q", test.kind, index, page.Items[index].Name, name)
				}
			}
		})
	}
}

func TestManageCharactersForWorkIsScopedAndUsesManagementOrder(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
	work, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Fate"}, now)
	if err != nil {
		t.Fatal(err)
	}
	otherWork, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Other"}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []CreateNamedEntityInput{{Name: "张三"}, {Name: "Alice"}, {Name: "Manual", SortName: "Aardvark", Aliases: []string{"Manual Alias"}}} {
		if _, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, input, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.CoreEntities().CreateCharacter(ctx, otherWork.UUID, CreateNamedEntityInput{Name: "Must not leak"}, now); err != nil {
		t.Fatal(err)
	}
	characters, err := db.CoreEntities().ManageCharactersForWork(ctx, work.UUID)
	if err != nil {
		t.Fatal(err)
	}
	wanted := []string{"Manual", "Alice", "张三"}
	if len(characters) != len(wanted) {
		t.Fatalf("Work Characters = %#v", characters)
	}
	for index, name := range wanted {
		if characters[index].Name != name || characters[index].WorkUUID != work.UUID || characters[index].WorkName != work.Name {
			t.Fatalf("Work Character[%d] = %#v, want name %q bound to %q", index, characters[index], name, work.Name)
		}
	}
	if len(characters[0].Aliases) != 1 || characters[0].Aliases[0] != "Manual Alias" {
		t.Fatalf("Work Character Aliases = %#v", characters[0].Aliases)
	}
	if _, err := db.CoreEntities().ManageCharactersForWork(ctx, "missing-work"); err == nil {
		t.Fatal("missing Work unexpectedly returned a Character collection")
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

func TestManageCoreEntityNameConflictsIncludeWorkContextAndExactMatches(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 3, 20, 0, 0, 0, time.UTC)
	workA, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Fate/stay night", Aliases: []string{"Fate SN"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	workB, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Fate Grand Order"}, now)
	if err != nil {
		t.Fatal(err)
	}
	characterA, err := db.CoreEntities().CreateCharacter(ctx, workA.UUID, CreateNamedEntityInput{Name: "Saber", Aliases: []string{"Artoria"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	characterB, err := db.CoreEntities().CreateCharacter(ctx, workB.UUID, CreateNamedEntityInput{Name: "Artoria"}, now)
	if err != nil {
		t.Fatal(err)
	}

	works, err := db.CoreEntities().ManageCoreEntityNameConflicts(ctx, "WORK", " fate sn ", 10)
	if err != nil || len(works) != 1 || works[0].Entity.UUID != workA.UUID || works[0].PrimaryNameMatch || len(works[0].MatchedValues) != 1 || works[0].MatchedValues[0] != "Fate SN" {
		t.Fatalf("Work name conflicts = %#v, %v", works, err)
	}
	characters, err := db.CoreEntities().ManageCoreEntityNameConflicts(ctx, "CHARACTER", "ARTORIA", 10)
	if err != nil || len(characters) != 2 {
		t.Fatalf("Character name conflicts = %#v, %v", characters, err)
	}
	byUUID := map[string]ManageCoreEntityNameConflict{}
	for _, conflict := range characters {
		byUUID[conflict.Entity.UUID] = conflict
	}
	if byUUID[characterA.UUID].WorkName != workA.Name || byUUID[characterA.UUID].Entity.WorkName != workA.Name || byUUID[characterA.UUID].PrimaryNameMatch || byUUID[characterA.UUID].MatchedValues[0] != "Artoria" {
		t.Fatalf("Alias Character conflict = %#v", byUUID[characterA.UUID])
	}
	if byUUID[characterB.UUID].WorkName != workB.Name || !byUUID[characterB.UUID].PrimaryNameMatch || byUUID[characterB.UUID].MatchedValues[0] != "Artoria" {
		t.Fatalf("primary Character conflict = %#v", byUUID[characterB.UUID])
	}
	if _, err := db.CoreEntities().ManageCoreEntityNameConflicts(ctx, "COSER", "Alice", 10); err == nil {
		t.Fatal("generic conflict review accepted unsupported Coser kind")
	}
	if partial, err := db.CoreEntities().ManageCoreEntityNameConflicts(ctx, "WORK", "Fate", 10); err != nil || len(partial) != 0 {
		t.Fatalf("partial Work conflicts = %#v, %v", partial, err)
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

func TestReplaceGalleryTagsPreservesLifecycleAndOtherRelations(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	created, source, _ := createCompleteAlbumFixture(t, db, now)
	active, err := db.Galleries().SetState(ctx, created.ID, created.MetadataRevision, gallery.StateActive, now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.CoreEntities().CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Portrait"}, UseInRecommendation: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.CoreEntities().CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Studio"}, UseInRecommendation: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gallery_manifest_sync(
		gallery_id,manifest_path,status,schema_version,manifest_revision,file_hash,baseline_json,
		baseline_metadata_revision,checked_at_utc,last_error_code
	) VALUES(?,'/media/.cosplay.json','CLEAN',1,1,'hash','{}',?,?, '')`, created.ID, active.MetadataRevision, formatTime(now)); err != nil {
		t.Fatal(err)
	}
	// Make the source invalid before the Tag-only edit. The operation must not
	// opportunistically change the lifecycle state for this unrelated fact.
	if _, err := db.ExecContext(ctx, `UPDATE gallery_sources SET availability_state='MISSING' WHERE id=?`, source.ID); err != nil {
		t.Fatal(err)
	}
	input := []ReplaceGalleryTagInput{
		{TagUUID: first.UUID, Position: 1024},
		{TagUUID: second.UUID, Position: 2048},
	}
	if err := db.Galleries().ReplaceTags(ctx, created.ID, active.MetadataRevision, input, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	detail, err := db.Manage().GalleryDetail(ctx, created.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Row.State != gallery.StateActive || detail.Row.MetadataRevision != active.MetadataRevision+1 {
		t.Fatalf("Tag edit changed lifecycle or wrong revision: %#v", detail.Row)
	}
	if len(detail.Credits) != 1 || len(detail.Tags) != 2 || detail.Tags[0].UUID != first.UUID || detail.Tags[1].UUID != second.UUID {
		t.Fatalf("Tag edit changed other relations or ordering: %#v %#v", detail.Credits, detail.Tags)
	}
	var manifestStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM gallery_manifest_sync WHERE gallery_id=?`, created.ID).Scan(&manifestStatus); err != nil {
		t.Fatal(err)
	}
	if manifestStatus != "DB_DIRTY" {
		t.Fatalf("manifest status = %s, want DB_DIRTY", manifestStatus)
	}
	if err := db.Galleries().ReplaceTags(ctx, created.ID, active.MetadataRevision, nil, now.Add(2*time.Minute)); !errors.Is(err, ErrMetadataRevisionConflict) {
		t.Fatalf("stale Tag edit error = %v", err)
	}
}
