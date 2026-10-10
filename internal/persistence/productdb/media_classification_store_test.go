package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/mediaclassification"
)

func TestMediaClassificationRuleValidationRevisionAndPriority(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	invalid := MediaClassificationRule{Name: "bad", Enabled: true, Subject: mediaclassification.SubjectFileName, Operator: mediaclassification.OperatorRE2, Pattern: "([", Category: mediaclassification.CategorySelfie}
	if _, err := db.MediaClassificationRules().Create(ctx, invalid, now); err == nil {
		t.Fatal("invalid RE2 was persisted")
	}
	var invalidCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_classification_rules WHERE name='bad'`).Scan(&invalidCount); err != nil || invalidCount != 0 {
		t.Fatalf("invalid rule count=%d err=%v", invalidCount, err)
	}

	created, err := db.MediaClassificationRules().Create(ctx, MediaClassificationRule{
		Name: "explicit photo", Enabled: true, Order: 50, Subject: mediaclassification.SubjectParentFolder,
		Operator: mediaclassification.OperatorExact, Pattern: "not-selfie", Category: mediaclassification.CategoryPhoto,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	created.Pattern = "ordinary"
	updated, err := db.MediaClassificationRules().Update(ctx, created, now.Add(time.Minute))
	if err != nil || updated.Revision != 2 {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	match, err := TestMediaClassificationRule(updated, "ordinary/a.jpg")
	if err != nil || !match.Matched || match.Category != "PHOTO" {
		t.Fatalf("match=%#v err=%v", match, err)
	}
}

func TestMediaClassificationDefaultsCanBeDeletedAndRestored(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 17, 10, 30, 0, 0, time.UTC)
	rules, err := db.MediaClassificationRules().List(ctx, nil)
	if err != nil || len(rules) != 2 {
		t.Fatalf("defaults=%#v err=%v", rules, err)
	}
	if err := db.MediaClassificationRules().Delete(ctx, rules[0].ID, now); err != nil {
		t.Fatal(err)
	}
	restored, err := db.MediaClassificationRules().RestoreDefaults(ctx, now)
	if err != nil || len(restored) != 2 {
		t.Fatalf("restored=%#v err=%v", restored, err)
	}
	for _, rule := range restored {
		if !rule.SystemDefault {
			t.Fatalf("restored non-default=%#v", rule)
		}
	}
}

func TestMediaClassificationOnlySuggestsStaticAndKeepsRejectedRevision(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 17, 11, 0, 0, 0, time.UTC)
	created, source := createEmptySourceFixture(t, db, now)
	runID, err := db.Scans().Begin(ctx, source.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []ScanObservation{scanPhoto("selfie/photo.jpg", "p"), scanAnimated("selfie/animated.gif", "a"), scanVideo("selfie/video.mp4", "v")} {
		if err := db.Scans().Stage(ctx, runID, item); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().Commit(ctx, runID, now); err != nil {
		t.Fatal(err)
	}
	suggestions, err := db.MediaClassificationRules().Suggestions(ctx, nil, "PENDING")
	if err != nil || len(suggestions) != 1 || suggestions[0].RelativePath != "selfie/photo.jpg" {
		t.Fatalf("suggestions=%#v err=%v", suggestions, err)
	}
	rejected, err := db.MediaClassificationRules().ResolveSuggestion(ctx, suggestions[0].ID, false, created.MetadataRevision, now)
	if err != nil || rejected.Status != "REJECTED" {
		t.Fatalf("rejected=%#v err=%v", rejected, err)
	}
	evaluated, err := db.MediaClassificationRules().EvaluateExisting(ctx, nil, now.Add(time.Minute))
	if err != nil || evaluated.Pending != 0 {
		t.Fatalf("evaluation=%#v err=%v", evaluated, err)
	}
	pending, err := db.MediaClassificationRules().Suggestions(ctx, nil, "PENDING")
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending=%#v err=%v", pending, err)
	}

	rules, err := db.MediaClassificationRules().List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules[0].Pattern += "\nportrait"
	if _, err := db.MediaClassificationRules().Update(ctx, rules[0], now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	evaluated, err = db.MediaClassificationRules().EvaluateExisting(ctx, nil, now.Add(3*time.Minute))
	if err != nil || evaluated.Pending != 1 {
		t.Fatalf("new revision evaluation=%#v err=%v", evaluated, err)
	}
	rules, err = db.MediaClassificationRules().List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules[0].Pattern += "\nself-portrait"
	if _, err := db.MediaClassificationRules().Update(ctx, rules[0], now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	pending, err = db.MediaClassificationRules().Suggestions(ctx, nil, "PENDING")
	if err != nil || len(pending) != 0 {
		t.Fatalf("rule update retained stale pending suggestions=%#v err=%v", pending, err)
	}
}
