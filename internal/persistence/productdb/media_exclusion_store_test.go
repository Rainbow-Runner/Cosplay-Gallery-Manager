package productdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/mediaexclusion"
)

func TestMediaExclusionRuleValidationRevisionAndScopePriority(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	library, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Rules", RootPath: t.TempDir(), Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	invalid := MediaExclusionRule{Name: "bad", Enabled: true, Subject: mediaexclusion.SubjectFileName,
		Operator: mediaexclusion.OperatorRE2, Pattern: "([", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionExclude}
	if _, err := db.MediaExclusionRules().Create(ctx, invalid, now); err == nil {
		t.Fatal("invalid RE2 was persisted")
	}

	global, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{
		Name: "global exclude", Enabled: true, Order: 50, Subject: mediaexclusion.SubjectParentFolder,
		Operator: mediaexclusion.OperatorExact, Pattern: "reject", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionExclude,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	specific, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{
		LibraryID: &library.ID, Name: "library include", Enabled: true, Order: 50, Subject: mediaexclusion.SubjectParentFolder,
		Operator: mediaexclusion.OperatorExact, Pattern: "reject", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionInclude,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := db.MediaExclusionRules().List(ctx, &library.ID)
	if err != nil || len(rules) != 2 || rules[0].ID != specific.ID || rules[1].ID != global.ID {
		t.Fatalf("effective rule order=%#v err=%v", rules, err)
	}

	global.Pattern = "reject\ntrash"
	updated, err := db.MediaExclusionRules().Update(ctx, global, now.Add(time.Minute))
	if err != nil || updated.Revision != 2 {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	match, err := TestMediaExclusionRule(updated, "trash/file.jpg", "STATIC_IMAGE")
	if err != nil || !match.Matched || match.Decision != "EXCLUDE" || match.MatchedValue != "trash" {
		t.Fatalf("match=%#v err=%v", match, err)
	}
	if err := db.MediaExclusionRules().Delete(ctx, specific.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MediaExclusionRules().Find(ctx, specific.ID); err == nil {
		t.Fatal("deleted rule remains readable")
	}
}

func TestMediaExclusionEvaluationRequiresExplicitDecisionAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	runID, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, observation := range []ScanObservation{scanPendingPhoto("keep/a.jpg", "a"), scanPendingPhoto("discard/b.jpg", "b")} {
		if err := db.Scans().Stage(ctx, runID, observation); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().Commit(ctx, runID, now); err != nil {
		t.Fatal(err)
	}
	rule, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{
		Name: "discard folders", Enabled: true, Order: 10, Subject: mediaexclusion.SubjectParentFolder,
		Operator: mediaexclusion.OperatorExact, Pattern: "discard", MediaKind: mediaexclusion.MediaKindStatic, Decision: mediaexclusion.DecisionExclude,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := db.MediaExclusionRules().Preview(ctx, rule, nil)
	if err != nil || preview.TotalMatches != 1 || len(preview.Samples) != 1 || preview.Samples[0].RelativePath != "discard/b.jpg" {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	evaluation, err := db.MediaExclusionRules().EvaluateExisting(ctx, nil, now.Add(2*time.Minute))
	if err != nil || evaluation.Evaluated != 2 || evaluation.Matched != 1 || evaluation.Pending != 1 {
		t.Fatalf("evaluation=%#v err=%v", evaluation, err)
	}
	items := loadGalleryItemsForTest(t, db, created.ID)
	var discardID int64
	for _, item := range items {
		if item.RelativePath == "discard/b.jpg" {
			discardID = item.ID
			if item.Excluded {
				t.Fatal("evaluation silently excluded existing media")
			}
		}
	}
	decisions, err := db.MediaExclusionRules().Decisions(ctx, nil, "PENDING")
	if err != nil || len(decisions) != 1 || decisions[0].MatchedValue != "discard" {
		t.Fatalf("pending decisions=%#v err=%v", decisions, err)
	}
	applied, err := db.MediaExclusionRules().ResolveDecision(ctx, decisions[0].ID, true, created.MetadataRevision, now.Add(3*time.Minute))
	if err != nil || applied.Status != "APPLIED" {
		t.Fatalf("applied=%#v err=%v", applied, err)
	}
	item, err := db.Galleries().FindItem(ctx, discardID)
	if err != nil || !item.Excluded {
		t.Fatalf("excluded item=%#v err=%v", item, err)
	}
	var jobStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM processing_jobs WHERE item_uuid=?`, item.UUID).Scan(&jobStatus); err != nil || jobStatus != "CANCELLED" {
		t.Fatalf("excluded item job status=%q err=%v", jobStatus, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyManifestExclusions(ctx, tx, created.ID, map[string]any{}, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM processing_jobs WHERE item_uuid=?`, item.UUID).Scan(&jobStatus); err != nil || jobStatus != "PENDING" {
		t.Fatalf("restored item job status=%q err=%v", jobStatus, err)
	}
	reversed, err := db.MediaExclusionRules().Decisions(ctx, nil, "REVERSED")
	if err != nil || len(reversed) != 1 || reversed[0].ID != applied.ID {
		t.Fatalf("reversed decisions=%#v err=%v", reversed, err)
	}
	if err := db.MediaExclusionRules().Delete(ctx, rule.ID, now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var retainedRuleID sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT rule_id FROM media_exclusion_decisions WHERE id=?`, applied.ID).Scan(&retainedRuleID); err != nil {
		t.Fatal(err)
	}
	if retainedRuleID.Valid {
		t.Fatalf("deleted rule reference=%d, want NULL history snapshot", retainedRuleID.Int64)
	}
}

func scanPendingPhoto(relativePath, fingerprintValue string) ScanObservation {
	value := scanPhoto(relativePath, fingerprintValue)
	value.ProcessingState = "PENDING"
	return value
}

func TestMediaExclusionPreviewCanBeScopedToLibrary(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 28, 11, 0, 0, 0, time.UTC)
	root := t.TempDir()
	library, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Scoped", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Scoped gallery"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &library.ID, Type: "DIRECTORY", Path: filepath.Join(root, "gallery"), Availability: "AVAILABLE",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	runID, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Stage(ctx, runID, scanPhoto("tmp/a.jpg", "scoped")); err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Commit(ctx, runID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MediaExclusionRules().Create(ctx, MediaExclusionRule{LibraryID: &library.ID, Name: "keep tmp", Enabled: true, Order: 100,
		Subject: mediaexclusion.SubjectParentFolder, Operator: mediaexclusion.OperatorExact, Pattern: "tmp",
		MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionInclude}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	rule := MediaExclusionRule{Name: "tmp", Enabled: true, Order: 100, Subject: mediaexclusion.SubjectParentFolder,
		Operator: mediaexclusion.OperatorExact, Pattern: "tmp", MediaKind: mediaexclusion.MediaKindAll, Decision: mediaexclusion.DecisionExclude}
	preview, err := db.MediaExclusionRules().Preview(ctx, rule, &library.ID)
	if err != nil || preview.TotalMatches != 0 {
		t.Fatalf("same-order library INCLUDE did not win preview=%#v err=%v", preview, err)
	}
	rule.Order = 99
	preview, err = db.MediaExclusionRules().Preview(ctx, rule, &library.ID)
	if err != nil || preview.TotalMatches != 1 {
		t.Fatalf("scoped preview=%#v err=%v", preview, err)
	}
	if preview.Samples[0].WinningRuleName != "tmp" || preview.Samples[0].ProposedDecision != "EXCLUDE" {
		t.Fatalf("effective preview winner=%#v", preview.Samples[0])
	}
}
