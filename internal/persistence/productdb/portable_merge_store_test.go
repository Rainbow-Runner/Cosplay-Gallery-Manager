package productdb

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPortableMergeDecisionsAreCompleteTypedAndImmutable(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 9, 17, 0, 0, 0, time.UTC)
	mergeID := "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"
	issues := []PortableMergeConflict{
		{IssueKey: strings.Repeat("1", 64), IssueCode: "PORTABLE_CORE_ENTITY_CONTENT_CONFLICT", Severity: "REVIEW", EntityKind: "WORK", IncomingUUID: "11111111-1111-4111-8111-111111111111", LocalUUID: "11111111-1111-4111-8111-111111111111"},
		{IssueKey: strings.Repeat("2", 64), IssueCode: "PORTABLE_CORE_NAME_MATCH_REVIEW", Severity: "REVIEW", EntityKind: "COSER", IncomingUUID: "22222222-2222-4222-8222-222222222222", LocalUUID: "33333333-3333-4333-8333-333333333333"},
	}
	input := PortableMergeSessionInput{MergeID: mergeID, ExportID: "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb", PackageSHA256: strings.Repeat("a", 64), PackageRelativePath: "portable-merges/" + mergeID + "/package.zip", TargetFingerprint: strings.Repeat("b", 64), FormatVersion: 1, IdentityAdd: 2, IdentityReuse: 1, EntityAdd: 1, EntityReuse: 1, Issues: issues}
	if err := db.CreatePortableMergeSession(ctx, input, now); err != nil {
		t.Fatal(err)
	}
	session, err := db.FindPortableMergeSession(ctx, mergeID)
	if err != nil || session.State != "DECISIONS_PENDING" || session.ReviewCount != 2 {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	if err := db.SetPortableMergeDecisions(ctx, mergeID, input.TargetFingerprint, []PortableMergeDecision{{IssueKey: issues[0].IssueKey, Decision: "KEEP_LOCAL"}}, now); err == nil {
		t.Fatal("incomplete decision set was accepted")
	}
	if err := db.SetPortableMergeDecisions(ctx, mergeID, input.TargetFingerprint, []PortableMergeDecision{{IssueKey: issues[0].IssueKey, Decision: "MAP_TO_LOCAL"}, {IssueKey: issues[1].IssueKey, Decision: "MAP_TO_LOCAL"}}, now); err == nil {
		t.Fatal("decision incompatible with conflict type was accepted")
	}
	if err := db.SetPortableMergeDecisions(ctx, mergeID, input.TargetFingerprint, []PortableMergeDecision{{IssueKey: issues[0].IssueKey, Decision: "KEEP_LOCAL"}, {IssueKey: issues[1].IssueKey, Decision: "MAP_TO_LOCAL"}}, now); err != nil {
		t.Fatal(err)
	}
	session, _ = db.FindPortableMergeSession(ctx, mergeID)
	if session.State != "READY" {
		t.Fatalf("session state=%q", session.State)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM portable_merge_conflicts WHERE merge_id=?`, mergeID); err == nil {
		t.Fatal("merge conflicts were deletable")
	}
	if _, err := db.ExecContext(ctx, `UPDATE portable_merge_conflicts SET issue_code='CHANGED' WHERE merge_id=?`, mergeID); err == nil {
		t.Fatal("merge conflict identity was mutable")
	}
}

func TestPortableMergeHardConflictCannotReceiveDecision(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mergeID := "cccccccc-3333-4333-8333-cccccccccccc"
	issue := PortableMergeConflict{IssueKey: strings.Repeat("3", 64), IssueCode: "PORTABLE_UUID_KIND_CONFLICT", Severity: "BLOCKING", EntityKind: "TAG", IncomingUUID: "44444444-4444-4444-8444-444444444444", LocalUUID: "44444444-4444-4444-8444-444444444444"}
	input := PortableMergeSessionInput{MergeID: mergeID, ExportID: "dddddddd-4444-4444-8444-dddddddddddd", PackageSHA256: strings.Repeat("c", 64), PackageRelativePath: "portable-merges/" + mergeID + "/package.zip", TargetFingerprint: strings.Repeat("d", 64), FormatVersion: 1, Issues: []PortableMergeConflict{issue}}
	if err := db.CreatePortableMergeSession(ctx, input, time.Now()); err != nil {
		t.Fatal(err)
	}
	session, _ := db.FindPortableMergeSession(ctx, mergeID)
	if session.State != "BLOCKED" {
		t.Fatalf("hard-conflict state=%q", session.State)
	}
	if err := db.SetPortableMergeDecisions(ctx, mergeID, input.TargetFingerprint, []PortableMergeDecision{{IssueKey: issue.IssueKey, Decision: "KEEP_LOCAL"}}, time.Now()); err == nil {
		t.Fatal("hard conflict accepted a decision")
	}
}

func TestConflictFreePortableMergeRollsBackAllCoreWritesOnLateConstraint(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 9, 20, 0, 0, 0, time.UTC)
	localWork, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Local redirect owner"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO slug_redirects(entity_kind,old_slug,target_uuid,created_at_utc) VALUES('WORK','old-work',?,?)`, localWork.UUID, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	bundle, identities := portableCoreImportFixture()
	mergeID := "eeeeeeee-5555-4555-8555-eeeeeeeeeeee"
	input := PortableMergeSessionInput{MergeID: mergeID, ExportID: "ffffffff-6666-4666-8666-ffffffffffff", PackageSHA256: strings.Repeat("e", 64), PackageRelativePath: "portable-merges/" + mergeID + "/package.zip", TargetFingerprint: strings.Repeat("f", 64), FormatVersion: 1, IdentityAdd: len(identities)}
	if err := db.CreatePortableMergeSession(ctx, input, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MergePortableCoreConflictFree(ctx, mergeID, input.TargetFingerprint, "aaaaaaaa-7777-4777-8777-aaaaaaaaaaaa", bundle, sliceIdentityStream(identities), now); err == nil {
		t.Fatal("late slug constraint did not fail merge")
	}
	var incomingRegistry int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_uuid_registry WHERE uuid=?`, identities[0].UUID).Scan(&incomingRegistry); err != nil || incomingRegistry != 0 {
		t.Fatalf("failed merge retained incoming Registry=%d err=%v", incomingRegistry, err)
	}
	session, err := db.FindPortableMergeSession(ctx, mergeID)
	if err != nil || session.State != "READY" {
		t.Fatalf("failed merge session=%+v err=%v", session, err)
	}
}
