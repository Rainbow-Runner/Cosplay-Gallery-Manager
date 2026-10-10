package productdb

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestEntityInferenceTextRemovesSequenceAndMediaNoise(t *testing.T) {
	for _, value := range []string{
		"NO.002", "vol.313", "120P10G3V-2.13GB", "133P-1.0G",
		"123P-258MB", "55P10G-789M", "88P 3V 940MB",
	} {
		if cleaned := strings.TrimSpace(entityInferenceText(value)); cleaned != "" {
			t.Errorf("entityInferenceText(%q) = %q, want empty", value, cleaned)
		}
	}
	if cleaned := strings.Join(strings.Fields(entityInferenceText("Alice - NO.002 Rem [120P10G3V-2.13GB]")), " "); cleaned != "Alice - Rem [ ]" {
		t.Fatalf("cleaned identity text = %q", cleaned)
	}
	if parts := splitMultiCoserToken("Maxine"); len(parts) != 0 {
		t.Fatalf("ordinary Latin name split as multi-Coser token: %v", parts)
	}
	if parts := splitMultiCoserToken("甲x乙"); len(parts) != 2 || parts[0] != "甲" || parts[1] != "乙" {
		t.Fatalf("Han x separator parts = %v", parts)
	}
}

func TestManageCharacterMatchesRespectBoundariesAndWeakFallback(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	work, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Test Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	rem, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "Rem"}, now)
	if err != nil {
		t.Fatal(err)
	}
	anby, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "安比"}, now)
	if err != nil {
		t.Fatal(err)
	}
	zeroTwo, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "02"}, now)
	if err != nil {
		t.Fatal(err)
	}

	assertCharacters := func(label string, want ...string) {
		t.Helper()
		matches, matchErr := db.Manage().sourceEntityMatches(ctx, []string{label})
		if matchErr != nil {
			t.Fatal(matchErr)
		}
		got := map[string]bool{}
		for _, match := range matches {
			if match.Kind == "CHARACTER" {
				got[match.UUID] = true
			}
		}
		if len(got) != len(want) {
			t.Fatalf("characters for %q = %#v, want %v", label, matches, want)
		}
		for _, uuid := range want {
			if !got[uuid] {
				t.Fatalf("characters for %q = %#v, missing %s", label, matches, uuid)
			}
		}
	}

	assertCharacters("Bremerton")
	assertCharacters("Galaxy Rem", rem.UUID)
	assertCharacters("安比·德玛拉", anby.UUID)
	assertCharacters("安比 德玛拉", anby.UUID)
	assertCharacters("安比德玛拉", anby.UUID)
	assertCharacters("02", zeroTwo.UUID)
	assertCharacters("Alice - 02 - swimsuit", zeroTwo.UUID)
	assertCharacters("Alice 2023-02 swimsuit")
	assertCharacters("NO.002 Original [120P10G3V-2.13GB]")
	assertCharacters("vol.02 Original [88P 3V 940MB]")

	fullName, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "安比德玛拉"}, now)
	if err != nil {
		t.Fatal(err)
	}
	assertCharacters("安比德玛拉", fullName.UUID)
}

func TestManageSourceMatchesIndependentEntitiesAndNoWrites(t *testing.T) {
	for _, scenario := range []struct {
		name, path               string
		kind                     gallery.SourceType
		wantCoser, wantCharacter bool
	}{
		{"directory-coser-only", "麻花麻花酱 - 赛博修女", gallery.SourceTypeDirectory, true, false},
		{"directory-character-only", "阿尔托莉雅", gallery.SourceTypeDirectory, false, true},
		{"archive-coser-only", "麻花麻花酱 - 赛博修女 [86P4V].7z", gallery.SourceTypeArchive, true, false},
		{"archive-character-only", "阿尔托莉雅.tar.gz", gallery.SourceTypeArchive, false, true},
		{"archive-parent-coser", "麻花麻花酱/阿尔托莉雅.tar", gallery.SourceTypeArchive, true, true},
		{"unmatched", "unknown.zip", gallery.SourceTypeArchive, false, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			db, _ := openTestDatabaseAndRegistry(t)
			now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
			library := createTestLibrary(t, db, now)
			coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "麻花麻花酱"}}, now)
			if err != nil {
				t.Fatal(err)
			}
			work, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "命运之夜"}, now)
			if err != nil {
				t.Fatal(err)
			}
			character, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "Saber", Aliases: []string{"阿尔托莉雅"}}, now)
			if err != nil {
				t.Fatal(err)
			}
			value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "unrelated title"}, now)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Galleries().AddSource(ctx, value.ID, CreateSourceInput{LibraryID: &library.ID, Type: scenario.kind, Path: filepath.Join(library.RootPath, filepath.FromSlash(scenario.path)), Availability: gallery.AvailabilityAvailable}, now); err != nil {
				t.Fatal(err)
			}
			before, err := db.Galleries().Find(ctx, value.ID)
			if err != nil {
				t.Fatal(err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				detail, err := db.Manage().GalleryDetail(ctx, value.SetID)
				if err != nil {
					t.Fatal(err)
				}
				matches := map[string]string{}
				for _, match := range detail.FolderMatches {
					matches[match.Kind] = match.UUID
					if match.Kind == "CHARACTER" && (match.WorkUUID != work.UUID || match.WorkName != work.Name) {
						t.Fatalf("missing Work context: %#v", match)
					}
				}
				if (matches["COSER"] == coser.UUID) != scenario.wantCoser || (matches["CHARACTER"] == character.UUID) != scenario.wantCharacter {
					t.Fatalf("matches: %#v", detail.FolderMatches)
				}
				if len(detail.Credits) != 0 {
					t.Fatal("read-only hints wrote relations")
				}
			}
			after, err := db.Galleries().Find(ctx, value.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.MetadataRevision != before.MetadataRevision || after.State != before.State {
				t.Fatal("reading hints changed Gallery")
			}
			var suggestions int
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gallery_identity_suggestions WHERE gallery_id=?", value.ID).Scan(&suggestions); err != nil || suggestions != 0 {
				t.Fatalf("hints changed persistent suggestions: %d %v", suggestions, err)
			}
		})
	}
}

func TestManageSourceMatchesExcludeLibraryNameAndAmbiguousIdentities(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Now().UTC()
	library := createTestLibrary(t, db, now)
	for _, name := range []string{"Same Name", "Same Name", filepath.Base(library.RootPath)} {
		if _, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: name}}, now); err != nil {
			t.Fatal(err)
		}
	}
	value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Same Name"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddSource(ctx, value.ID, CreateSourceInput{LibraryID: &library.ID, Type: gallery.SourceTypeArchive, Path: filepath.Join(library.RootPath, "Same Name.zip"), Availability: gallery.AvailabilityAvailable}, now); err != nil {
		t.Fatal(err)
	}
	detail, err := db.Manage().GalleryDetail(ctx, value.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.FolderMatches) != 0 {
		t.Fatalf("ambiguous/library matches: %#v", detail.FolderMatches)
	}
}
