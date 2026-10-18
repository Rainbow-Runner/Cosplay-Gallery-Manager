package productdb

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
)

// Refresh strong name evidence only. No stat, WalkDir, archive open or media job.
// Existing evidence (including unresolved evidence) is retained. Any explicit
// rejection conservatively prevents regeneration, including alias resurrection.
func (s *AutomationStore) refreshDraftIdentitySuggestions(ctx context.Context, galleryID int64, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state string
	var rejected int
	if err := tx.QueryRowContext(ctx, `SELECT state,(SELECT COUNT(*) FROM gallery_identity_suggestions WHERE gallery_id=? AND status='REJECTED') FROM galleries WHERE id=?`, galleryID, galleryID).Scan(&state, &rejected); err != nil {
		return err
	}
	if state != "DRAFT" || rejected > 0 {
		return tx.Commit()
	}
	var root, path, kind string
	if err := tx.QueryRowContext(ctx, `SELECT l.root_path,s.source_path,s.source_type FROM gallery_sources s JOIN media_libraries l ON l.id=s.library_id WHERE s.gallery_id=? ORDER BY s.id LIMIT 1`, galleryID).Scan(&root, &path, &kind); err != nil {
		return err
	}
	// The archive parser also handles path components; prevent extension stripping
	// of dotted directory names without changing the established inference rules.
	if kind == "DIRECTORY" {
		path += ".zip"
	}
	suggestions, err := archiveEntitySuggestions(ctx, tx, root, path)
	if err != nil {
		return err
	}
	for _, suggestion := range suggestions {
		kind := strings.ToUpper(suggestion.Field)
		if kind != "COSER" && kind != "WORK" && kind != "CHARACTER" {
			continue
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO gallery_identity_suggestions(gallery_id,suggestion_kind,value,status,created_at_utc)
			SELECT ?,?,?,'PENDING',? WHERE NOT EXISTS(SELECT 1 FROM gallery_identity_suggestions WHERE gallery_id=? AND suggestion_kind=? AND value=?)`, galleryID, kind, suggestion.Value, formatTime(normalisedTime(now)), galleryID, kind, suggestion.Value)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Never replace credits, Cast IDs, positions or tags. Only fill an empty Cast
// for an already credited, identically inferred Coser. Existing Cast remains an
// owner decision; differing suggestions remain pending for review.
func appendMissingDraftCastTx(ctx context.Context, tx *sql.Tx, value gallery.Gallery, input ReplaceGalleryRelationsInput, now time.Time) error {
	// An accepted role that is no longer linked may have been removed manually.
	// Do not recreate it through a newly added alias or infer a new ownership.
	rows, err := tx.QueryContext(ctx, `SELECT value FROM gallery_identity_suggestions WHERE gallery_id=? AND suggestion_kind='CHARACTER' AND status='ACCEPTED'`, value.ID)
	if err != nil {
		return err
	}
	var reviewed []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		reviewed = append(reviewed, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, name := range reviewed {
		all, saved, err := matchingLinkedIdentityUUIDs(ctx, tx, value.ID, "CHARACTER", name)
		if err != nil {
			return err
		}
		if len(all) != 1 || len(saved) != 1 {
			return nil
		}
	}
	linked, err := linkedIdentityUUIDs(ctx, tx, value.ID, "COSER")
	if err != nil {
		return err
	}
	if len(linked) != len(input.Credits) {
		return nil
	}
	for _, credit := range input.Credits {
		if err := requireActivePortableKind(ctx, tx, credit.CoserUUID, portableid.KindCoser); err != nil {
			return err
		}
		if !linked[credit.CoserUUID] {
			return nil
		}
	}
	changed := false
	for _, credit := range input.Credits {
		var id, count int64
		if err := tx.QueryRowContext(ctx, `SELECT id,(SELECT COUNT(*) FROM gallery_cast WHERE gallery_credit_id=c.id) FROM gallery_credits c WHERE gallery_id=? AND coser_uuid=?`, value.ID, credit.CoserUUID).Scan(&id, &count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		for _, cast := range credit.Cast {
			if err := requireActivePortableKind(ctx, tx, cast.CharacterUUID, portableid.KindCharacter); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO gallery_cast(gallery_id,gallery_credit_id,character_uuid,position) VALUES(?,?,?,?)`, value.ID, id, cast.CharacterUUID, cast.Position); err != nil {
				return err
			}
			changed = true
		}
	}
	if changed {
		if err := touchGalleryMetadata(ctx, tx, value.ID, value.MetadataRevision, now); err != nil {
			return err
		}
	}
	return reconcileSavedIdentitySuggestions(ctx, tx, value.ID, now)
}

type DraftRetryResult struct{ SetID, Title, Outcome, Reason string }

// Read-only, bounded result history; use gallery list / automation issues for
// larger reviews. Pending targets of cancelled runs stay explicitly pending.
func (s *AutomationStore) DraftRetryResults(ctx context.Context, libraryID int64) ([]DraftRetryResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.set_id,t.title,t.outcome,t.reason FROM library_draft_retry_targets t JOIN galleries g ON g.id=t.gallery_id WHERE t.run_id=(SELECT MAX(id) FROM library_automation_runs WHERE library_id=? AND retry_drafts=1) ORDER BY t.gallery_id LIMIT 1000`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DraftRetryResult{}
	for rows.Next() {
		var v DraftRetryResult
		if err := rows.Scan(&v.SetID, &v.Title, &v.Outcome, &v.Reason); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
