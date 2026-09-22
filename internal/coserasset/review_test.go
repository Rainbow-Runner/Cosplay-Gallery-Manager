package coserasset

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
)

func TestReviewAndCleanupUnreferencedManagedAssetGroups(t *testing.T) {
	ctx := context.Background()
	database := reviewTestDatabase(t)
	root := t.TempDir()
	coser, err := database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{
		CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Managed Review"},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	currentID, oldID := portableid.New(), portableid.New()
	currentOriginal := "assets/avatar-" + currentID + ".jpg"
	oldOriginal := "assets/avatar-" + oldID + ".png"
	for name, content := range map[string]string{
		filepath.Base(currentOriginal):             "current original",
		"avatar-" + currentID + "-480.jpg":         "current derivative",
		filepath.Base(oldOriginal):                 "old original",
		"avatar-" + oldID + "-480.jpg":             "old derivative",
		"owner-provided-not-managed-by-cgm.txt":    "unknown",
		".coser-derivative-interrupted-upload.tmp": "temporary",
	} {
		writeReviewFile(t, root, coser.UUID, name, content)
	}
	updated, err := database.CoreEntities().SetCoserManagedAsset(ctx, coser.UUID, coser.MetadataRevision, productdb.CoserManagedAssetInput{
		Kind: productdb.CoserAssetAvatar, RelativePath: currentOriginal,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, coser.UUID, "assets", "avatar-"+portableid.New()+".jpg")
	if err := os.Symlink(filepath.Join(root, coser.UUID, "assets", filepath.Base(oldOriginal)), link); err != nil {
		t.Fatal(err)
	}

	service := Service{Database: database, Root: root}
	review, err := service.ReviewUnreferenced(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Groups) != 1 || review.Groups[0].Reason != ReviewReasonReplaced || review.Groups[0].FileCount != 2 {
		t.Fatalf("review = %#v", review)
	}
	if review.IgnoredEntryCount != 3 {
		t.Fatalf("ignored entries = %d", review.IgnoredEntryCount)
	}
	encoded, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "assets/") || strings.Contains(string(encoded), oldID) {
		t.Fatalf("review leaked a managed relative path or asset filename: %s", encoded)
	}

	oldReviewID := review.Groups[0].ID
	updated, err = database.CoreEntities().SetCoserManagedAsset(ctx, coser.UUID, updated.MetadataRevision, productdb.CoserManagedAssetInput{
		Kind: productdb.CoserAssetAvatar, RelativePath: oldOriginal,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CleanupUnreferenced(ctx, []string{oldReviewID}); !errors.Is(err, ErrCleanupReviewStale) {
		t.Fatalf("cleanup after reference change = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, coser.UUID, filepath.FromSlash(oldOriginal))); err != nil {
		t.Fatalf("stale cleanup removed a newly referenced file: %v", err)
	}

	updated, err = database.CoreEntities().SetCoserManagedAsset(ctx, coser.UUID, updated.MetadataRevision, productdb.CoserManagedAssetInput{
		Kind: productdb.CoserAssetAvatar, RelativePath: currentOriginal,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.CleanupUnreferenced(ctx, []string{oldReviewID})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedGroupCount != 1 || result.DeletedFileCount != 2 || result.DeletedByteSize == 0 {
		t.Fatalf("cleanup result = %#v", result)
	}
	if result.Review.Groups == nil {
		t.Fatal("empty cleanup review must encode as an array")
	}
	for _, name := range []string{filepath.Base(oldOriginal), "avatar-" + oldID + "-480.jpg"} {
		if _, err := os.Stat(filepath.Join(root, coser.UUID, "assets", name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cleaned file %q still exists: %v", name, err)
		}
	}
	for _, name := range []string{filepath.Base(currentOriginal), "avatar-" + currentID + "-480.jpg", "owner-provided-not-managed-by-cgm.txt"} {
		if _, err := os.Stat(filepath.Join(root, coser.UUID, "assets", name)); err != nil {
			t.Fatalf("non-selected file %q changed: %v", name, err)
		}
	}
}

func TestReviewClassifiesDeletedCoserAssetsWithoutRemovingProfile(t *testing.T) {
	ctx := context.Background()
	database := reviewTestDatabase(t)
	root := t.TempDir()
	coser, err := database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{
		CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Deleted Managed Profile"},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	assetID := portableid.New()
	writeReviewFile(t, root, coser.UUID, "banner-"+assetID+".webp", "original")
	writeReviewFile(t, root, coser.UUID, "banner-"+assetID+"-960.jpg", "responsive")
	manifestPath := filepath.Join(root, coser.UUID, "coser.json")
	if err := os.WriteFile(manifestPath, []byte(`{"coser_uuid":"`+coser.UUID+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.CoreEntities().DeleteCoreEntity(ctx, portableid.KindCoser, coser.UUID, coser.MetadataRevision, "deleted by owner", time.Now()); err != nil {
		t.Fatal(err)
	}

	service := Service{Database: database, Root: root}
	review, err := service.ReviewUnreferenced(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Groups) != 1 || review.Groups[0].Reason != ReviewReasonDeletedCoser {
		t.Fatalf("deleted Coser review = %#v", review)
	}
	if _, err := service.CleanupUnreferenced(ctx, []string{review.Groups[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("asset cleanup removed the retained Coser profile: %v", err)
	}
	if info, err := os.Stat(filepath.Dir(manifestPath)); err != nil || !info.IsDir() {
		t.Fatalf("asset cleanup removed the retained Coser directory: %#v/%v", info, err)
	}
}

func reviewTestDatabase(t *testing.T) *productdb.Database {
	t.Helper()
	database, err := productdb.Open(context.Background(), filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func writeReviewFile(t *testing.T, root, coserUUID, name, content string) {
	t.Helper()
	directory := filepath.Join(root, coserUUID, "assets")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
