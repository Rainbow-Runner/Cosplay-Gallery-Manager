package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/manage"
)

var ErrIdentitySuggestionNotPending = errors.New("identity suggestion is not pending")
var ErrIdentitySuggestionNotLinked = errors.New("confirm a matching saved relation before accepting this suggestion")

type identityQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func identityEntityTable(kind string) (table, aliases, owner string, err error) {
	switch kind {
	case "COSER":
		return "cosers", "coser_aliases", "coser_uuid", nil
	case "WORK":
		return "works", "work_aliases", "work_uuid", nil
	case "CHARACTER":
		return "characters", "character_aliases", "character_uuid", nil
	default:
		return "", "", "", fmt.Errorf("unsupported identity suggestion kind %q", kind)
	}
}

// Only currently saved relations count: Work exists through a saved Character.
func linkedIdentityUUIDs(ctx context.Context, q identityQueryer, galleryID int64, kind string) (map[string]bool, error) {
	var query string
	switch kind {
	case "COSER":
		query = `SELECT coser_uuid FROM gallery_credits WHERE gallery_id=?`
	case "WORK":
		query = `SELECT DISTINCT character.work_uuid FROM gallery_cast cast_item
			JOIN characters character ON character.uuid=cast_item.character_uuid WHERE cast_item.gallery_id=?`
	case "CHARACTER":
		query = `SELECT DISTINCT character_uuid FROM gallery_cast WHERE gallery_id=?`
	default:
		return nil, fmt.Errorf("unsupported identity suggestion kind %q", kind)
	}
	rows, err := q.QueryContext(ctx, query, galleryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	linked := make(map[string]bool)
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, err
		}
		linked[uuid] = true
	}
	return linked, rows.Err()
}

func matchingLinkedIdentityUUIDs(ctx context.Context, q identityQueryer, galleryID int64, kind, value string) ([]string, []string, error) {
	table, aliases, owner, err := identityEntityTable(kind)
	if err != nil {
		return nil, nil, err
	}
	if normalizedKey(value) == "" {
		return nil, nil, nil
	}
	allMatches, err := exactEntityUUIDs(ctx, q, table, aliases, owner, value, "")
	if err != nil {
		return nil, nil, err
	}
	linked, err := linkedIdentityUUIDs(ctx, q, galleryID, kind)
	if err != nil {
		return nil, nil, err
	}
	var saved []string
	for _, uuid := range allMatches {
		if linked[uuid] {
			saved = append(saved, uuid)
		}
	}
	sort.Strings(saved)
	return allMatches, saved, nil
}

// Called in the same transaction as an explicit manual relation save. A
// unique global exact/alias match is necessary; a coincidental relation or a
// duplicated name is never enough to silently consume review evidence.
func reconcileSavedIdentitySuggestions(ctx context.Context, tx *sql.Tx, galleryID int64, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,suggestion_kind,value FROM gallery_identity_suggestions
		WHERE gallery_id=? AND status='PENDING' ORDER BY id`, galleryID)
	if err != nil {
		return err
	}
	type suggestion struct {
		id          int64
		kind, value string
	}
	var pending []suggestion
	for rows.Next() {
		var item suggestion
		if err := rows.Scan(&item.id, &item.kind, &item.value); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range pending {
		all, saved, err := matchingLinkedIdentityUUIDs(ctx, tx, galleryID, item.kind, item.value)
		if err != nil {
			return err
		}
		if len(all) != 1 || len(saved) != 1 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_identity_suggestions
			SET status='ACCEPTED',resolved_at_utc=? WHERE id=? AND gallery_id=? AND status='PENDING'`,
			formatTime(normalisedTime(now)), item.id, galleryID); err != nil {
			return err
		}
	}
	return nil
}

// Explicit owner review is separate from relation editing. It never invents
// Credits, Cast or Works, and does not dirty the portable Gallery Manifest.
func (s *GalleryStore) ResolveIdentitySuggestion(ctx context.Context, galleryID, suggestionID, expectedRevision int64, accept bool, entityUUID string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := findGallery(ctx, tx, galleryID)
	if err != nil {
		return err
	}
	if current.MetadataRevision != expectedRevision {
		return ErrMetadataRevisionConflict
	}
	var kind, value, status string
	if err := tx.QueryRowContext(ctx, `SELECT suggestion_kind,value,status FROM gallery_identity_suggestions
		WHERE id=? AND gallery_id=?`, suggestionID, galleryID).Scan(&kind, &value, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrIdentitySuggestionNotPending
		}
		return err
	}
	if status != "PENDING" {
		return ErrIdentitySuggestionNotPending
	}
	decision := "REJECTED"
	if accept {
		_, saved, err := matchingLinkedIdentityUUIDs(ctx, tx, galleryID, kind, value)
		if err != nil {
			return err
		}
		found := false
		for _, uuid := range saved {
			found = found || uuid == entityUUID
		}
		if !found {
			return ErrIdentitySuggestionNotLinked
		}
		decision = "ACCEPTED"
	} else if entityUUID != "" {
		return errors.New("rejecting an identity suggestion must not specify an entity")
	}
	result, err := tx.ExecContext(ctx, `UPDATE gallery_identity_suggestions
		SET status=?,resolved_at_utc=? WHERE id=? AND gallery_id=? AND status='PENDING'`,
		decision, formatTime(normalisedTime(now)), suggestionID, galleryID)
	if err != nil {
		return err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return ErrIdentitySuggestionNotPending
	}
	return tx.Commit()
}

