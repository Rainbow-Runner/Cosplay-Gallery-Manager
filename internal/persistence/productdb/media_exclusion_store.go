package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/mediaexclusion"
)

const (
	maxMediaExclusionRules   = 500
	maxMediaExclusionPreview = 200
)

type MediaExclusionRule = mediaexclusion.Rule

type MediaExclusionRuleStore struct{ db *sql.DB }

func (db *Database) MediaExclusionRules() *MediaExclusionRuleStore {
	return &MediaExclusionRuleStore{db: db.DB}
}

func (s *MediaExclusionRuleStore) List(ctx context.Context, libraryID *int64) ([]MediaExclusionRule, error) {
	query := `SELECT id,library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,media_kind,decision,revision,system_default
		FROM media_exclusion_rules`
	args := []any{}
	if libraryID != nil {
		query += ` WHERE library_id IS NULL OR library_id=?`
		args = append(args, *libraryID)
	} else {
		query += ` WHERE library_id IS NULL`
	}
	query += ` ORDER BY sort_order,CASE WHEN library_id IS NULL THEN 1 ELSE 0 END,id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MediaExclusionRule
	for rows.Next() {
		value, err := scanMediaExclusionRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *MediaExclusionRuleStore) Create(ctx context.Context, input MediaExclusionRule, now time.Time) (MediaExclusionRule, error) {
	input.ID, input.Revision, input.SystemDefault = 0, 1, false
	if err := mediaexclusion.Validate(input); err != nil {
		return MediaExclusionRule{}, err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_exclusion_rules`).Scan(&count); err != nil {
		return MediaExclusionRule{}, err
	}
	if count >= maxMediaExclusionRules {
		return MediaExclusionRule{}, errors.New("media exclusion rule limit reached")
	}
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `INSERT INTO media_exclusion_rules
		(library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,media_kind,decision,revision,system_default,created_at_utc,updated_at_utc)
		VALUES (?,?,?,?,?,?,?,?,?,?,1,0,?,?)`, input.LibraryID, input.Name, input.Enabled, input.Order, input.Subject,
		input.Operator, input.Pattern, input.CaseSensitive, input.MediaKind, input.Decision, timestamp, timestamp)
	if err != nil {
		return MediaExclusionRule{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return MediaExclusionRule{}, err
	}
	return s.Find(ctx, id)
}

