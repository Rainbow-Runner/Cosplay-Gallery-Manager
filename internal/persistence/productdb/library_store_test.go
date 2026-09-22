package productdb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/discovery"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaclassification"
	"github.com/stashapp/stash/internal/mediaexclusion"
)

func TestMostSpecificMediaLibraryOwnsPathEvenWhenDisabled(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Libraries()
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	root := t.TempDir()

	parent, err := store.Create(ctx, CreateLibraryInput{
		Name: "Parent", RootPath: root, Enabled: true, CaptureTimezone: "Asia/Shanghai",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.Create(ctx, CreateLibraryInput{
		Name: "Disabled child", RootPath: filepath.Join(root, "cosplay"), Enabled: false,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	owner, err := store.OwnerForPath(ctx, filepath.Join(root, "cosplay", "set-a"))
	if err != nil {
		t.Fatal(err)
	}
	if owner == nil || owner.ID != child.ID {
		t.Fatalf("owner = %#v, want disabled child %d", owner, child.ID)
	}
	boundaries, err := store.ChildBoundaries(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(boundaries) != 1 || boundaries[0].ID != child.ID {
		t.Fatalf("child boundaries = %#v", boundaries)
	}
}

func TestLibraryChangeRequiresExplicitSourceAssignment(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	libraries := db.Libraries()
	galleries := db.Galleries()
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	root := t.TempDir()

	parent, err := libraries.Create(ctx, CreateLibraryInput{Name: "Parent", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	childRoot := filepath.Join(root, "child")
	child, err := libraries.Create(ctx, CreateLibraryInput{Name: "Child", RootPath: childRoot, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := galleries.Create(ctx, CreateGalleryInput{Title: "Set"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := galleries.AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &child.ID, Type: gallery.SourceTypeDirectory,
		Path: filepath.Join(childRoot, "set"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	preview, err := libraries.PreviewChange(ctx, child.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Impacts) != 1 || preview.Impacts[0].SuggestedOwner == nil ||
		*preview.Impacts[0].SuggestedOwner != parent.ID {
		t.Fatalf("change preview = %#v", preview)
	}
	if preview.CurrentRoot != childRoot || preview.Impacts[0].GalleryTitle != "Set" || preview.IgnoredSourceCount != 0 {
		t.Fatalf("incomplete impact preview = %#v", preview)
	}
	persisted, err := findSource(ctx, db.DB, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.LibraryID == nil || *persisted.LibraryID != child.ID {
		t.Fatal("preview silently reassigned GallerySource")
	}

	if err := libraries.AssignSource(ctx, source.ID, nil); err != nil {
		t.Fatal(err)
	}
	persisted, err = findSource(ctx, db.DB, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.LibraryID != nil {
		t.Fatalf("unassigned Source still has library %v", persisted.LibraryID)
	}
	if err := libraries.AssignSource(ctx, source.ID, &parent.ID); err != nil {
		t.Fatal(err)
	}
}

func TestLibrarySourceTransferRequiresFreshOwnerAndContainedTarget(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Libraries()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	root := t.TempDir()
	parent, err := store.Create(ctx, CreateLibraryInput{Name: "Parent", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.Create(ctx, CreateLibraryInput{Name: "Child", RootPath: filepath.Join(root, "child"), Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.Create(ctx, CreateLibraryInput{Name: "Other", RootPath: t.TempDir(), Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Gallery"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &child.ID, Type: gallery.SourceTypeDirectory, Path: filepath.Join(child.RootPath, "set"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.TransferSource(ctx, source.ID, child.ID, &other.ID); err == nil {
		t.Fatal("outside target accepted")
	}
	if err := store.TransferSource(ctx, source.ID, parent.ID, &parent.ID); err == nil {
		t.Fatal("stale expected owner accepted")
	}
	if err := store.TransferSource(ctx, source.ID, child.ID, &parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.TransferSource(ctx, source.ID, child.ID, nil); err == nil {
		t.Fatal("stale preview accepted after transfer")
	}
	persisted, err := findSource(ctx, db.DB, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.LibraryID == nil || *persisted.LibraryID != parent.ID {
		t.Fatalf("binding = %#v", persisted.LibraryID)
	}
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{
		LibraryID: parent.ID, Mode: AutomationAssisted, ExcludeNewRootMedia: true,
	}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Automation().EnqueueRun(ctx, policy, now); err != nil {
		t.Fatal(err)
	}
	if err := store.TransferSource(ctx, source.ID, parent.ID, nil); err == nil {
		t.Fatal("transfer during active automation accepted")
	}
	persisted, err = findSource(ctx, db.DB, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.LibraryID == nil || *persisted.LibraryID != parent.ID {
		t.Fatalf("active-run rejection changed binding: %#v", persisted.LibraryID)
	}
}

func TestGallerySourceCannotBindOutsideSelectedLibrary(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	mediaLibrary, err := db.Libraries().Create(ctx, CreateLibraryInput{
		Name: "Library", RootPath: filepath.Join(t.TempDir(), "library"), Enabled: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Set"}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &mediaLibrary.ID, Type: gallery.SourceTypeDirectory,
		Path: filepath.Join(t.TempDir(), "outside"), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err == nil {
		t.Fatal("GallerySource outside selected library unexpectedly succeeded")
	}
}

func TestLibraryChangeRechecksImpactAndRemapsOwnedPaths(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Libraries()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	oldRoot, newRoot := t.TempDir(), t.TempDir()
	lib, err := store.Create(ctx, CreateLibraryInput{Name: "Old", RootPath: oldRoot, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	g, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Set"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, g.ID, CreateSourceInput{LibraryID: &lib.ID, Type: gallery.SourceTypeDirectory, Path: filepath.Join(oldRoot, "set"), Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.PreviewChange(ctx, lib.ID, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, `INSERT INTO ignored_gallery_sources(library_id,source_path,reason,created_at_utc) VALUES(?,?,?,?)`, lib.ID, filepath.Join(oldRoot, "ignored"), "reviewed", formatTime(now)); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyChange(ctx, lib.ID, newRoot, before.RevisionToken, now); err == nil {
		t.Fatal("stale impact accepted")
	}
	fresh, err := store.PreviewChange(ctx, lib.ID, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.IgnoredSources) != 1 || fresh.RevisionToken == before.RevisionToken {
		t.Fatalf("ignored impact not tracked: %#v", fresh)
	}
	if _, err := db.DB.ExecContext(ctx, `UPDATE gallery_sources SET reconcile_state='SCANNING' WHERE id=?`, source.ID); err != nil {
		t.Fatal(err)
	}
	fresh, err = store.PreviewChange(ctx, lib.ID, newRoot)
	if err != nil || fresh.ScanningSourceCount != 1 {
		t.Fatalf("scanning impact=%#v err=%v", fresh, err)
	}
	if err := store.ApplyChange(ctx, lib.ID, newRoot, fresh.RevisionToken, now); err == nil {
		t.Fatal("active source scan accepted")
	}
	if _, err := db.DB.ExecContext(ctx, `UPDATE gallery_sources SET reconcile_state='NEEDS_RESCAN' WHERE id=?`, source.ID); err != nil {
		t.Fatal(err)
	}
	fresh, err = store.PreviewChange(ctx, lib.ID, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyChange(ctx, lib.ID, newRoot, fresh.RevisionToken, now); err != nil {
		t.Fatal(err)
	}
	moved, err := store.Find(ctx, lib.ID)
	if err != nil || moved.RootPath != newRoot {
		t.Fatalf("library=%#v err=%v", moved, err)
	}
	persisted, err := findSource(ctx, db.DB, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Path != filepath.Join(newRoot, "set") || persisted.Availability != gallery.AvailabilityMissing {
		t.Fatalf("source=%#v", persisted)
	}
	var ignoredPath string
	if err := db.DB.QueryRowContext(ctx, `SELECT source_path FROM ignored_gallery_sources WHERE library_id=?`, lib.ID).Scan(&ignoredPath); err != nil || ignoredPath != filepath.Join(newRoot, "ignored") {
		t.Fatalf("ignored=%q err=%v", ignoredPath, err)
	}
}

func TestLibraryDeleteRequiresExplicitSourceTransferAndFreshPolicyPreview(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.Libraries()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	root := t.TempDir()
	lib, err := store.Create(ctx, CreateLibraryInput{Name: "Delete", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	g, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Keep"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, g.ID, CreateSourceInput{LibraryID: &lib.ID, Type: gallery.SourceTypeDirectory, Path: filepath.Join(root, "set"), Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.PreviewChange(ctx, lib.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyChange(ctx, lib.ID, "", p.RevisionToken, now); err == nil {
		t.Fatal("delete orphaned bound Source")
	}
	if err := store.TransferSource(ctx, source.ID, lib.ID, nil); err != nil {
		t.Fatal(err)
	}
	p, err = store.PreviewChange(ctx, lib.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{LibraryID: lib.ID, Mode: AutomationAssisted, ExcludeNewRootMedia: true}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecognitionRules().Create(ctx, CreateRecognitionRuleInput{LibraryID: lib.ID, Name: "Marker", Kind: discovery.RuleKindMarker, Enabled: true}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MediaClassificationRules().Create(ctx, MediaClassificationRule{LibraryID: &lib.ID, Name: "Selfie", Enabled: true, Subject: mediaclassification.SubjectFileName, Operator: mediaclassification.OperatorExact, Pattern: "selfie.jpg", Category: mediaclassification.CategorySelfie}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{LibraryID: &lib.ID, Name: "Exclude", Enabled: true, Subject: mediaexclusion.SubjectFileName, Operator: mediaexclusion.OperatorExact, Pattern: "skip.jpg", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionExclude}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, `INSERT INTO ignored_gallery_sources(library_id,source_path,reason,created_at_utc) VALUES(?,?,?,?)`, lib.ID, filepath.Join(root, "ignored"), "reviewed", formatTime(now)); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyChange(ctx, lib.ID, "", p.RevisionToken, now); err == nil {
		t.Fatal("stale policy preview accepted")
	}
	run, err := db.Automation().EnqueueRun(ctx, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	p, err = store.PreviewChange(ctx, lib.ID, "")
	if err != nil || p.AutomationMode != "ASSISTED" || p.AutomationPolicyRevision != policy.Revision || len(p.RecognitionRules) != 1 || len(p.ClassificationRules) != 1 || len(p.ExclusionRules) != 1 || len(p.IgnoredSources) != 1 || len(p.UnassignedSourcePaths) != 1 || p.ActiveRunCount != 1 {
		t.Fatalf("policy impact=%#v err=%v", p, err)
	}
	if err := store.ApplyChange(ctx, lib.ID, "", p.RevisionToken, now); err == nil {
		t.Fatal("active automation accepted")
	}
	if _, err := db.Automation().RequestCancel(ctx, run.ID, now); err != nil {
		t.Fatal(err)
	}
	p, err = store.PreviewChange(ctx, lib.ID, "")
	if err != nil || p.ActiveRunCount != 0 || p.AutomationRunCount != 1 {
		t.Fatalf("cancelled run impact=%#v err=%v", p, err)
	}
	if err := store.ApplyChange(ctx, lib.ID, "", p.RevisionToken, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find(ctx, lib.ID); !errors.Is(err, ErrMediaLibraryNotFound) {
		t.Fatalf("deleted library err=%v", err)
	}
	persisted, err := findSource(ctx, db.DB, source.ID)
	if err != nil || persisted.LibraryID != nil {
		t.Fatalf("GallerySource lost or rebound: %#v err=%v", persisted, err)
	}
	var ignoredOwner sql.NullInt64
	if err := db.DB.QueryRowContext(ctx, `SELECT library_id FROM ignored_gallery_sources WHERE source_path=?`, filepath.Join(root, "ignored")).Scan(&ignoredOwner); err != nil || ignoredOwner.Valid {
		t.Fatalf("ignore tombstone lost or still bound: %#v err=%v", ignoredOwner, err)
	}
	if err := db.DB.QueryRowContext(ctx, `SELECT library_id FROM ignored_gallery_sources WHERE source_path=?`, filepath.Join(root, "set")).Scan(&ignoredOwner); err != nil || ignoredOwner.Valid {
		t.Fatalf("unassigned Source was not protected from parent rediscovery: %#v err=%v", ignoredOwner, err)
	}
}
