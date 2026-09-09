package productdb

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/portableid"
)

func TestImportPortableCorePreservesCoreAndReservesGalleryIdentities(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	bundle, identities := portableCoreImportFixture()
	importID := "99999999-9999-4999-8999-999999999999"
	createPortableImportTestSession(t, db, importID, len(identities), 6, 1, 1, 1)
	if err := db.BeginPortableImport(ctx, importID, time.Now()); err != nil {
		t.Fatal(err)
	}
	result, err := db.ImportPortableCore(ctx, importID, bundle, sliceIdentityStream(identities), nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.CoreIdentityCount != 8 || result.CoreEntityCount != 6 || result.ClaimCount != 3 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	for table, want := range map[string]int{"portable_uuid_registry": 8, "portable_uuid_aliases": 1, "portable_uuid_tombstones": 1, "cosers": 1, "coser_aliases": 1, "works": 1, "work_aliases": 1, "characters": 1, "tags": 2, "tag_edges": 1, "coser_social_accounts": 1, "slug_redirects": 1, "portable_identity_claims": 3} {
		var got int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&got); err != nil || got != want {
			t.Fatalf("%s count=%d want=%d err=%v", table, got, want, err)
		}
	}
	var state string
	if err := db.QueryRowContext(ctx, `SELECT state FROM portable_import_sessions WHERE import_id=?`, importID).Scan(&state); err != nil || state != "CORE_IMPORTED" {
		t.Fatalf("session state=%q err=%v", state, err)
	}
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT metadata_revision FROM cosers WHERE uuid=?`, identities[0].UUID).Scan(&revision); err != nil || revision != 7 {
		t.Fatalf("Coser revision=%d err=%v", revision, err)
	}

	galleryUUID := identities[4].UUID
	if _, err := db.UUIDRegistry().Register(ctx, galleryUUID, portableid.KindGallery, time.Now()); err == nil || !strings.Contains(err.Error(), "reserved by a pending import claim") {
		t.Fatalf("pending claim did not reserve UUID: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM portable_identity_claims WHERE uuid=?`, galleryUUID); err == nil {
		t.Fatal("portable identity claim was deletable")
	}
	if _, err := db.ExecContext(ctx, `UPDATE portable_identity_claims SET claim_state='CLAIMED',claimed_at_utc=? WHERE uuid=?`, time.Now().UTC().Format(time.RFC3339Nano), galleryUUID); err == nil {
		t.Fatal("pending claim skipped the required CLAIMING transition")
	}
	if _, err := db.ExecContext(ctx, `UPDATE portable_identity_claims SET claim_state='CLAIMING' WHERE uuid=?`, galleryUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UUIDRegistry().Register(ctx, galleryUUID, portableid.KindGallery, time.Now()); err != nil {
		t.Fatalf("claiming UUID could not be registered: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE portable_identity_claims SET claim_state='CLAIMED',claimed_at_utc=? WHERE uuid=?`, time.Now().UTC().Format(time.RFC3339Nano), galleryUUID); err != nil {
		t.Fatal(err)
	}
}

func TestImportPortableCoreRollsBackWhenAssetPublishFails(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bundle, identities := portableCoreImportFixture()
	importID := "88888888-8888-4888-8888-888888888888"
	createPortableImportTestSession(t, db, importID, len(identities), 6, 1, 1, 1)
	if err := db.BeginPortableImport(ctx, importID, time.Now()); err != nil {
		t.Fatal(err)
	}
	publishErr := errors.New("publish failed")
	if _, err := db.ImportPortableCore(ctx, importID, bundle, sliceIdentityStream(identities), func() error { return publishErr }, time.Now()); !errors.Is(err, publishErr) {
		t.Fatalf("import error=%v", err)
	}
	for _, table := range []string{"portable_uuid_registry", "portable_identity_claims", "cosers", "works", "characters", "tags", "coser_social_accounts", "tag_edges", "slug_redirects"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s was not rolled back: count=%d err=%v", table, count, err)
		}
	}
	var state string
	if err := db.QueryRowContext(ctx, `SELECT state FROM portable_import_sessions WHERE import_id=?`, importID).Scan(&state); err != nil || state != "IMPORTING" {
		t.Fatalf("session state=%q err=%v", state, err)
	}
}

