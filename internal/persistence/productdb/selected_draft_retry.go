package productdb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type GalleryDraftRetryQueueResult struct {
	SetID, Title, Outcome, Reason string
	LibraryID, RunID              int64
}

// Queue only the selected identities, grouped by each library's saved policy.
// Revalidate the exact subset in the same transaction as its durable run; an
// empty or stale selection can never fall back to retrying the entire library.
func (s *AutomationStore) EnqueueSelectedDraftRetries(ctx context.Context, setIDs []string, now time.Time) ([]GalleryDraftRetryQueueResult, error) {
	if len(setIDs) == 0 || len(setIDs) > 100 {
		return nil, errors.New("select between 1 and 100 Galleries")
	}
	results := make([]GalleryDraftRetryQueueResult, 0, len(setIDs))
	groups := map[int64][]int{}
	var order []int64
	seen := map[string]bool{}
	for _, setID := range setIDs {
		if seen[setID] {
			continue
		}
		seen[setID] = true
		item := GalleryDraftRetryQueueResult{SetID: setID, Outcome: "SKIPPED"}
		var state string
		var enabled int
		err := s.db.QueryRowContext(ctx, `SELECT g.title,g.state,COALESCE(s.library_id,0),COALESCE(l.enabled,0)
			FROM galleries g LEFT JOIN gallery_sources s ON s.gallery_id=g.id LEFT JOIN media_libraries l ON l.id=s.library_id WHERE g.set_id=?`, setID).Scan(&item.Title, &state, &item.LibraryID, &enabled)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			item.Reason = "GALLERY_NOT_FOUND"
		case err != nil:
			return nil, err
		case state != "DRAFT":
			item.Reason = "NOT_DRAFT"
		case item.LibraryID == 0:
			item.Reason = "LIBRARY_REQUIRED"
		case enabled != 1:
			item.Reason = "LIBRARY_DISABLED"
		default:
			if _, ok := groups[item.LibraryID]; !ok {
				order = append(order, item.LibraryID)
			}
			groups[item.LibraryID] = append(groups[item.LibraryID], len(results))
		}
		results = append(results, item)
	}
	for _, libraryID := range order {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		indexes := groups[libraryID]
		policy, err := s.FindPolicy(ctx, libraryID)
		reason := ""
		if err != nil {
			reason = "POLICY_READ_FAILED"
		} else if policy.Revision <= 0 || policy.Mode == AutomationManual {
			reason = "SAVED_AUTOMATION_POLICY_REQUIRED"
		}
		if reason == "" {
			active, findErr := s.FindActiveRun(ctx, libraryID)
			if findErr != nil {
				reason = "QUEUE_FAILED"
			} else if active != nil {
				reason = "AUTOMATION_ACTIVE"
			}
		}
		var run AutomationRun
		if reason == "" {
			selected := make([]string, 0, len(indexes))
			for _, i := range indexes {
				selected = append(selected, results[i].SetID)
			}
			run, err = s.enqueueRun(ctx, policy, true, selected, now)
			if errors.Is(err, ErrDraftSelectionChanged) {
				reason = "SELECTION_CHANGED"
			} else if err != nil {
				reason = "QUEUE_FAILED"
			}
		}
		for _, i := range indexes {
			if reason != "" {
				results[i].Reason = reason
				continue
			}
			results[i].Outcome = "QUEUED"
			results[i].RunID = run.ID
		}
	}
	return results, nil
}
