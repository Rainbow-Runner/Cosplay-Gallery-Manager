package productdb

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestEntityNameSpansBoundariesAndTags(t *testing.T) {
	for _, tc := range []struct {
		text, name string
		tags       []string
		strength   int
	}{
		{"galaxy rem", "rem", nil, entityMatchStrong},
		{"rem", "galaxy rem", nil, entityMatchNone},
		{"bremerton", "rem", nil, entityMatchNone},
		{"黑天鹅 #黑丝", "黑天鹅", nil, entityMatchStrong},
		{"黑天鹅黑丝", "黑天鹅", nil, entityMatchWeak},
		{"黑天鹅黑丝", "黑天鹅", []string{"黑丝"}, entityMatchStrong},
		{"黑天鹅黑丝未知", "黑天鹅", []string{"黑丝"}, entityMatchWeak},
	} {
		spans := entityNameSpans(tc.text, tc.name, tc.tags)
		got := entityMatchNone
		for _, s := range spans {
			if s.strength > got {
				got = s.strength
			}
		}
		if got != tc.strength {
			t.Errorf("%q / %q: %d, want %d", tc.text, tc.name, got, tc.strength)
		}
	}
}

func TestArchiveNamesHashtagsAndLocalOverlap(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Now()
	library := createTestLibrary(t, db, now)
	_, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "蜜汁猫裘"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	work, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "崩坏：星穹铁道"}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"黑天鹅", "Rem", "Galaxy Rem", "Laburi Rem"} {
		if _, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: name}, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.CoreEntities().CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "黑丝"}}, now); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file string
		want map[string]bool
	}{
		{"蜜汁猫裘 - 黑天鹅 #黑丝 #巨乳 #崩坏：星穹铁道 [74P2V-2.07GB].7z", map[string]bool{"coser:蜜汁猫裘": true, "work:崩坏：星穹铁道": true, "character:黑天鹅": true}},
		{"Galaxy Rem - Rem.7z", map[string]bool{"character:Galaxy Rem": true, "character:Rem": true}},
		{"Rem.7z", map[string]bool{"character:Rem": true}},
		{"黑天鹅未知.7z", map[string]bool{}},
		{"黑天鹅黑丝.7z", map[string]bool{"character:黑天鹅": true}},
	} {
		got, err := archiveEntitySuggestions(ctx, db, library.RootPath, filepath.Join(library.RootPath, tc.file))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: %#v", tc.file, got)
		}
		for _, s := range got {
			if !tc.want[s.Field+":"+s.Value] {
				t.Errorf("unexpected %s: %#v", tc.file, s)
			}
		}
	}
}

func TestAutomationUniqueCharactersWithoutWorkDescription(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Now()
	core := db.CoreEntities()
	if _, err := core.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Coser"}}, now); err != nil {
		t.Fatal(err)
	}
	work, err := core.CreateWork(ctx, CreateNamedEntityInput{Name: "Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"黑天鹅", "卡芙卡"} {
		if _, err := core.CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: name}, now); err != nil {
			t.Fatal(err)
		}
	}
	value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Test"}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range [][2]string{{"COSER", "Coser"}, {"CHARACTER", "黑天鹅"}, {"CHARACTER", "卡芙卡"}} {
		addAutomationIdentitySuggestion(t, db, value.ID, item[0], item[1], now)
	}
	if err := db.Automation().acceptDeterminateIdentities(ctx, value, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_cast WHERE gallery_id=?`, value.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("unique roles without Work: %d, %v", count, err)
	}
}
