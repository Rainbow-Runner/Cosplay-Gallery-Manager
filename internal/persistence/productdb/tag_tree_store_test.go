package productdb

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestManageTagTreeAndAtomicChildCreation(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.CoreEntities()
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	parent, err := store.CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Parent", Aliases: []string{"Alias"}}, UseInRecommendation: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Child"}, UseInRecommendation: true, ParentUUID: parent.UUID, ExpectedParentRevision: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ManageTagTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "Child" || len(items[0].ParentUUIDs) != 1 || items[0].ParentUUIDs[0] != parent.UUID || items[1].ChildCount != 1 || items[1].MetadataRevision != 2 || len(items[1].Aliases) != 1 || items[1].Aliases[0] != "Alias" {
		t.Fatalf("Tag tree = %#v", items)
	}
	if _, err := store.CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Stale Child"}, ParentUUID: parent.UUID, ExpectedParentRevision: 1}, now); !errors.Is(err, ErrCoreMetadataRevisionConflict) {
		t.Fatalf("stale parent revision: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("stale creation left Tag: %d %v", count, err)
	}
	if err := store.AddTagParent(ctx, child.UUID, parent.UUID, 2048, 1, 2, now); err == nil {
		t.Fatal("duplicate edge accepted")
	}
}
