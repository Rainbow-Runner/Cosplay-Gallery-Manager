package productdb

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/coreentity"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/product"
)

func TestPortableCatalogSnapshotIncludesCoreRelationshipsWithoutLocalPaths(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "portable.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	store := db.CoreEntities()
	coser, err := store.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Alice", SortName: "alice", Aliases: []string{"爱丽丝"}}, ProfileSummary: "Profile", Biography: "Biography", CountryOrRegion: "CN"}, now)
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.AddSocialAccount(ctx, coser.UUID, "x", "X", "alice", "https://example.com/alice", "ACTIVE", true, 1024, coser.MetadataRevision, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	coser, err = store.FindCoser(ctx, coser.UUID)
	if err != nil {
		t.Fatal(err)
	}
	coser, err = store.SetCoserManagedAsset(ctx, coser.UUID, coser.MetadataRevision, CoserManagedAssetInput{Kind: CoserAssetAvatar, RelativePath: "assets/avatar-11111111-1111-4111-8111-111111111111.png", AvatarCrop: &coreentity.AvatarCrop{X: 0.1, Y: 0.2, Size: 0.7}}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	work, err := store.CreateWork(ctx, CreateNamedEntityInput{Name: "Work", Aliases: []string{"作品"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := store.CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "Hero"}, now)
	if err != nil {
		t.Fatal(err)
	}
	category := false
	parent, err := store.CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Parent"}, UseInRecommendation: true, AllowDirectAssignment: &category}, now)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Child"}, UseInRecommendation: false}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTagParent(ctx, child.UUID, parent.UUID, 1024, child.MetadataRevision, parent.MetadataRevision, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ChangeSlug(ctx, portableid.KindWork, work.UUID, work.MetadataRevision, "renamed-work", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	manifest := portablecatalog.PackageManifest{Format: portablecatalog.Format, FormatVersion: portablecatalog.FormatVersion, ProductID: product.ID, ExportID: "77777777-7777-4777-8777-777777777777", CreatedAt: now.Format(time.RFC3339), Versions: portablecatalog.Versions(product.CurrentVersions("1.5.0-test"))}
	snapshot, err := db.PortableCatalogSnapshot(ctx, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Bundle.Catalog.Cosers) != 1 || len(snapshot.Bundle.Catalog.Works) != 1 || len(snapshot.Bundle.Catalog.Characters) != 1 || len(snapshot.Bundle.Catalog.Tags) != 2 || len(snapshot.Bundle.Catalog.Accounts) != 1 || len(snapshot.Bundle.Catalog.TagEdges) != 1 || len(snapshot.Bundle.Catalog.SlugRedirects) != 1 {
		t.Fatalf("unexpected portable catalog counts: %+v", snapshot.Bundle.Catalog)
	}
	policies := map[string]bool{}
	for _, value := range snapshot.Bundle.Catalog.Tags {
		policies[value.Name] = value.DirectAssignmentAllowed()
	}
	if !policies["Child"] || policies["Parent"] {
		t.Fatalf("portable Tag assignment policy was not preserved: %#v", snapshot.Bundle.Catalog.Tags)
	}
	if snapshot.Bundle.Catalog.Accounts[0].UUID != account.UUID || snapshot.Bundle.Catalog.Characters[0].UUID != character.UUID || snapshot.Bundle.Catalog.Characters[0].WorkUUID != work.UUID {
		t.Fatal("portable relationships were not preserved")
	}
	if len(snapshot.Assets) != 1 || snapshot.Bundle.Catalog.Cosers[0].Avatar == nil || snapshot.Bundle.Catalog.Cosers[0].AvatarCrop == nil {
		t.Fatal("current avatar metadata was not exported")
	}
	encoded, err := json.Marshal(snapshot.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{db.Path(), filepath.Dir(db.Path()), "root_path", "source_path", "manifest_path"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("portable snapshot leaked %q", forbidden)
		}
	}
	if err := snapshot.Bundle.Validate(); err != nil {
		t.Fatal(err)
	}
	preflight, err := db.PortableCatalogPreflight(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.IdentityCount != 6 || preflight.IdentityByKind["TAG"] != 2 || preflight.IdentityByState["ACTIVE"] != 6 {
		t.Fatalf("unexpected preflight identity counts: %+v", preflight)
	}
	if preflight.CoserCount != 1 || preflight.WorkCount != 1 || preflight.CharacterCount != 1 || preflight.TagCount != 2 || preflight.AccountCount != 1 || preflight.AssetCount != 1 {
		t.Fatalf("unexpected preflight catalog counts: %+v", preflight)
	}
	if preflight.BlockingCount() != 0 || preflight.WarningCount() != 0 || preflight.IncompleteGalleryCount != 0 {
		t.Fatalf("unexpected preflight findings: %+v", preflight.Issues)
	}
}
