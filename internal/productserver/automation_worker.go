package productserver

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/stashapp/stash/internal/persistence/productdb"
)

func (s *Server) runAutomationWorkerLoop(ctx context.Context) {
	const owner = "local-library-automation"
	const lease = 30 * time.Minute
	for ctx.Err() == nil {
		run, done, err := s.RunAutomationBatchOnce(ctx, owner, lease, 25, time.Now())
		if errors.Is(err, productdb.ErrAutomationRunNotClaimable) {
			if !waitAutomationWorker(ctx, 500*time.Millisecond) {
				return
			}
			continue
		}
		if err != nil {
			if ctx.Err() == nil {
				if run.ID > 0 {
					slog.Error("CGM_LIBRARY_AUTOMATION_FAILED", "run_id", run.ID, "error_code", run.ErrorCode)
				} else {
					slog.Error("CGM_LIBRARY_AUTOMATION_CLAIM_FAILED")
				}
				_ = waitAutomationWorker(ctx, time.Second)
			}
			continue
		}
		if ctx.Err() != nil {
			_ = s.Database.Automation().ReleaseRun(context.WithoutCancel(ctx), run.ID, owner, time.Now())
			return
		}
		if done {
			switch run.Status {
			case "COMPLETED":
				slog.Info("CGM_LIBRARY_AUTOMATION_COMPLETED", "run_id", run.ID, "candidates", run.CandidatesSeen,
					"drafts", run.DraftsCreated, "scanned", run.Scanned, "activated", run.Activated,
					"needs_review", run.NeedsReview)
			case "CANCELLED":
				slog.Info("CGM_LIBRARY_AUTOMATION_CANCELLED", "run_id", run.ID, "candidates", run.CandidatesSeen,
					"drafts", run.DraftsCreated, "scanned", run.Scanned, "activated", run.Activated,
					"needs_review", run.NeedsReview)
			case "FAILED":
				slog.Error("CGM_LIBRARY_AUTOMATION_FAILED", "run_id", run.ID, "error_code", run.ErrorCode)
			default:
				slog.Error("CGM_LIBRARY_AUTOMATION_TERMINAL_STATE_INVALID", "run_id", run.ID)
			}
		}
	}
}

// RunAutomationBatchOnce is deterministic for startup recovery and tests. A
// single server mutex keeps scheduled source reconciliation and automation
// from scanning the same source concurrently.
func (s *Server) RunAutomationBatchOnce(ctx context.Context, owner string, lease time.Duration, batchSize int, now time.Time) (productdb.AutomationRun, bool, error) {
	run, err := s.Database.Automation().ClaimNextRun(ctx, owner, lease, now)
	if err != nil {
		return productdb.AutomationRun{}, false, err
	}
	s.operationMu.Lock()
	updated, done, err := s.Database.Automation().ProcessClaimedRunBatch(ctx, run.ID, owner, batchSize, lease, now)
	s.operationMu.Unlock()
	if err != nil && updated.Status == "RUNNING" {
		_ = s.Database.Automation().ReleaseRun(context.WithoutCancel(ctx), run.ID, owner, time.Now())
	}
	return updated, done, err
}

func waitAutomationWorker(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
