package productdb

import (
	"context"
	"os"
	"testing"
	"time"
)

const automationPerformanceDraftCount = 10_000

// TestAutomationPerformanceGate is opt-in because it measures the complete
// persistent queue lifecycle for 10,000 Draft Galleries. The fixture keeps
// sources IN_SYNC so this gate isolates batching, cursor persistence, policy
// application and lease churn from filesystem and decoder throughput.
func TestAutomationPerformanceGate(t *testing.T) {
	if os.Getenv("CGM_AUTOMATION_GATE") != "1" {
		t.Skip("set CGM_AUTOMATION_GATE=1 to run the 10,000 Draft automation gate")
	}
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 8, 30, 3, 0, 0, 0, time.UTC)
	library := createTestLibrary(t, db, now)
	fixtureStarted := time.Now()
	insertAutomationDraftFixture(t, db, library.ID, library.RootPath, automationPerformanceDraftCount)
	fixtureDuration := time.Since(fixtureStarted)
	policy, err := db.Automation().SavePolicy(ctx, LibraryAutomationPolicy{LibraryID: library.ID, Mode: AutomationAssisted}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := db.Automation().EnqueueRun(ctx, policy, now)
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	batches := 0
	for {
		owner := "automation-performance-worker"
		claimed, err := db.Automation().ClaimNextRun(ctx, owner, time.Minute, now.Add(time.Duration(batches)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if batches == 0 {
			claimed.DiscoveryCompleted = true
			if _, err := db.Automation().SaveRunProgress(ctx, claimed, owner, now); err != nil {
				t.Fatal(err)
			}
		}
		finished, done, err := db.Automation().ProcessClaimedRunBatch(ctx, claimed.ID, owner, 25, time.Minute, now.Add(time.Duration(batches)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		batches++
		if done {
			if finished.ID != queued.ID || finished.Status != "COMPLETED" || finished.CursorGalleryID != automationPerformanceDraftCount ||
				finished.NeedsReview != automationPerformanceDraftCount || finished.Scanned != 0 || finished.IssueCount != 0 {
				t.Fatalf("final automation run = %#v", finished)
			}
			break
		}
	}
	duration := time.Since(started)
	wantBatches := automationPerformanceDraftCount / 25
	if batches != wantBatches {
		t.Fatalf("batches=%d want=%d", batches, wantBatches)
	}
	if duration > 45*time.Second {
		t.Fatalf("10,000 Draft automation=%s target<=45s", duration)
	}
	t.Logf("automation drafts=%d batches=%d fixture=%s processing=%s target<=45s",
		automationPerformanceDraftCount, batches, fixtureDuration.Round(time.Millisecond), duration.Round(time.Millisecond))
}