func TestSetPortableLibraryMappingsRequiresCompleteExplicitDecisions(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 9, 14, 0, 0, 0, time.UTC)
	enabled, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Enabled", RootPath: t.TempDir(), Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Disabled", RootPath: t.TempDir(), Enabled: false}, now)
	if err != nil {
		t.Fatal(err)
	}
	importID := "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	if err := db.CreatePortableImportSession(ctx, PortableImportSessionInput{
		ImportID: importID, ExportID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", PackageSHA256: strings.Repeat("a", 64),
		PackageRelativePath: "portable-imports/" + importID + "/package.zip", FormatVersion: 1,
		GalleryIndex: portablecatalog.GalleryIndex{SchemaVersion: 1, Libraries: []portablecatalog.LibraryLocator{{Key: "library-000001", Name: "First"}, {Key: "library-000002", Name: "Second"}}},
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE portable_import_sessions SET state='CORE_IMPORTED' WHERE import_id=?`, importID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPortableLibraryMappings(ctx, importID, []PortableLibraryDecision{{LibraryKey: "library-000001", TargetLibraryID: &enabled.ID}}, now); err == nil {
		t.Fatal("incomplete portable library decision set was accepted")
	}
	if err := db.SetPortableLibraryMappings(ctx, importID, []PortableLibraryDecision{{LibraryKey: "library-000001", TargetLibraryID: &disabled.ID}, {LibraryKey: "library-000002"}}, now); err == nil {
		t.Fatal("disabled portable library target was accepted")
	}
	if err := db.SetPortableLibraryMappings(ctx, importID, []PortableLibraryDecision{{LibraryKey: "library-000001", TargetLibraryID: &enabled.ID}, {LibraryKey: "library-000002"}}, now); err != nil {
		t.Fatal(err)
	}
	mappings, err := db.ListPortableLibraryMappings(ctx, importID)
	if err != nil || len(mappings) != 2 || mappings[0].Decision != "MAPPED" || mappings[1].Decision != "SKIPPED" {
		t.Fatalf("mappings=%+v err=%v", mappings, err)
	}
}

func TestFinalizePortableGalleryRebuildPublishesDeferredLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 9, 15, 0, 0, 0, time.UTC)
	importID := "ffffffff-ffff-4fff-8fff-ffffffffffff"
	createPortableImportTestSession(t, db, importID, 0, 0, 0, 0, 0)
	if _, err := db.ExecContext(ctx, `UPDATE portable_import_sessions SET state='LIBRARIES_MAPPED' WHERE import_id=?`, importID); err != nil {
		t.Fatal(err)
	}
	targetUUID := "11111111-2222-4333-8444-555555555555"
	aliasUUID := "22222222-3333-4444-8555-666666666666"
	tombstoneUUID := "33333333-4444-4555-8666-777777777777"
	created := now.Format(time.RFC3339Nano)
	retired := now.Add(time.Hour).Format(time.RFC3339Nano)
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO portable_identity_claims(uuid,import_id,entity_kind,identity_state,created_at_utc) VALUES(?,?,'GALLERY','ACTIVE',?)`, []any{targetUUID, importID, created}},
		{`INSERT INTO portable_identity_claims(uuid,import_id,entity_kind,identity_state,target_uuid,created_at_utc,retired_at_utc) VALUES(?,?,'GALLERY','ALIAS',?,?,?)`, []any{aliasUUID, importID, targetUUID, created, retired}},
		{`INSERT INTO portable_identity_claims(uuid,import_id,entity_kind,identity_state,created_at_utc,retired_at_utc,reason) VALUES(?,?,'GALLERY_ITEM','TOMBSTONE',?,?,?)`, []any{tombstoneUUID, importID, created, retired, "removed"}},
	} {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := claimPortableUUID(ctx, tx, importID, targetUUID, portableid.KindGallery, now); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.FinalizePortableGalleryRebuilds(ctx, importID, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for table, uuid := range map[string]string{"portable_uuid_aliases": aliasUUID, "portable_uuid_tombstones": tombstoneUUID} {
		var count int
		column := "uuid"
		if table == "portable_uuid_aliases" {
			column = "alias_uuid"
		}
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+column+`=?`, uuid).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s lifecycle count=%d err=%v", table, count, err)
		}
	}
	var state string
	if err := db.QueryRowContext(ctx, `SELECT state FROM portable_import_sessions WHERE import_id=?`, importID).Scan(&state); err != nil || state != "GALLERIES_REBUILT" {
		t.Fatalf("session state=%q err=%v", state, err)
	}
}

