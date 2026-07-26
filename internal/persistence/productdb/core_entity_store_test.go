package productdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portableid"
)

func TestCoreEntityNameAndSlugIdentityRules(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.CoreEntities()
	now := time.Date(2026, 7, 22, 22, 0, 0, 0, time.UTC)

	firstCoser, err := store.CreateCoser(ctx, CreateCoserInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "同名", Aliases: []string{"Alias"}},
		ProfileSummary:         "summary",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	secondCoser, err := store.CreateCoser(ctx, CreateCoserInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "同名"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if firstCoser.UUID == secondCoser.UUID || firstCoser.Slug == secondCoser.Slug || len(firstCoser.Aliases) != 1 {
		t.Fatalf("duplicate-name Cosers = %#v %#v", firstCoser, secondCoser)
	}

	workA, err := store.CreateWork(ctx, CreateNamedEntityInput{Name: "Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	workB, err := store.CreateWork(ctx, CreateNamedEntityInput{Name: "Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if workA.Slug == workB.Slug {
		t.Fatal("duplicate-name Works shared a slug")
	}
	if _, err := store.CreateCharacter(ctx, workA.UUID, CreateNamedEntityInput{Name: "Hero"}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCharacter(ctx, workA.UUID, CreateNamedEntityInput{Name: "hero"}, now); err == nil {
		t.Fatal("same Work accepted normalized duplicate Character name")
	}
	if _, err := store.CreateCharacter(ctx, workB.UUID, CreateNamedEntityInput{Name: "Hero"}, now); err != nil {
		t.Fatalf("different Work rejected same Character name: %v", err)
	}
}

func TestCoreEntityUpdatesKeepSlugStableAndExplicitSlugChangeRedirects(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.CoreEntities()
	now := time.Date(2026, 7, 23, 1, 0, 0, 0, time.UTC)

	work, err := store.CreateWork(ctx, CreateNamedEntityInput{Name: "原作品"}, now)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateWork(ctx, work.UUID, work.MetadataRevision, UpdateNamedEntityInput{Name: "新作品", Aliases: []string{"旧名"}}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Slug != work.Slug || updated.MetadataRevision != 2 {
		t.Fatalf("ordinary update changed stable slug or revision: %#v", updated)
	}
	newSlug, err := store.ChangeSlug(ctx, portableid.KindWork, work.UUID, updated.MetadataRevision, "新作品-特别版", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if newSlug != "新作品-特别版" {
		t.Fatalf("changed slug = %q", newSlug)
	}
	resolved, redirected, err := store.ResolveSlug(ctx, portableid.KindWork, work.Slug)
	if err != nil || !redirected || resolved != work.UUID {
		t.Fatalf("historical slug resolution = %q, %v, %v", resolved, redirected, err)
	}
	if _, err := store.UpdateWork(ctx, work.UUID, 1, UpdateNamedEntityInput{Name: "stale"}, now); !errors.Is(err, ErrCoreMetadataRevisionConflict) {
		t.Fatalf("stale update error = %v", err)
	}
}

func TestCoreEntityDeleteRequiresNoReferencesAndPermanentlyTombstones(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.CoreEntities()
	now := time.Date(2026, 7, 23, 2, 0, 0, 0, time.UTC)

	work, err := store.CreateWork(ctx, CreateNamedEntityInput{Name: "Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := store.CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "Hero"}, now)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := store.PreviewDelete(ctx, portableid.KindWork, work.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.MetadataRevision != work.MetadataRevision || preview.ReferenceCount != 1 || len(preview.Blockers) != 1 ||
		preview.Blockers[0].Code != "CHARACTER" || preview.Blockers[0].ReferenceCount != 1 {
		t.Fatalf("referenced Work delete preview = %#v", preview)
	}
	if err := store.DeleteCoreEntity(ctx, portableid.KindWork, work.UUID, work.MetadataRevision, "test", now); !errors.Is(err, ErrCoreEntityReferenced) {
		t.Fatalf("referenced Work delete error = %v", err)
	}
	if err := store.DeleteCoreEntity(ctx, portableid.KindCharacter, character.UUID, character.MetadataRevision, "test", now); err != nil {
		t.Fatal(err)
	}
	preview, err = store.PreviewDelete(ctx, portableid.KindWork, work.UUID)
	if err != nil || preview.ReferenceCount != 0 || len(preview.Blockers) != 0 {
		t.Fatalf("unreferenced Work delete preview = %#v, %v", preview, err)
	}
	if err := store.DeleteCoreEntity(ctx, portableid.KindWork, work.UUID, work.MetadataRevision, "test", now); err != nil {
		t.Fatal(err)
	}
	for _, uuid := range []string{character.UUID, work.UUID} {
		record, err := db.UUIDRegistry().Lookup(ctx, uuid)
		if err != nil || record.State != PortableUUIDTombstone {
			t.Fatalf("deleted UUID %s state = %#v, %v", uuid, record, err)
		}
	}
	if _, err := store.CreateWork(ctx, CreateNamedEntityInput{UUID: work.UUID, Name: "Recreated"}, now); err == nil {
		t.Fatal("tombstoned Work UUID was recreated")
	}
}

func TestCoreEntityMergeMovesRelationshipsAndKeepsPermanentAliases(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.CoreEntities()
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)

	source, err := store.CreateWork(ctx, CreateNamedEntityInput{Name: "Source Work", Aliases: []string{"Source Alias"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.CreateWork(ctx, CreateNamedEntityInput{Name: "Target Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := store.CreateCharacter(ctx, source.UUID, CreateNamedEntityInput{Name: "Hero"}, now)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := store.PreviewMerge(ctx, portableid.KindWork, source.UUID, target.UUID)
	if err != nil || len(preview.Conflicts) != 0 {
		t.Fatalf("merge preview = %#v, %v", preview, err)
	}
	if _, err := store.Merge(ctx, portableid.KindWork, source.UUID, target.UUID, source.MetadataRevision, target.MetadataRevision, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	resolved, err := db.UUIDRegistry().Resolve(ctx, source.UUID)
	if err != nil || resolved.UUID != target.UUID {
		t.Fatalf("source UUID resolution = %#v, %v", resolved, err)
	}
	moved, err := findCharacter(ctx, db.DB, character.UUID)
	if err != nil || moved.WorkUUID != target.UUID {
		t.Fatalf("moved Character = %#v, %v", moved, err)
	}
	resolvedSlug, redirected, err := store.ResolveSlug(ctx, portableid.KindWork, source.Slug)
	if err != nil || !redirected || resolvedSlug != target.UUID {
		t.Fatalf("source slug resolution = %q, %v, %v", resolvedSlug, redirected, err)
	}
	mergedTarget, err := findWork(ctx, db.DB, target.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mergedTarget.Aliases) != 2 || mergedTarget.Aliases[0] != "Source Work" || mergedTarget.Aliases[1] != "Source Alias" {
		t.Fatalf("merged aliases = %#v", mergedTarget.Aliases)
	}
}

func TestCoreEntityMergePreviewBlocksProfileAndTagRelationshipConflicts(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.CoreEntities()
	now := time.Date(2026, 7, 23, 4, 0, 0, 0, time.UTC)

	sourceCoser, err := store.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "A"}, ProfileSummary: "source"}, now)
	if err != nil {
		t.Fatal(err)
	}
	targetCoser, err := store.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "B"}, ProfileSummary: "target"}, now)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := store.PreviewMerge(ctx, portableid.KindCoser, sourceCoser.UUID, targetCoser.UUID)
	if err != nil || len(preview.Conflicts) == 0 || preview.Conflicts[0].Code != "COSER_PROFILE" {
		t.Fatalf("Coser conflict preview = %#v, %v", preview, err)
	}
	if _, err := store.Merge(ctx, portableid.KindCoser, sourceCoser.UUID, targetCoser.UUID, 1, 1, now); !errors.Is(err, ErrCoreEntityMergeConflict) {
		t.Fatalf("conflicting Coser merge error = %v", err)
	}

	parent, err := store.CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Parent"}, UseInRecommendation: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Child"}, UseInRecommendation: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTagParent(ctx, child.UUID, parent.UUID, 1024, 1, 1, now); err != nil {
		t.Fatal(err)
	}
	preview, err = store.PreviewMerge(ctx, portableid.KindTag, child.UUID, parent.UUID)
	if err != nil || len(preview.Conflicts) == 0 || preview.Conflicts[0].Code != "TAG_SELF_EDGE" {
		t.Fatalf("Tag conflict preview = %#v, %v", preview, err)
	}
}

func TestCommittedCoserMergeCanWriteRetrySafeRedirect(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.CoreEntities()
	now := time.Date(2026, 7, 23, 4, 30, 0, 0, time.UTC)
	source, err := store.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Source"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Target"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Merge(ctx, portableid.KindCoser, source.UUID, target.UUID, 1, 1, now); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := db.Manifests().WriteCoserMergeRedirect(ctx, root, source.UUID, target.UUID); err != nil {
		t.Fatal(err)
	}
	resolved, err := manifest.ReadCoserRedirect(root, source.UUID)
	if err != nil || resolved != target.UUID {
		t.Fatalf("Coser redirect = %q, %v", resolved, err)
	}
}

func TestTagNamesAreGloballyUnambiguousAndDAGRejectsCycles(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.CoreEntities()
	now := time.Date(2026, 7, 22, 22, 0, 0, 0, time.UTC)

	parentA, err := store.CreateTag(ctx, CreateTagInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "Parent A", Aliases: []string{"Shared Alias"}},
		UseInRecommendation:    true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTag(ctx, CreateTagInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "shared alias"},
	}, now); err == nil {
		t.Fatal("Tag primary name reused an existing alias")
	}
	parentB, err := store.CreateTag(ctx, CreateTagInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "Parent B"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateTag(ctx, CreateTagInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "Child"},
		UseInRecommendation:    true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTagParent(ctx, child.UUID, parentA.UUID, 1024, 1, 1, now); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTagParent(ctx, child.UUID, parentB.UUID, 2048, 2, 1, now); err != nil {
		t.Fatalf("Tag DAG rejected second parent: %v", err)
	}
	if err := store.AddTagParent(ctx, parentA.UUID, child.UUID, 1024, 2, 3, now); err == nil {
		t.Fatal("Tag DAG accepted a cycle")
	}
	var edgeCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tag_edges`).Scan(&edgeCount); err != nil {
		t.Fatal(err)
	}
	if edgeCount != 2 {
		t.Fatalf("failed cycle transaction changed edges: %d", edgeCount)
	}
	if err := store.ReplaceTagParents(ctx, child.UUID, 3,
		[]ReplaceTagParentInput{{UUID: parentB.UUID, Position: 1024}},
		[]ExpectedTagRevision{{UUID: parentA.UUID, MetadataRevision: 2}, {UUID: parentB.UUID, MetadataRevision: 2}}, now); err != nil {
		t.Fatalf("replacing Tag parents: %v", err)
	}
	detail, err := store.ManageFind(ctx, "TAG", child.UUID)
	if err != nil || detail.MetadataRevision != 4 || len(detail.Parents) != 1 || detail.Parents[0].UUID != parentB.UUID || detail.Parents[0].MetadataRevision != 3 {
		t.Fatalf("replaced Tag parents = %#v, %v", detail, err)
	}
	if err := store.ReplaceTagParents(ctx, child.UUID, 3, nil,
		[]ExpectedTagRevision{{UUID: parentB.UUID, MetadataRevision: 3}}, now); !errors.Is(err, ErrCoreMetadataRevisionConflict) {
		t.Fatalf("stale Tag parent replacement error = %v", err)
	}
}
