package productdb

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

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