func (s *ManageStore) galleryReview(ctx context.Context, galleryID int64) (manage.GalleryReview, error) {
	var review manage.GalleryReview
	current, err := findGallery(ctx, s.db, galleryID)
	if err != nil {
		return review, err
	}
	facts, err := activationFacts(ctx, s.db, galleryID)
	if err != nil {
		return review, err
	}
	for _, blocker := range current.ActivationBlockers(facts) {
		review.Blockers = append(review.Blockers, blocker.Code)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT CAST(id AS TEXT),suggestion_kind,value,status,COALESCE(resolved_at_utc,'')
		FROM gallery_identity_suggestions WHERE gallery_id=? AND (status='PENDING' OR id IN (
			SELECT id FROM gallery_identity_suggestions WHERE gallery_id=? AND status!='PENDING' ORDER BY id DESC LIMIT 20))
		ORDER BY CASE status WHEN 'PENDING' THEN 0 ELSE 1 END,id DESC`, galleryID, galleryID)
	if err != nil {
		return review, err
	}
	for rows.Next() {
		var item manage.GalleryIdentitySuggestion
		if err := rows.Scan(&item.ID, &item.Kind, &item.Value, &item.Status, &item.ResolvedAt); err != nil {
			rows.Close()
			return review, err
		}
		review.IdentitySuggestions = append(review.IdentitySuggestions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return review, err
	}
	if err := rows.Close(); err != nil {
		return review, err
	}
	for i := range review.IdentitySuggestions {
		item := &review.IdentitySuggestions[i]
		if item.Status != "PENDING" {
			continue
		}
		_, saved, err := matchingLinkedIdentityUUIDs(ctx, s.db, galleryID, item.Kind, item.Value)
		if err != nil {
			return review, err
		}
		for _, uuid := range saved {
			nameQuery := `SELECT name FROM cosers WHERE uuid=?`
			switch item.Kind {
			case "WORK":
				nameQuery = `SELECT name FROM works WHERE uuid=?`
			case "CHARACTER":
				nameQuery = `SELECT name FROM characters WHERE uuid=?`
			}
			var name string
			if err := s.db.QueryRowContext(ctx, nameQuery, uuid).Scan(&name); err != nil {
				return review, err
			}
			item.Options = append(item.Options, manage.GalleryIdentityOption{UUID: uuid, Name: name})
		}
	}
	rows, err = s.db.QueryContext(ctx, `SELECT issue.code,issue.severity,issue.message FROM gallery_source_issues issue
		JOIN gallery_sources source ON source.id=issue.source_id WHERE source.gallery_id=? AND issue.resolved_at_utc IS NULL
		ORDER BY CASE issue.severity WHEN 'BLOCKING' THEN 0 ELSE 1 END,issue.id`, galleryID)
	if err != nil {
		return review, err
	}
	for rows.Next() {
		var issue manage.GallerySourceIssue
		if err := rows.Scan(&issue.Code, &issue.Severity, &issue.Message); err != nil {
			rows.Close()
			return review, err
		}
		review.SourceIssues = append(review.SourceIssues, issue)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return review, err
	}
	if err := rows.Close(); err != nil {
		return review, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT CAST(issue.run_id AS TEXT),issue.stage,issue.error_code,run.status,issue.created_at_utc
		FROM library_automation_run_issues issue JOIN library_automation_runs run ON run.id=issue.run_id
		WHERE issue.gallery_id=? ORDER BY issue.id DESC LIMIT 20`, galleryID)
	if err != nil {
		return review, err
	}
	defer rows.Close()
	for rows.Next() {
		var issue manage.GalleryAutomationIssue
		if err := rows.Scan(&issue.RunID, &issue.Stage, &issue.ErrorCode, &issue.RunStatus, &issue.CreatedAt); err != nil {
			return review, err
		}
		review.AutomationIssues = append(review.AutomationIssues, issue)
	}
	return review, rows.Err()
}
