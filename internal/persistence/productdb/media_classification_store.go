package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stashapp/stash/internal/mediaclassification"
)

const (
	maxMediaClassificationRules   = 500
	maxMediaClassificationPreview = 200
)

type MediaClassificationRule = mediaclassification.Rule

type MediaClassificationRuleStore struct{ db *sql.DB }

func (db *Database) MediaClassificationRules() *MediaClassificationRuleStore {
	return &MediaClassificationRuleStore{db: db.DB}
}

func (s *MediaClassificationRuleStore) List(ctx context.Context, libraryID *int64) ([]MediaClassificationRule, error) {
	query := `SELECT id,library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,result_category,revision,system_default
		FROM media_classification_rules`
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
	var result []MediaClassificationRule
	for rows.Next() {
		value, err := scanMediaClassificationRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *MediaClassificationRuleStore) Create(ctx context.Context, input MediaClassificationRule, now time.Time) (MediaClassificationRule, error) {
	input.ID, input.Revision, input.SystemDefault = 0, 1, false
	if err := mediaclassification.Validate(input); err != nil {
		return MediaClassificationRule{}, err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_classification_rules`).Scan(&count); err != nil {
		return MediaClassificationRule{}, err
	}
	if count >= maxMediaClassificationRules {
		return MediaClassificationRule{}, errors.New("media classification rule limit reached")
	}
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `INSERT INTO media_classification_rules
		(library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,result_category,revision,system_default,created_at_utc,updated_at_utc)
		VALUES (?,?,?,?,?,?,?,?,?,1,0,?,?)`, input.LibraryID, input.Name, input.Enabled, input.Order, input.Subject, input.Operator, input.Pattern, input.CaseSensitive, input.Category, timestamp, timestamp)
	if err != nil {
		return MediaClassificationRule{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return MediaClassificationRule{}, err
	}
	return s.Find(ctx, id)
}

func (s *MediaClassificationRuleStore) Update(ctx context.Context, input MediaClassificationRule, now time.Time) (MediaClassificationRule, error) {
	if input.ID <= 0 {
		return MediaClassificationRule{}, errors.New("media classification rule id is required")
	}
	if err := mediaclassification.Validate(input); err != nil {
		return MediaClassificationRule{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaClassificationRule{}, err
	}
	defer func() { _ = tx.Rollback() }()
	timestamp := formatTime(normalisedTime(now))
	result, err := tx.ExecContext(ctx, `UPDATE media_classification_rules SET library_id=?,name=?,enabled=?,sort_order=?,match_subject=?,match_operator=?,pattern=?,case_sensitive=?,result_category=?,revision=revision+1,updated_at_utc=? WHERE id=?`,
		input.LibraryID, input.Name, input.Enabled, input.Order, input.Subject, input.Operator, input.Pattern, input.CaseSensitive, input.Category, timestamp, input.ID)
	if err != nil {
		return MediaClassificationRule{}, err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return MediaClassificationRule{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_classification_suggestions SET status='SUPERSEDED',resolved_at_utc=? WHERE rule_id=? AND status='PENDING'`, timestamp, input.ID); err != nil {
		return MediaClassificationRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaClassificationRule{}, err
	}
	return s.Find(ctx, input.ID)
}

func (s *MediaClassificationRuleStore) Delete(ctx context.Context, id int64, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE media_classification_suggestions SET status='SUPERSEDED',resolved_at_utc=? WHERE rule_id=? AND status='PENDING'`, formatTime(normalisedTime(now)), id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM media_classification_rules WHERE id=?`, id)
	if err != nil {
		return err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *MediaClassificationRuleStore) RestoreDefaults(ctx context.Context, now time.Time) ([]MediaClassificationRule, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `UPDATE media_classification_suggestions SET status='SUPERSEDED',resolved_at_utc=? WHERE status='PENDING' AND rule_id IN (SELECT id FROM media_classification_rules WHERE default_key IS NOT NULL)`, timestamp); err != nil {
		return nil, err
	}
	defaults := []struct {
		key, name, subject, operator, pattern, category string
		enabled                                         bool
		order                                           int
	}{
		{"DEFAULT_FOLDER", "Default selfie folders", "PARENT_FOLDER", "EXACT", "selfie\nselfies\nself-portrait\n自拍\n自拍照\n自拍写真\n自撮り\nセルフィー\n셀카", "SELFIE", true, 100},
		{"OPTIONAL_FILENAME", "Optional selfie filenames", "FILE_STEM", "GLOB", "selfie_*\n*_selfie\n自拍_*\n*_自拍", "SELFIE", false, 200},
	}
	for _, value := range defaults {
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_classification_rules
			(library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,result_category,revision,system_default,default_key,created_at_utc,updated_at_utc)
			VALUES(NULL,?,?,?,?,?,?,0,?,1,1,?,?,?)
			ON CONFLICT(default_key) DO UPDATE SET library_id=NULL,name=excluded.name,enabled=excluded.enabled,sort_order=excluded.sort_order,
			match_subject=excluded.match_subject,match_operator=excluded.match_operator,pattern=excluded.pattern,case_sensitive=0,
			result_category=excluded.result_category,revision=media_classification_rules.revision+1,system_default=1,updated_at_utc=excluded.updated_at_utc`,
			value.name, value.enabled, value.order, value.subject, value.operator, value.pattern, value.category, value.key, timestamp, timestamp); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.List(ctx, nil)
}

func (s *MediaClassificationRuleStore) Find(ctx context.Context, id int64) (MediaClassificationRule, error) {
	return scanMediaClassificationRule(s.db.QueryRowContext(ctx, `SELECT id,library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,result_category,revision,system_default FROM media_classification_rules WHERE id=?`, id))
}

type MediaClassificationMatch struct {
	Matched      bool
	RuleID       int64
	RuleName     string
	Category     string
	Subject      string
	MatchedValue string
}

func TestMediaClassificationRule(rule MediaClassificationRule, relativePath string) (MediaClassificationMatch, error) {
	compiled, err := mediaclassification.Compile(rule)
	if err != nil {
		return MediaClassificationMatch{}, err
	}
	matched, value := compiled.Match(relativePath)
	return MediaClassificationMatch{Matched: matched, RuleID: rule.ID, RuleName: rule.Name, Category: string(rule.Category), Subject: string(rule.Subject), MatchedValue: value}, nil
}

type MediaClassificationPreviewSample struct {
	GallerySetID     string
	GalleryTitle     string
	ItemUUID         string
	RelativePath     string
	CurrentCategory  string
	ProposedCategory string
	MatchedValue     string
}

type MediaClassificationPreview struct {
	TotalMatches int
	Samples      []MediaClassificationPreviewSample
}

func (s *MediaClassificationRuleStore) Preview(ctx context.Context, rule MediaClassificationRule, libraryID *int64) (MediaClassificationPreview, error) {
	compiled, err := mediaclassification.Compile(rule)
	if err != nil {
		return MediaClassificationPreview{}, err
	}
	query := `SELECT gallery.set_id,gallery.title,item.item_uuid,item.relative_path,item.image_category
		FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id JOIN gallery_sources source ON source.id=item.source_id
		WHERE item.media_kind='STATIC_IMAGE' AND item.availability_state='AVAILABLE'`
	args := []any{}
	if libraryID != nil {
		query += ` AND source.library_id=?`
		args = append(args, *libraryID)
	}
	query += ` ORDER BY gallery.id,item.position`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return MediaClassificationPreview{}, err
	}
	defer rows.Close()
	var preview MediaClassificationPreview
	for rows.Next() {
		var sample MediaClassificationPreviewSample
		if err := rows.Scan(&sample.GallerySetID, &sample.GalleryTitle, &sample.ItemUUID, &sample.RelativePath, &sample.CurrentCategory); err != nil {
			return MediaClassificationPreview{}, err
		}
		matched, value := compiled.Match(sample.RelativePath)
		if !matched {
			continue
		}
		preview.TotalMatches++
		if len(preview.Samples) < maxMediaClassificationPreview {
			sample.ProposedCategory, sample.MatchedValue = string(rule.Category), value
			preview.Samples = append(preview.Samples, sample)
		}
	}
	return preview, rows.Err()
}

type MediaClassificationEvaluation struct {
	Evaluated  int
	Matched    int
	Pending    int
	Superseded int
}

func (s *MediaClassificationRuleStore) EvaluateExisting(ctx context.Context, libraryID *int64, now time.Time) (MediaClassificationEvaluation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaClassificationEvaluation{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := evaluateMediaClassification(ctx, tx, libraryID, nil, now)
	if err != nil {
		return MediaClassificationEvaluation{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaClassificationEvaluation{}, err
	}
	return result, nil
}

func evaluateMediaClassificationForSource(ctx context.Context, tx *sql.Tx, sourceID int64, now time.Time) error {
	_, err := evaluateMediaClassification(ctx, tx, nil, &sourceID, now)
	return err
}

func evaluateMediaClassification(ctx context.Context, tx *sql.Tx, requestedLibraryID, sourceID *int64, now time.Time) (MediaClassificationEvaluation, error) {
	var sourceLibraryID sql.NullInt64
	if sourceID != nil {
		if err := tx.QueryRowContext(ctx, `SELECT library_id FROM gallery_sources WHERE id=?`, *sourceID).Scan(&sourceLibraryID); err != nil {
			return MediaClassificationEvaluation{}, err
		}
	}
	libraryID := requestedLibraryID
	if sourceID != nil && sourceLibraryID.Valid {
		value := sourceLibraryID.Int64
		libraryID = &value
	}
	rules, err := loadEffectiveMediaClassificationRules(ctx, tx, libraryID)
	if err != nil {
		return MediaClassificationEvaluation{}, err
	}
	compiled := make([]*mediaclassification.CompiledRule, 0, len(rules))
	for _, rule := range rules {
		value, err := mediaclassification.Compile(rule)
		if err != nil {
			return MediaClassificationEvaluation{}, fmt.Errorf("compiling media classification rule %d: %w", rule.ID, err)
		}
		compiled = append(compiled, value)
	}
	query := `SELECT item.item_uuid,item.gallery_id,item.relative_path,item.image_category FROM gallery_items item JOIN gallery_sources source ON source.id=item.source_id
		WHERE item.media_kind='STATIC_IMAGE' AND item.availability_state='AVAILABLE'`
	args := []any{}
	if sourceID != nil {
		query += ` AND item.source_id=?`
		args = append(args, *sourceID)
	} else if requestedLibraryID != nil {
		query += ` AND source.library_id=?`
		args = append(args, *requestedLibraryID)
	}
	query += ` ORDER BY item.id`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return MediaClassificationEvaluation{}, err
	}
	type item struct {
		uuid           string
		galleryID      int64
		path, category string
	}
	var items []item
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.uuid, &value.galleryID, &value.path, &value.category); err != nil {
			rows.Close()
			return MediaClassificationEvaluation{}, err
		}
		items = append(items, value)
	}
	if err := rows.Close(); err != nil {
		return MediaClassificationEvaluation{}, err
	}
	timestamp := formatTime(normalisedTime(now))
	var result MediaClassificationEvaluation
	for _, item := range items {
		result.Evaluated++
		var winner *mediaclassification.CompiledRule
		matchedValue := ""
		for _, rule := range compiled {
			if ok, value := rule.Match(item.path); ok {
				winner, matchedValue = rule, value
				break
			}
		}
		keepRuleID, keepRevision := int64(0), 0
		if winner != nil {
			result.Matched++
			keepRuleID, keepRevision = winner.Rule().ID, winner.Rule().Revision
		}
		superseded, err := tx.ExecContext(ctx, `UPDATE media_classification_suggestions SET status='SUPERSEDED',resolved_at_utc=?
			WHERE item_uuid=? AND status='PENDING' AND NOT (rule_id=? AND rule_revision=?)`, timestamp, item.uuid, keepRuleID, keepRevision)
		if err != nil {
			return MediaClassificationEvaluation{}, err
		}
		if count, _ := superseded.RowsAffected(); count > 0 {
			result.Superseded += int(count)
		}
		if winner == nil || item.category == string(winner.Rule().Category) {
			continue
		}
		insert, err := tx.ExecContext(ctx, `INSERT INTO media_classification_suggestions
			(gallery_id,item_uuid,rule_id,rule_revision,rule_name,proposed_category,matched_subject,matched_value,status,created_at_utc)
			VALUES (?,?,?,?,?,?,?,?, 'PENDING',?) ON CONFLICT(item_uuid,rule_id,rule_revision) DO NOTHING`,
			item.galleryID, item.uuid, winner.Rule().ID, winner.Rule().Revision, winner.Rule().Name, winner.Rule().Category, winner.Rule().Subject, matchedValue, timestamp)
		if err != nil {
			return MediaClassificationEvaluation{}, err
		}
		if count, _ := insert.RowsAffected(); count > 0 {
			result.Pending += int(count)
		}
	}
	return result, nil
}

func loadEffectiveMediaClassificationRules(ctx context.Context, queryer *sql.Tx, libraryID *int64) ([]MediaClassificationRule, error) {
	query := `SELECT id,library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,result_category,revision,system_default
		FROM media_classification_rules WHERE enabled=1 AND library_id IS NULL`
	args := []any{}
	if libraryID != nil {
		query = `SELECT id,library_id,name,enabled,sort_order,match_subject,match_operator,pattern,case_sensitive,result_category,revision,system_default
			FROM media_classification_rules WHERE enabled=1 AND (library_id IS NULL OR library_id=?)`
		args = append(args, *libraryID)
	}
	query += ` ORDER BY sort_order,CASE WHEN library_id IS NULL THEN 1 ELSE 0 END,id`
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MediaClassificationRule
	for rows.Next() {
		value, err := scanMediaClassificationRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

type mediaClassificationRuleScanner interface{ Scan(...any) error }

func scanMediaClassificationRule(row mediaClassificationRuleScanner) (MediaClassificationRule, error) {
	var result MediaClassificationRule
	var libraryID sql.NullInt64
	if err := row.Scan(&result.ID, &libraryID, &result.Name, &result.Enabled, &result.Order, &result.Subject, &result.Operator, &result.Pattern, &result.CaseSensitive, &result.Category, &result.Revision, &result.SystemDefault); err != nil {
		return MediaClassificationRule{}, err
	}
	if libraryID.Valid {
		result.LibraryID = &libraryID.Int64
	}
	return result, nil
}

type MediaClassificationSuggestion struct {
	ID, GalleryID, GalleryRevision                                   int64
	GallerySetID, GalleryTitle, ItemUUID, RelativePath               string
	RuleID                                                           int64
	RuleRevision                                                     int
	RuleName, ProposedCategory, MatchedSubject, MatchedValue, Status string
}

func (s *MediaClassificationRuleStore) Suggestions(ctx context.Context, libraryID *int64, status string) ([]MediaClassificationSuggestion, error) {
	if status == "" {
		status = "PENDING"
	}
	query := `SELECT suggestion.id,suggestion.gallery_id,gallery.metadata_revision,gallery.set_id,gallery.title,suggestion.item_uuid,item.relative_path,
		COALESCE(suggestion.rule_id,0),suggestion.rule_revision,suggestion.rule_name,suggestion.proposed_category,suggestion.matched_subject,suggestion.matched_value,suggestion.status
		FROM media_classification_suggestions suggestion JOIN galleries gallery ON gallery.id=suggestion.gallery_id
		JOIN gallery_items item ON item.item_uuid=suggestion.item_uuid JOIN gallery_sources source ON source.id=item.source_id WHERE suggestion.status=?`
	args := []any{status}
	if libraryID != nil {
		query += ` AND source.library_id=?`
		args = append(args, *libraryID)
	}
	query += ` ORDER BY suggestion.id LIMIT 500`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MediaClassificationSuggestion
	for rows.Next() {
		var value MediaClassificationSuggestion
		if err := rows.Scan(&value.ID, &value.GalleryID, &value.GalleryRevision, &value.GallerySetID, &value.GalleryTitle, &value.ItemUUID, &value.RelativePath, &value.RuleID, &value.RuleRevision, &value.RuleName, &value.ProposedCategory, &value.MatchedSubject, &value.MatchedValue, &value.Status); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func findMediaClassificationSuggestion(ctx context.Context, queryer galleryQueryer, id int64) (MediaClassificationSuggestion, error) {
	var value MediaClassificationSuggestion
	err := queryer.QueryRowContext(ctx, `SELECT suggestion.id,suggestion.gallery_id,gallery.metadata_revision,gallery.set_id,gallery.title,suggestion.item_uuid,item.relative_path,
		COALESCE(suggestion.rule_id,0),suggestion.rule_revision,suggestion.rule_name,suggestion.proposed_category,suggestion.matched_subject,suggestion.matched_value,suggestion.status
		FROM media_classification_suggestions suggestion JOIN galleries gallery ON gallery.id=suggestion.gallery_id
		JOIN gallery_items item ON item.item_uuid=suggestion.item_uuid WHERE suggestion.id=?`, id).Scan(
		&value.ID, &value.GalleryID, &value.GalleryRevision, &value.GallerySetID, &value.GalleryTitle, &value.ItemUUID, &value.RelativePath,
		&value.RuleID, &value.RuleRevision, &value.RuleName, &value.ProposedCategory, &value.MatchedSubject, &value.MatchedValue, &value.Status,
	)
	return value, err
}

func (s *MediaClassificationRuleStore) ResolveSuggestion(ctx context.Context, id int64, accept bool, expectedGalleryRevision int64, now time.Time) (MediaClassificationSuggestion, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaClassificationSuggestion{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var value MediaClassificationSuggestion
	err = tx.QueryRowContext(ctx, `SELECT id,gallery_id,item_uuid,proposed_category,status FROM media_classification_suggestions WHERE id=?`, id).Scan(&value.ID, &value.GalleryID, &value.ItemUUID, &value.ProposedCategory, &value.Status)
	if err != nil {
		return MediaClassificationSuggestion{}, err
	}
	if value.Status != "PENDING" {
		return MediaClassificationSuggestion{}, errors.New("media classification suggestion was already resolved")
	}
	timestamp := formatTime(normalisedTime(now))
	status := "REJECTED"
	if accept {
		status = "ACCEPTED"
		result, err := tx.ExecContext(ctx, `UPDATE gallery_items SET image_category=?,updated_at_utc=? WHERE item_uuid=? AND media_kind='STATIC_IMAGE'`, value.ProposedCategory, timestamp, value.ItemUUID)
		if err != nil {
			return MediaClassificationSuggestion{}, err
		}
		if err := requireOneRevisionRow(result); err != nil {
			return MediaClassificationSuggestion{}, err
		}
		if err := touchGalleryCoverMetadata(ctx, tx, value.GalleryID, &expectedGalleryRevision, now); err != nil {
			return MediaClassificationSuggestion{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_classification_suggestions SET status=?,resolved_at_utc=? WHERE id=?`, status, timestamp, id); err != nil {
		return MediaClassificationSuggestion{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaClassificationSuggestion{}, err
	}
	return findMediaClassificationSuggestion(ctx, s.db, id)
}
