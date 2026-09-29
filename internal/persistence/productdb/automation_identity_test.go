package productdb

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

func TestAutomationAcceptsMultipleDeterminateEntitiesAndPreservesTags(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Now().UTC()
	core := db.CoreEntities()
	coserB, err := core.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "B", Aliases: []string{"Bee"}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	coserA, err := core.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "A"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	workA, err := core.CreateWork(ctx, CreateNamedEntityInput{Name: "Work A"}, now)
	if err != nil {
		t.Fatal(err)
	}
	workB, err := core.CreateWork(ctx, CreateNamedEntityInput{Name: "Work B"}, now)
	if err != nil {
		t.Fatal(err)
	}
	charA, err := core.CreateCharacter(ctx, workA.UUID, CreateNamedEntityInput{Name: "Role A", Aliases: []string{"Role Alias"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	charB, err := core.CreateCharacter(ctx, workB.UUID, CreateNamedEntityInput{Name: "Role B"}, now)
	if err != nil {
		t.Fatal(err)
	}
	value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Multiple"}, now)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := core.CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Keep me"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gallery_tags(gallery_id,tag_uuid,position) VALUES(?,?,1024)`, value.ID, tag.UUID); err != nil {
		t.Fatal(err)
	}
	for _, item := range [][2]string{{"COSER", "B"}, {"COSER", "A"}, {"COSER", "Bee"}, {"WORK", "Work A"}, {"WORK", "Work B"}, {"CHARACTER", "Role A"}, {"CHARACTER", "Role Alias"}, {"CHARACTER", "Role B"}} {
		addAutomationIdentitySuggestion(t, db, value.ID, item[0], item[1], now)
	}
	if err := db.Automation().acceptDeterminateIdentities(ctx, value, now); err != nil {
		t.Fatal(err)
	}
	var first, second string
	if err := db.QueryRowContext(ctx, `SELECT coser_uuid FROM gallery_credits WHERE gallery_id=? ORDER BY position LIMIT 1`, value.ID).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT coser_uuid FROM gallery_credits WHERE gallery_id=? ORDER BY position LIMIT 1 OFFSET 1`, value.ID).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if first != coserB.UUID || second != coserA.UUID {
		t.Fatalf("credit order = %s, %s", first, second)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_cast role JOIN gallery_credits credit ON credit.id=role.gallery_credit_id WHERE role.gallery_id=? AND credit.coser_uuid=? AND role.character_uuid IN (?,?)`, value.ID, coserB.UUID, charA.UUID, charB.UUID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("first coser casts = %d: %v", count, err)
	}
	for _, query := range []string{`SELECT COUNT(*) FROM gallery_tags WHERE gallery_id=?`, `SELECT COUNT(*) FROM gallery_identity_suggestions WHERE gallery_id=? AND status='PENDING'`} {
		if err := db.QueryRowContext(ctx, query, value.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		want := 0
		if query == `SELECT COUNT(*) FROM gallery_tags WHERE gallery_id=?` {
			want = 1
		}
		if count != want {
			t.Fatalf("%s: %d", query, count)
		}
	}
	updated, err := db.Galleries().Find(ctx, value.ID)
	if err != nil || updated.MetadataRevision != value.MetadataRevision+1 {
		t.Fatalf("revision: %#v, %v", updated, err)
	}
	// Repeating cannot overwrite an existing relation or bump its revision.
	if err := db.Automation().acceptDeterminateIdentities(ctx, updated, now); err != nil {
		t.Fatal(err)
	}
	after, _ := db.Galleries().Find(ctx, value.ID)
	if after.MetadataRevision != updated.MetadataRevision {
		t.Fatal("repeated automation changed relations")
	}
}

func TestAutomationIdentityAmbiguityAndCoserOnlyAlbum(t *testing.T) {
	for _, mode := range []string{"coser-only", "ambiguous-coser", "ambiguous-character", "work-disambiguation", "work-only-context"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			db, _ := openTestDatabaseAndRegistry(t)
			now := time.Now().UTC()
			core := db.CoreEntities()
			if _, err := core.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Coser"}}, now); err != nil {
				t.Fatal(err)
			}
			work, err := core.CreateWork(ctx, CreateNamedEntityInput{Name: "Work"}, now)
			if err != nil {
				t.Fatal(err)
			}
			character, err := core.CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "Role"}, now)
			if err != nil {
				t.Fatal(err)
			}
			value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: mode}, now)
			if err != nil {
				t.Fatal(err)
			}
			addAutomationIdentitySuggestion(t, db, value.ID, "COSER", "Coser", now)
			wantCredits, wantCast, wantPending := 1, 0, 0
			switch mode {
			case "ambiguous-coser":
				if _, err := core.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Coser"}}, now); err != nil {
					t.Fatal(err)
				}
				wantCredits, wantPending = 0, 1
			case "ambiguous-character", "work-disambiguation":
				other, err := core.CreateWork(ctx, CreateNamedEntityInput{Name: "Other Work"}, now)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := core.CreateCharacter(ctx, other.UUID, CreateNamedEntityInput{Name: "Role"}, now); err != nil {
					t.Fatal(err)
				}
				addAutomationIdentitySuggestion(t, db, value.ID, "CHARACTER", "Role", now)
				wantPending = 1
				if mode == "work-disambiguation" {
					addAutomationIdentitySuggestion(t, db, value.ID, "WORK", "Work", now)
					wantCast, wantPending = 1, 0
				}
			case "work-only-context":
				addAutomationIdentitySuggestion(t, db, value.ID, "WORK", "Work", now)
			}
			if err := db.Automation().acceptDeterminateIdentities(ctx, value, now); err != nil {
				t.Fatal(err)
			}
			for _, check := range []struct {
				sql  string
				want int
			}{{`SELECT COUNT(*) FROM gallery_credits WHERE gallery_id=?`, wantCredits}, {`SELECT COUNT(*) FROM gallery_cast WHERE gallery_id=?`, wantCast}, {`SELECT COUNT(*) FROM gallery_identity_suggestions WHERE gallery_id=? AND status='PENDING'`, wantPending}} {
				var count int
				if err := db.QueryRowContext(ctx, check.sql, value.ID).Scan(&count); err != nil || count != check.want {
					t.Fatalf("%s: %d, want %d: %v", check.sql, count, check.want, err)
				}
			}
			if wantCast > 0 {
				var got string
				if err := db.QueryRowContext(ctx, `SELECT character_uuid FROM gallery_cast WHERE gallery_id=?`, value.ID).Scan(&got); err != nil || got != character.UUID {
					t.Fatalf("wrong role: %s %v", got, err)
				}
			}
		})
	}
}

func TestAutomationIdentitySaveRollsBackWithSuggestionFailure(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Now().UTC()
	if _, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Coser"}}, now); err != nil {
		t.Fatal(err)
	}
	value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Atomic"}, now)
	if err != nil {
		t.Fatal(err)
	}
	addAutomationIdentitySuggestion(t, db, value.ID, "COSER", "Coser", now)
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_identity BEFORE UPDATE ON gallery_identity_suggestions BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := db.Automation().acceptDeterminateIdentities(ctx, value, now); err == nil {
		t.Fatal("expected failure")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_credits WHERE gallery_id=?`, value.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial credits: %d %v", count, err)
	}
	after, _ := db.Galleries().Find(ctx, value.ID)
	if after.MetadataRevision != value.MetadataRevision {
		t.Fatal("partial revision")
	}
	stale := value
	stale.MetadataRevision++
	if err := db.Automation().acceptDeterminateIdentities(ctx, stale, now); !errors.Is(err, ErrMetadataRevisionConflict) {
		t.Fatalf("stale save: %v", err)
	}
}

func TestArchiveMultipleSuggestionsFollowPathAppearance(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Now().UTC()
	for _, name := range []string{"Alpha", "Zulu"} {
		if _, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: name}}, now); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	for range 4 {
		got, err := archiveEntitySuggestions(ctx, db, root, filepath.Join(root, "Zulu - Alpha [86P].7z"))
		if err != nil || len(got) != 2 || got[0].Value != "Zulu" || got[1].Value != "Alpha" {
			t.Fatalf("suggestion order: %#v %v", got, err)
		}
	}
}