func portableCoreImportFixture() (portablecatalog.Bundle, []portablecatalog.IdentityRecord) {
	const timestamp = "2026-09-08T00:00:00Z"
	coserUUID := "11111111-1111-4111-8111-111111111111"
	workUUID := "22222222-2222-4222-8222-222222222222"
	characterUUID := "33333333-3333-4333-8333-333333333333"
	tagUUID := "44444444-4444-4444-8444-444444444444"
	childTagUUID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab"
	accountUUID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	identities := []portablecatalog.IdentityRecord{
		{UUID: coserUUID, Kind: "COSER", State: "ACTIVE", CreatedAt: timestamp},
		{UUID: workUUID, Kind: "WORK", State: "ACTIVE", CreatedAt: timestamp},
		{UUID: characterUUID, Kind: "CHARACTER", State: "ACTIVE", CreatedAt: timestamp},
		{UUID: tagUUID, Kind: "TAG", State: "ACTIVE", CreatedAt: timestamp},
		{UUID: "55555555-5555-4555-8555-555555555555", Kind: "GALLERY", State: "ACTIVE", CreatedAt: timestamp},
		{UUID: "66666666-6666-4666-8666-666666666666", Kind: "GALLERY_ITEM", State: "ACTIVE", CreatedAt: timestamp},
		{UUID: "77777777-7777-4777-8777-777777777777", Kind: "EXTERNAL_LINK", State: "ACTIVE", CreatedAt: timestamp},
		{UUID: childTagUUID, Kind: "TAG", State: "ACTIVE", CreatedAt: timestamp},
		{UUID: accountUUID, Kind: "SOCIAL_ACCOUNT", State: "ACTIVE", CreatedAt: timestamp},
		{UUID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Kind: "COSER", State: "ALIAS", CreatedAt: timestamp, TargetUUID: coserUUID, RetiredAt: "2026-09-08T01:00:00Z"},
		{UUID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", Kind: "WORK", State: "TOMBSTONE", CreatedAt: timestamp, RetiredAt: "2026-09-08T01:00:00Z", Reason: "deleted"},
	}
	named := func(uuid, name, slug string) portablecatalog.NamedEntity {
		return portablecatalog.NamedEntity{UUID: uuid, Name: name, SortName: name, Aliases: []string{}, Slug: slug, MetadataRevision: 7, CreatedAt: timestamp, UpdatedAt: timestamp}
	}
	bundle := portablecatalog.Bundle{Catalog: portablecatalog.CoreCatalog{
		SchemaVersion: 1,
		Cosers:        []portablecatalog.Coser{{NamedEntity: named(coserUUID, "Coser", "coser")}},
		Works:         []portablecatalog.Work{{NamedEntity: named(workUUID, "Work", "work")}},
		Characters:    []portablecatalog.Character{{NamedEntity: named(characterUUID, "Character", "character"), WorkUUID: workUUID}},
		Tags:          []portablecatalog.Tag{{NamedEntity: named(tagUUID, "Tag", "tag"), UseInRecommendation: true}, {NamedEntity: named(childTagUUID, "Child", "child"), UseInRecommendation: true}},
		TagEdges:      []portablecatalog.TagEdge{{ParentUUID: tagUUID, ChildUUID: childTagUUID, Position: 1}},
		Accounts:      []portablecatalog.SocialAccount{{UUID: accountUUID, CoserUUID: coserUUID, PlatformKey: "twitter", Label: "X", Handle: "coser", URL: "https://example.com/coser", Status: "ACTIVE", Visible: true, Position: 1}},
		SlugRedirects: []portablecatalog.SlugRedirect{{Kind: "WORK", OldSlug: "old-work", TargetUUID: workUUID, CreatedAt: timestamp}},
	}}
	bundle.Catalog.Cosers[0].Aliases = []string{"Coser Alias"}
	bundle.Catalog.Works[0].Aliases = []string{"Work Alias"}
	return bundle, identities
}

func sliceIdentityStream(values []portablecatalog.IdentityRecord) func(func(portablecatalog.IdentityRecord) error) (int, error) {
	return func(yield func(portablecatalog.IdentityRecord) error) (int, error) {
		for index, value := range values {
			if err := yield(value); err != nil {
				return index, err
			}
		}
		return len(values), nil
	}
}

func createPortableImportTestSession(t *testing.T, db *Database, importID string, identities, core, galleries, items, links int) {
	t.Helper()
	err := db.CreatePortableImportSession(context.Background(), PortableImportSessionInput{
		ImportID: importID, ExportID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", PackageSHA256: strings.Repeat("a", 64),
		PackageRelativePath: "portable-imports/" + importID + "/package.zip", FormatVersion: 1,
		IdentityCount: identities, CoreEntityCount: core, GalleryClaimCount: galleries, ItemClaimCount: items, LinkClaimCount: links,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
}
