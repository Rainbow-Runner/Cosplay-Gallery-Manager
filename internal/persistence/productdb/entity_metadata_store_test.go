package productdb

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestApplyEntityMetadataAliasesMergesThroughRevisionedEntityUpdates(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.CoreEntities()
	now := time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)

	work, err := store.CreateWork(ctx, CreateNamedEntityInput{Name: "鸣潮", Aliases: []string{"Wuthering Waves"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.ApplyEntityMetadataAliases(ctx, "WORK", work.UUID, work.MetadataRevision,
		[]string{"Wuthering Waves", "鳴潮"}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if updated.MetadataRevision != work.MetadataRevision+1 || !reflect.DeepEqual(updated.Aliases, []string{"Wuthering Waves", "鳴潮"}) {
		t.Fatalf("updated Work = %#v", updated)
	}
	if _, err := store.ApplyEntityMetadataAliases(ctx, "WORK", work.UUID, work.MetadataRevision,
		[]string{"鸣潮手游"}, now.Add(2*time.Minute)); !errors.Is(err, ErrCoreMetadataRevisionConflict) {
		t.Fatalf("stale revision error = %v", err)
	}
}

func TestApplyEntityMetadataAliasesRejectsUnsupportedOrNoOpSelections(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.CoreEntities()
	now := time.Date(2026, 9, 3, 9, 30, 0, 0, time.UTC)

	work, err := store.CreateWork(ctx, CreateNamedEntityInput{Name: "作品", Aliases: []string{"Existing"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyEntityMetadataAliases(ctx, "COSER", work.UUID, work.MetadataRevision, []string{"Alias"}, now); err == nil {
		t.Fatal("Coser metadata aliases were accepted")
	}
	if _, err := store.ApplyEntityMetadataAliases(ctx, "WORK", work.UUID, work.MetadataRevision, []string{"existing"}, now); err == nil {
		t.Fatal("normalized duplicate selection was accepted as an update")
	}
	current, err := store.ManageFind(ctx, "WORK", work.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if current.MetadataRevision != work.MetadataRevision {
		t.Fatalf("no-op selection changed revision to %d", current.MetadataRevision)
	}
}
