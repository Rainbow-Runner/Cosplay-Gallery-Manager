package productdb

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manifest"
)

func TestCoserManifestPushAndExplicitNewSocialAccountPull(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 2, 0, 0, 0, time.UTC)
	root := t.TempDir()
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "Coser", Aliases: []string{"Alias"}},
		ProfileSummary:         "Original summary", Biography: "## Biography",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	account, err := db.CoreEntities().AddSocialAccount(ctx, coser.UUID, "twitter", "Main", "coser",
		"https://example.test/main", "ACTIVE", true, 1024, coser.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	coser, err = findCoser(ctx, db.DB, coser.UUID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := db.Manifests().PushCoser(ctx, root, coser.UUID, coser.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := manifest.ReadFile(state.Path, manifest.MaxCoserBytes)
	if err != nil {
		t.Fatal(err)
	}
	document, err := manifest.ParseCoser(bytesReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.SocialAccounts.Value) != 1 || document.SocialAccounts.Value[0].AccountUUID != account.UUID {
		t.Fatalf("pushed Coser Manifest = %#v", document)
	}

	var external map[string]any
	if err := json.Unmarshal(data, &external); err != nil {
		t.Fatal(err)
	}
	external["profile_summary"] = "Externally updated summary"
	accounts := external["social_accounts"].([]any)
	external["social_accounts"] = append(accounts, map[string]any{
		"platform_key": "twitter", "label": "Second", "handle": "coser2",
		"url": "https://example.test/second", "status": "INACTIVE",
		"visible": true, "position": 2048,
	})
	externalData, _ := json.MarshalIndent(external, "", "  ")
	externalData = append(externalData, '\n')
	if err := os.WriteFile(state.Path, externalData, 0o600); err != nil {
		t.Fatal(err)
	}
	state, err = db.Manifests().PullCoser(ctx, root, coser.UUID, coser.MetadataRevision, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != ManifestClean {
		t.Fatalf("Coser Pull state = %#v", state)
	}
	updated, err := findCoser(ctx, db.DB, coser.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ProfileSummary != "Externally updated summary" || updated.MetadataRevision != coser.MetadataRevision+1 {
		t.Fatalf("pulled Coser = %#v", updated)
	}
	var accountCount, inactivePosition int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*), MAX(CASE WHEN status='INACTIVE' THEN position ELSE 0 END)
		FROM coser_social_accounts WHERE coser_uuid = ?
	`, coser.UUID).Scan(&accountCount, &inactivePosition); err != nil {
		t.Fatal(err)
	}
	if accountCount != 2 || inactivePosition != 2048 {
		t.Fatalf("pulled SocialAccounts count=%d inactive position=%d", accountCount, inactivePosition)
	}
}

func TestStandaloneCoserManifestCreatesManageOnlyEntity(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 2, 0, 0, 0, time.UTC)
	root := t.TempDir()
	uuid := "92345678-1234-4123-8123-123456789abc"
	directory := filepath.Join(root, uuid)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := `{
		"schema_version":1,"revision":0,"coser_uuid":"` + uuid + `",
		"updated_at":"2026-07-23T02:00:00Z","name":"Standalone",
		"aliases":["Independent"],"profile_summary":"Long profile",
		"social_accounts":[{"platform_key":"custom_site","label":"Profile",
			"url":"https://example.test/profile","status":"ACTIVE","visible":true,"position":1024}]
	}`
	if err := os.WriteFile(filepath.Join(directory, "coser.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := db.Manifests().ImportStandaloneCoser(ctx, root, uuid, now)
	if err != nil {
		t.Fatal(err)
	}
	if created.UUID != uuid || created.Name != "Standalone" || len(created.Aliases) != 1 {
		t.Fatalf("standalone Coser = %#v", created)
	}
	var credits int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_credits WHERE coser_uuid = ?`, uuid).Scan(&credits); err != nil {
		t.Fatal(err)
	}
	if credits != 0 {
		t.Fatal("standalone Coser unexpectedly gained a Gallery relation")
	}
	state, err := db.Manifests().PushCoser(ctx, root, uuid, created.MetadataRevision, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := manifest.ReadFile(state.Path, manifest.MaxCoserBytes)
	if err != nil {
		t.Fatal(err)
	}
	document, err := manifest.ParseCoser(bytesReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.SocialAccounts.Value) != 1 || document.SocialAccounts.Value[0].AccountUUID == "" {
		t.Fatalf("standalone system Push did not supplement account UUID: %#v", document)
	}
}