func TestAutomationCoserOnlyPolicyCanActivateAlbumAndHonoursOptIn(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Now().UTC()
	value, _, _ := createCompleteAlbumFixture(t, db, now)
	var name string
	if err := db.QueryRowContext(ctx, `SELECT coser.name FROM cosers coser JOIN gallery_credits credit ON credit.coser_uuid=coser.uuid WHERE credit.gallery_id=?`, value.ID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM gallery_credits WHERE gallery_id=?`, value.ID); err != nil {
		t.Fatal(err)
	}
	addAutomationIdentitySuggestion(t, db, value.ID, "COSER", name, now)
	if err := db.Automation().applyAutomationPolicy(ctx, value.ID, LibraryAutomationPolicy{}, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_credits WHERE gallery_id=?`, value.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("opt-in ignored: %d %v", count, err)
	}
	if err := db.Automation().applyAutomationPolicy(ctx, value.ID, LibraryAutomationPolicy{AutoAcceptUniqueEntities: true}, now); err != nil {
		t.Fatal(err)
	}
	updated, err := db.Galleries().Find(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().SetState(ctx, value.ID, updated.MetadataRevision, gallery.StateActive, now); err != nil {
		t.Fatalf("Coser-only Album activation: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_cast WHERE gallery_id=?`, value.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("fabricated Cast: %d %v", count, err)
	}
}

func TestAutomationPreservesReviewedAndManualIdentityState(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Now().UTC()
	library := createTestLibrary(t, db, now)
	if _, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Auto Coser"}}, now); err != nil {
		t.Fatal(err)
	}
	value, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Reviewed"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddSource(ctx, value.ID, CreateSourceInput{LibraryID: &library.ID, Type: gallery.SourceTypeArchive, Path: filepath.Join(library.RootPath, "Auto Coser.zip"), Availability: gallery.AvailabilityAvailable}, now); err != nil {
		t.Fatal(err)
	}
	addAutomationIdentitySuggestion(t, db, value.ID, "COSER", "Auto Coser", now)
	if _, err := db.ExecContext(ctx, `UPDATE gallery_identity_suggestions SET status='REJECTED',resolved_at_utc=? WHERE gallery_id=?`, formatTime(now), value.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Automation().applyAutomationPolicy(ctx, value.ID, LibraryAutomationPolicy{AutoAcceptUniqueEntities: true}, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_identity_suggestions WHERE gallery_id=?`, value.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("reviewed suggestion recreated: %d %v", count, err)
	}
	manual, _, _ := createCompleteAlbumFixture(t, db, now)
	addAutomationIdentitySuggestion(t, db, manual.ID, "COSER", "Auto Coser", now)
	if err := db.Automation().applyAutomationPolicy(ctx, manual.ID, LibraryAutomationPolicy{AutoAcceptUniqueEntities: true}, now); err != nil {
		t.Fatal(err)
	}
	after, err := db.Galleries().Find(ctx, manual.ID)
	if err != nil || after.MetadataRevision != manual.MetadataRevision {
		t.Fatalf("manual relation changed: %#v %v", after, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_identity_suggestions WHERE gallery_id=? AND status='PENDING'`, manual.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("manual evidence accepted: %d %v", count, err)
	}
}

func addAutomationIdentitySuggestion(t *testing.T, db *Database, galleryID int64, kind, value string, now time.Time) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `INSERT INTO gallery_identity_suggestions(gallery_id,suggestion_kind,value,status,created_at_utc) VALUES(?,?,?,'PENDING',?)`, galleryID, kind, value, formatTime(now)); err != nil {
		t.Fatal(err)
	}
}
