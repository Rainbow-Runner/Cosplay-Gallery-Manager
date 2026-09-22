package productdb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIgnoredSourceRemovalRequiresFreshPreviewAndPreservesMedia(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	root := t.TempDir()
	mediaRoot := filepath.Join(root, "global-set")
	if err := os.Mkdir(mediaRoot, 0755); err != nil {
		t.Fatal(err)
	}
	lib, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Library", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CandidateDiscovery().IgnoreSource(ctx, nil, nil, mediaRoot, "LIBRARY_DELETED_UNASSIGNED_SOURCE", now); err != nil {
		t.Fatal(err)
	}
	if err := db.CandidateDiscovery().IgnoreSource(ctx, &lib.ID, nil, filepath.Join(root, "local-set"), "MANUAL", now); err != nil {
		t.Fatal(err)
	}
	global, err := db.Libraries().ListIgnoredSources(ctx, nil, 1, "global-set")
	if err != nil || global.Total != 1 || len(global.Items) != 1 || global.Items[0].LibraryID != nil {
		t.Fatalf("global list=%#v err=%v", global, err)
	}
	local, err := db.Libraries().ListIgnoredSources(ctx, &lib.ID, 1, "")
	if err != nil || local.Total != 1 || local.Items[0].LibraryID == nil {
		t.Fatalf("local list=%#v err=%v", local, err)
	}
	id := global.Items[0].ID
	preview, err := db.Libraries().PreviewIgnoredSourceRemoval(ctx, id)
	if err != nil || len(preview.AffectedLibraryIDs) != 1 || preview.AffectedLibraryIDs[0] != lib.ID {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	if _, err := db.DB.ExecContext(ctx, `UPDATE ignored_gallery_sources SET reason='UPDATED' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err := db.Libraries().RevokeIgnoredSource(ctx, id, preview.RevisionToken); err == nil {
		t.Fatal("stale ignore preview accepted")
	}
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{LibraryID: lib.ID, Mode: AutomationAssisted, ExcludeNewRootMedia: true}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	run, err := db.Automation().EnqueueRun(ctx, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	preview, err = db.Libraries().PreviewIgnoredSourceRemoval(ctx, id)
	if err != nil || preview.ActiveRunCount != 1 {
		t.Fatalf("active preview=%#v err=%v", preview, err)
	}
	if err := db.Libraries().RevokeIgnoredSource(ctx, id, preview.RevisionToken); err == nil {
		t.Fatal("active automation accepted")
	}
	if _, err := db.Automation().RequestCancel(ctx, run.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, `INSERT INTO discovery_snapshots(library_id,completed_at_utc) VALUES(?,?)`, lib.ID, formatTime(now)); err != nil {
		t.Fatal(err)
	}
	preview, err = db.Libraries().PreviewIgnoredSourceRemoval(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Libraries().RevokeIgnoredSource(ctx, id, preview.RevisionToken); err != nil {
		t.Fatal(err)
	}
	global, err = db.Libraries().ListIgnoredSources(ctx, nil, 1, "")
	if err != nil || global.Total != 0 {
		t.Fatalf("global ignore still present: %#v err=%v", global, err)
	}
	var snapshots int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM discovery_snapshots WHERE library_id=?`, lib.ID).Scan(&snapshots); err != nil || snapshots != 0 {
		t.Fatalf("snapshot not invalidated: %d %v", snapshots, err)
	}
	if _, err := os.Stat(mediaRoot); err != nil {
		t.Fatalf("media path changed: %v", err)
	}
}

func TestIgnoredSetIDRemovalInvalidatesAllLibraryDiscoveries(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	first, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "First", RootPath: t.TempDir(), Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Second", RootPath: t.TempDir(), Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	setID := "00000000-0000-4000-8000-000000000001"
	if err := db.CandidateDiscovery().IgnoreSource(ctx, &first.ID, &setID, filepath.Join(first.RootPath, "deleted"), "GALLERY_DELETED", now); err != nil {
		t.Fatal(err)
	}
	page, err := db.Libraries().ListIgnoredSources(ctx, &first.ID, 1, "")
	if err != nil || page.Total != 1 {
		t.Fatalf("list=%#v err=%v", page, err)
	}
	preview, err := db.Libraries().PreviewIgnoredSourceRemoval(ctx, page.Items[0].ID)
	if err != nil || len(preview.AffectedLibraryIDs) != 2 || preview.AffectedLibraryIDs[0] != first.ID || preview.AffectedLibraryIDs[1] != second.ID {
		t.Fatalf("set-id impact=%#v err=%v", preview, err)
	}
	for _, libraryID := range []int64{first.ID, second.ID} {
		if _, err := db.DB.ExecContext(ctx, `INSERT INTO discovery_snapshots(library_id,completed_at_utc) VALUES(?,?)`, libraryID, formatTime(now)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Libraries().RevokeIgnoredSource(ctx, page.Items[0].ID, preview.RevisionToken); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM discovery_snapshots`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("stale cross-library snapshots=%d err=%v", remaining, err)
	}
}
