package productdb

import (
	"context"
	"testing"
	"time"
)

func TestCoserMetadataImportIsOneRevisionAndOneTransaction(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := db.CoreEntities()
	coser, err := store.CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Import Coser"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	input := CoserMetadataImportInput{
		Avatar:   &CoserManagedAssetInput{Kind: CoserAssetAvatar, RelativePath: "assets/avatar-11111111-1111-4111-8111-111111111111.jpg"},
		Banner:   &CoserManagedAssetInput{Kind: CoserAssetBanner, RelativePath: "assets/banner-22222222-2222-4222-8222-222222222222.jpg"},
		Accounts: []SocialAccountInput{{PlatformKey: "twitter", Label: "X", Handle: "example", URL: "https://twitter.com/example", Status: "ACTIVE", Visible: true}},
	}
	updated, err := store.ApplyCoserMetadataImport(ctx, coser.UUID, coser.MetadataRevision, input, now)
	if err != nil {
		t.Fatal(err)
	}
	managed, err := store.ManageFind(ctx, "COSER", coser.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MetadataRevision != coser.MetadataRevision+1 || managed.AvatarPath == "" || managed.BannerPath == "" || len(managed.SocialAccounts) != 1 {
		t.Fatalf("imported Coser = %#v / %#v", updated, managed)
	}
	if _, err := store.ApplyCoserMetadataImport(ctx, coser.UUID, coser.MetadataRevision, CoserMetadataImportInput{
		Accounts: []SocialAccountInput{{PlatformKey: "invalid key", URL: "https://example.test", Status: "ACTIVE"}},
	}, now); err == nil {
		t.Fatal("invalid aggregate import unexpectedly succeeded")
	}
	managed, err = store.ManageFind(ctx, "COSER", coser.UUID)
	if err != nil || managed.MetadataRevision != updated.MetadataRevision || len(managed.SocialAccounts) != 1 {
		t.Fatalf("failed import changed aggregate: %#v, %v", managed, err)
	}
}