func (s *MediaExclusionRuleStore) Update(ctx context.Context, input MediaExclusionRule, now time.Time) (MediaExclusionRule, error) {
	if input.ID <= 0 {
		return MediaExclusionRule{}, errors.New("media exclusion rule id is required")
	}
	if err := mediaexclusion.Validate(input); err != nil {
		return MediaExclusionRule{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaExclusionRule{}, err
	}
	defer func() { _ = tx.Rollback() }()
	timestamp := formatTime(normalisedTime(now))
	result, err := tx.ExecContext(ctx, `UPDATE media_exclusion_rules SET library_id=?,name=?,enabled=?,sort_order=?,match_subject=?,match_operator=?,pattern=?,case_sensitive=?,media_kind=?,decision=?,revision=revision+1,updated_at_utc=? WHERE id=?`,
		input.LibraryID, input.Name, input.Enabled, input.Order, input.Subject, input.Operator, input.Pattern,
		input.CaseSensitive, input.MediaKind, input.Decision, timestamp, input.ID)
	if err != nil {
		return MediaExclusionRule{}, err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return MediaExclusionRule{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_exclusion_decisions SET status='SUPERSEDED',resolved_at_utc=? WHERE rule_id=? AND status='PENDING'`, timestamp, input.ID); err != nil {
		return MediaExclusionRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaExclusionRule{}, err
	}
	return s.Find(ctx, input.ID)
}

func (s *MediaExclusionRuleStore) Delete(ctx context.Context, id int64, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `UPDATE media_exclusion_decisions SET status='SUPERSEDED',resolved_at_utc=? WHERE rule_id=? AND status='PENDING'`, timestamp, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM media_exclusion_rules WHERE id=?`, id)
	if err != nil {
		return err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *MediaExclusionRuleStore) Find(ctx context.Context, id int64) (MediaExclusionRule, error) {
	return scanMediaExclusionRule(s.db.QueryRowContext(ctx, `SELECT id,library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,media_kind,decision,revision,system_default FROM media_exclusion_rules WHERE id=?`, id))
}

type mediaExclusionRuleScanner interface{ Scan(...any) error }

func scanMediaExclusionRule(row mediaExclusionRuleScanner) (MediaExclusionRule, error) {
	var result MediaExclusionRule
	var library sql.NullInt64
	if err := row.Scan(&result.ID, &library, &result.Name, &result.Enabled, &result.Order, &result.Subject, &result.Operator,
		&result.Pattern, &result.CaseSensitive, &result.MediaKind, &result.Decision, &result.Revision, &result.SystemDefault); err != nil {
		return MediaExclusionRule{}, err
	}
	if library.Valid {
		result.LibraryID = &library.Int64
	}
	return result, nil
}

type MediaExclusionMatch struct {
	Matched      bool
	RuleID       int64
	RuleName     string
	Decision     string
	Subject      string
	MatchedValue string
}

func TestMediaExclusionRule(rule MediaExclusionRule, relativePath, mediaKind string) (MediaExclusionMatch, error) {
	compiled, err := mediaexclusion.Compile(rule)
	if err != nil {
		return MediaExclusionMatch{}, err
	}
	matched, value := compiled.Match(relativePath, mediaKind)
	return MediaExclusionMatch{Matched: matched, RuleID: rule.ID, RuleName: rule.Name, Decision: string(rule.Decision), Subject: string(rule.Subject), MatchedValue: value}, nil
}

type MediaExclusionPreviewSample struct {
	GallerySetID, GalleryTitle, ItemUUID, RelativePath string
	MediaKind                                          string
	CurrentlyExcluded                                  bool
	ProposedDecision, WinningRuleName, MatchedValue    string
}

type MediaExclusionPreview struct {
	TotalMatches int
	Samples      []MediaExclusionPreviewSample
}

func (s *MediaExclusionRuleStore) Preview(ctx context.Context, rule MediaExclusionRule, libraryID *int64) (MediaExclusionPreview, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaExclusionPreview{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if rule.ID <= 0 {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0)+1 FROM media_exclusion_rules`).Scan(&rule.ID); err != nil {
			return MediaExclusionPreview{}, err
		}
	}
	draft, err := mediaexclusion.Compile(rule)
	if err != nil {
		return MediaExclusionPreview{}, err
	}
	compiled, err := loadCompiledMediaExclusionRules(ctx, tx, libraryID)
	if err != nil {
		return MediaExclusionPreview{}, err
	}
	effective := compiled[:0]
	for _, candidate := range compiled {
		if candidate.Rule().ID != rule.ID {
			effective = append(effective, candidate)
		}
	}
	if rule.Enabled && mediaExclusionRuleAppliesToLibrary(rule, libraryID) {
		effective = append(effective, draft)
	}
	sort.SliceStable(effective, func(i, j int) bool {
		left, right := effective[i].Rule(), effective[j].Rule()
		if left.Order != right.Order {
			return left.Order < right.Order
		}
		if (left.LibraryID != nil) != (right.LibraryID != nil) {
			return left.LibraryID != nil
		}
		return left.ID < right.ID
	})
	query := `SELECT gallery.set_id,gallery.title,item.item_uuid,item.relative_path,item.media_kind,item.excluded
		FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id JOIN gallery_sources source ON source.id=item.source_id
		WHERE item.availability_state='AVAILABLE'`
	args := []any{}
	if libraryID != nil {
		query += ` AND source.library_id=?`
		args = append(args, *libraryID)
	}
	query += ` ORDER BY gallery.id,item.position`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return MediaExclusionPreview{}, err
	}
	defer rows.Close()
	var result MediaExclusionPreview
	for rows.Next() {
		var sample MediaExclusionPreviewSample
		if err := rows.Scan(&sample.GallerySetID, &sample.GalleryTitle, &sample.ItemUUID, &sample.RelativePath, &sample.MediaKind, &sample.CurrentlyExcluded); err != nil {
			return MediaExclusionPreview{}, err
		}
		winner, value := matchMediaExclusionRules(effective, sample.RelativePath, sample.MediaKind)
		if winner == nil || winner.Rule().ID != rule.ID {
			continue
		}
		result.TotalMatches++
		if len(result.Samples) < maxMediaExclusionPreview {
			sample.ProposedDecision, sample.WinningRuleName, sample.MatchedValue = string(rule.Decision), rule.Name, value
			result.Samples = append(result.Samples, sample)
		}
	}
	return result, rows.Err()
}

func mediaExclusionRuleAppliesToLibrary(rule MediaExclusionRule, libraryID *int64) bool {
	if rule.LibraryID == nil {
		return true
	}
	return libraryID != nil && *rule.LibraryID == *libraryID
}

type MediaExclusionEvaluation struct {
	Evaluated, Matched, Pending, Superseded int
}

func (s *MediaExclusionRuleStore) EvaluateExisting(ctx context.Context, libraryID *int64, now time.Time) (MediaExclusionEvaluation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaExclusionEvaluation{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := evaluateMediaExclusion(ctx, tx, libraryID, now)
	if err != nil {
		return MediaExclusionEvaluation{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaExclusionEvaluation{}, err
	}
	return result, nil
}

func evaluateMediaExclusion(ctx context.Context, tx *sql.Tx, libraryID *int64, now time.Time) (MediaExclusionEvaluation, error) {
	compiled, err := loadCompiledMediaExclusionRules(ctx, tx, libraryID)
	if err != nil {
		return MediaExclusionEvaluation{}, err
	}
	query := `SELECT item.item_uuid,item.gallery_id,item.relative_path,item.media_kind FROM gallery_items item
		JOIN gallery_sources source ON source.id=item.source_id
		WHERE item.availability_state='AVAILABLE' AND item.excluded=0`
	args := []any{}
	if libraryID != nil {
		query += ` AND source.library_id=?`
		args = append(args, *libraryID)
	}
	query += ` ORDER BY item.id`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return MediaExclusionEvaluation{}, err
	}
	type item struct {
		uuid       string
		galleryID  int64
		path, kind string
	}
	var items []item
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.uuid, &value.galleryID, &value.path, &value.kind); err != nil {
			rows.Close()
			return MediaExclusionEvaluation{}, err
		}
		items = append(items, value)
	}
	if err := rows.Close(); err != nil {
		return MediaExclusionEvaluation{}, err
	}
	timestamp := formatTime(normalisedTime(now))
	var result MediaExclusionEvaluation
	for _, item := range items {
		result.Evaluated++
		winner, matchedValue := matchMediaExclusionRules(compiled, item.path, item.kind)
		keepRuleID, keepRevision := int64(0), 0
		if winner != nil && winner.Rule().Decision == mediaexclusion.DecisionExclude {
			result.Matched++
			keepRuleID, keepRevision = winner.Rule().ID, winner.Rule().Revision
		}
		superseded, err := tx.ExecContext(ctx, `UPDATE media_exclusion_decisions SET status='SUPERSEDED',resolved_at_utc=?
			WHERE item_uuid=? AND status='PENDING' AND NOT (rule_id=? AND rule_revision=?)`, timestamp, item.uuid, keepRuleID, keepRevision)
		if err != nil {
			return MediaExclusionEvaluation{}, err
		}
		if count, _ := superseded.RowsAffected(); count > 0 {
			result.Superseded += int(count)
		}
		if winner == nil || winner.Rule().Decision != mediaexclusion.DecisionExclude {
			continue
		}
		rule := winner.Rule()
		insert, err := tx.ExecContext(ctx, `INSERT INTO media_exclusion_decisions
			(gallery_id,item_uuid,rule_id,rule_revision,rule_name,decision,matched_subject,matched_value,status,created_at_utc)
			VALUES (?,?,?,?,?,?,?,?,'PENDING',?) ON CONFLICT(item_uuid,rule_id,rule_revision) DO NOTHING`,
			item.galleryID, item.uuid, rule.ID, rule.Revision, rule.Name, rule.Decision, rule.Subject, matchedValue, timestamp)
		if err != nil {
			return MediaExclusionEvaluation{}, err
		}
		if count, _ := insert.RowsAffected(); count > 0 {
			result.Pending += int(count)
		}
	}
	return result, nil
}

type compiledMediaExclusionRule = mediaexclusion.CompiledRule

func loadCompiledMediaExclusionRules(ctx context.Context, tx *sql.Tx, libraryID *int64) ([]*compiledMediaExclusionRule, error) {
	query := `SELECT id,library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,media_kind,decision,revision,system_default
		FROM media_exclusion_rules WHERE enabled=1 AND library_id IS NULL`
	args := []any{}
	if libraryID != nil {
		query = `SELECT id,library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,media_kind,decision,revision,system_default
			FROM media_exclusion_rules WHERE enabled=1 AND (library_id IS NULL OR library_id=?)`
		args = append(args, *libraryID)
	}
	query += ` ORDER BY sort_order,CASE WHEN library_id IS NULL THEN 1 ELSE 0 END,id`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*compiledMediaExclusionRule
	for rows.Next() {
		rule, err := scanMediaExclusionRule(rows)
		if err != nil {
			return nil, err
		}
		compiled, err := mediaexclusion.Compile(rule)
		if err != nil {
			return nil, fmt.Errorf("compiling media exclusion rule %d: %w", rule.ID, err)
		}
		result = append(result, compiled)
	}
	return result, rows.Err()
}

func matchMediaExclusionRules(rules []*compiledMediaExclusionRule, relativePath, mediaKind string) (*compiledMediaExclusionRule, string) {
	for _, rule := range rules {
		if matched, value := rule.Match(relativePath, mediaKind); matched {
			return rule, value
		}
	}
	return nil, ""
}

type MediaExclusionDecision struct {
	ID, GalleryID, GalleryRevision                     int64
	GallerySetID, GalleryTitle, ItemUUID, RelativePath string
	RuleID                                             int64
	RuleRevision                                       int
	RuleName, Decision, MatchedSubject, MatchedValue   string
	Status                                             string
}

func (s *MediaExclusionRuleStore) Decisions(ctx context.Context, libraryID *int64, status string) ([]MediaExclusionDecision, error) {
	if status == "" {
		status = "PENDING"
	}
	query := `SELECT decision.id,decision.gallery_id,gallery.metadata_revision,gallery.set_id,gallery.title,decision.item_uuid,item.relative_path,
		COALESCE(decision.rule_id,0),decision.rule_revision,decision.rule_name,decision.decision,decision.matched_subject,decision.matched_value,decision.status
		FROM media_exclusion_decisions decision JOIN galleries gallery ON gallery.id=decision.gallery_id
		JOIN gallery_items item ON item.item_uuid=decision.item_uuid JOIN gallery_sources source ON source.id=item.source_id WHERE decision.status=?`
	args := []any{status}
	if libraryID != nil {
		query += ` AND source.library_id=?`
		args = append(args, *libraryID)
	}
	query += ` ORDER BY decision.id LIMIT 500`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MediaExclusionDecision
	for rows.Next() {
		value, err := scanMediaExclusionDecision(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

type mediaExclusionDecisionScanner interface{ Scan(...any) error }

func scanMediaExclusionDecision(row mediaExclusionDecisionScanner) (MediaExclusionDecision, error) {
	var value MediaExclusionDecision
	err := row.Scan(&value.ID, &value.GalleryID, &value.GalleryRevision, &value.GallerySetID, &value.GalleryTitle, &value.ItemUUID,
		&value.RelativePath, &value.RuleID, &value.RuleRevision, &value.RuleName, &value.Decision, &value.MatchedSubject, &value.MatchedValue, &value.Status)
	return value, err
}

func findMediaExclusionDecision(ctx context.Context, queryer galleryQueryer, id int64) (MediaExclusionDecision, error) {
	return scanMediaExclusionDecision(queryer.QueryRowContext(ctx, `SELECT decision.id,decision.gallery_id,gallery.metadata_revision,gallery.set_id,gallery.title,decision.item_uuid,item.relative_path,
		COALESCE(decision.rule_id,0),decision.rule_revision,decision.rule_name,decision.decision,decision.matched_subject,decision.matched_value,decision.status
		FROM media_exclusion_decisions decision JOIN galleries gallery ON gallery.id=decision.gallery_id
		JOIN gallery_items item ON item.item_uuid=decision.item_uuid WHERE decision.id=?`, id))
}

func (s *MediaExclusionRuleStore) ResolveDecision(ctx context.Context, id int64, accept bool, expectedGalleryRevision int64, now time.Time) (MediaExclusionDecision, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaExclusionDecision{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var value MediaExclusionDecision
	err = tx.QueryRowContext(ctx, `SELECT id,gallery_id,item_uuid,decision,status FROM media_exclusion_decisions WHERE id=?`, id).
		Scan(&value.ID, &value.GalleryID, &value.ItemUUID, &value.Decision, &value.Status)
	if err != nil {
		return MediaExclusionDecision{}, err
	}
	if value.Status != "PENDING" {
		return MediaExclusionDecision{}, errors.New("media exclusion decision was already resolved")
	}
	timestamp := formatTime(normalisedTime(now))
	status := "REJECTED"
	if accept {
		if value.Decision != string(mediaexclusion.DecisionExclude) {
			return MediaExclusionDecision{}, errors.New("only EXCLUDE decisions can be applied to existing media")
		}
		status = "APPLIED"
		result, err := tx.ExecContext(ctx, `UPDATE gallery_items SET excluded=1,updated_at_utc=? WHERE item_uuid=? AND excluded=0`, timestamp, value.ItemUUID)
		if err != nil {
			return MediaExclusionDecision{}, err
		}
		if err := requireOneRevisionRow(result); err != nil {
			return MediaExclusionDecision{}, err
		}
		if err := cancelItemProcessingJobs(ctx, tx, value.ItemUUID, now); err != nil {
			return MediaExclusionDecision{}, err
		}
		if err := touchGalleryMetadata(ctx, tx, value.GalleryID, expectedGalleryRevision, now); err != nil {
			return MediaExclusionDecision{}, err
		}
		if err := demoteInvalidActiveGallery(ctx, tx, value.GalleryID); err != nil {
			return MediaExclusionDecision{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_exclusion_decisions SET status=?,resolved_at_utc=? WHERE id=?`, status, timestamp, id); err != nil {
		return MediaExclusionDecision{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaExclusionDecision{}, err
	}
	return findMediaExclusionDecision(ctx, s.db, id)
}

func recordAppliedMediaExclusion(ctx context.Context, tx *sql.Tx, galleryID int64, itemUUID string, rule MediaExclusionRule, matchedValue string, now time.Time) error {
	timestamp := formatTime(normalisedTime(now))
	_, err := tx.ExecContext(ctx, `INSERT INTO media_exclusion_decisions
		(gallery_id,item_uuid,rule_id,rule_revision,rule_name,decision,matched_subject,matched_value,status,created_at_utc,resolved_at_utc)
		VALUES (?,?,?,?,?,?,?,?,'APPLIED',?,?) ON CONFLICT(item_uuid,rule_id,rule_revision) DO NOTHING`,
		galleryID, itemUUID, rule.ID, rule.Revision, rule.Name, rule.Decision, rule.Subject, matchedValue, timestamp, timestamp)
	return err
}
