package productdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/archivefile"
	"github.com/stashapp/stash/internal/manage"
)

// Opt-in business audit against a read-only consistent snapshot. No galleries
// or suggestions are persisted; the result can be rendered outside the test.
func TestGalleryDataBusinessAudit(t *testing.T) {
	snapshot := os.Getenv("CGM_AUDIT_SNAPSHOT")
	if snapshot == "" {
		t.Skip("set CGM_AUDIT_SNAPSHOT, CGM_AUDIT_INPUT and CGM_AUDIT_OUTPUT")
	}
	ctx := context.Background()
	db, err := sql.Open(sqliteDriver, sqliteDSN(snapshot, true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	input, err := os.ReadFile(os.Getenv("CGM_AUDIT_INPUT"))
	if err != nil {
		t.Fatal(err)
	}
	type match struct{ Kind, Name, UUID, WorkUUID, WorkName string }
	type row struct {
		Name, Norm string
		Scan       []match
		Rejected   []match
		Admin      []manage.GalleryFolderMatch
		Problems   []string
	}
	var result []row
	for _, line := range strings.Split(string(input), "\n") {
		fields := strings.Split(line, " | ")
		if len(fields) != 8 || !strings.HasPrefix(fields[0], "| ") || fields[0] == "| #" {
			continue
		}
		current := row{Name: fields[1], Norm: fields[6]}
		name := strings.ReplaceAll(current.Name, `\|`, "|")
		suggestions, err := archiveEntitySuggestions(ctx, db, "/audit", filepath.Join("/audit", name))
		if err != nil {
			t.Fatal(err)
		}
		workContext := map[string]bool{}
		for _, s := range suggestions {
			if s.Field == "work" {
				ids, err := exactEntityUUIDs(ctx, db, "works", "work_aliases", "work_uuid", s.Value, "")
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range ids {
					workContext[id] = true
				}
			}
		}
		for _, s := range suggestions {
			table, aliases, owner := "cosers", "coser_aliases", "coser_uuid"
			if s.Field == "work" {
				table, aliases, owner = "works", "work_aliases", "work_uuid"
			}
			if s.Field == "character" {
				table, aliases, owner = "characters", "character_aliases", "character_uuid"
			}
			ids, err := exactEntityUUIDs(ctx, db, table, aliases, owner, s.Value, "")
			if err != nil {
				t.Fatal(err)
			}
			if s.Field == "character" && len(workContext) > 0 {
				var filtered []string
				for _, id := range ids {
					var work string
					if err := db.QueryRowContext(ctx, `SELECT work_uuid FROM characters WHERE uuid=?`, id).Scan(&work); err != nil {
						t.Fatal(err)
					}
					if workContext[work] {
						filtered = append(filtered, id)
					}
				}
				if len(filtered) == 0 {
					current.Problems = append(current.Problems, "CHARACTER与明确Work上下文冲突："+s.Value)
					for _, id := range ids {
						m := match{Kind: "CHARACTER", UUID: id}
						if err := db.QueryRowContext(ctx, `SELECT character.name,work.uuid,work.name FROM characters character JOIN works work ON work.uuid=character.work_uuid WHERE character.uuid=?`, id).Scan(&m.Name, &m.WorkUUID, &m.WorkName); err != nil {
							t.Fatal(err)
						}
						current.Rejected = append(current.Rejected, m)
					}
				}
				ids = filtered
			}
			if len(ids) > 1 {
				current.Problems = append(current.Problems, strings.ToUpper(s.Field)+"名称对应多个UUID："+s.Value)
			}
			for _, id := range ids {
				m := match{Kind: strings.ToUpper(s.Field), UUID: id}
				if s.Field == "character" {
					err = db.QueryRowContext(ctx, `SELECT character.name,work.uuid,work.name FROM characters character JOIN works work ON work.uuid=character.work_uuid WHERE character.uuid=?`, id).Scan(&m.Name, &m.WorkUUID, &m.WorkName)
				} else {
					err = db.QueryRowContext(ctx, `SELECT name FROM `+table+` WHERE uuid=?`, id).Scan(&m.Name)
				}
				if err != nil {
					t.Fatal(err)
				}
				current.Scan = append(current.Scan, m)
			}
		}
		labels := strings.Split(filepath.ToSlash(name), "/")
		labels[len(labels)-1] = archivefile.BaseName(labels[len(labels)-1])
		current.Admin, err = (&ManageStore{db: db}).sourceEntityMatches(ctx, labels)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, current)
	}
	if len(result) != 1170 {
		t.Fatalf("input count %d, expected 1170", len(result))
	}
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("CGM_AUDIT_OUTPUT"), output, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("audited %d names using production matchers against read-only snapshot", len(result))
}
