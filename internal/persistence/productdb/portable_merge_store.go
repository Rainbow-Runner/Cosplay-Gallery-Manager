package productdb

import (
	"context"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"github.com/stashapp/stash/internal/portableid"
)

var portableMergeRelativePathPattern = regexp.MustCompile(`^portable-merges/[0-9a-f-]{36}/package\.zip$`)

type PortableMergeSessionInput struct {
	MergeID, ExportID, PackageSHA256, PackageRelativePath, TargetFingerprint string
	FormatVersion                                                            int
	IdentityAdd, IdentityReuse, EntityAdd, EntityReuse                       int
	Issues                                                                   []PortableMergeConflict
}

type PortableMergeSession struct {
	MergeID, ExportID, PackageSHA256, PackageRelativePath, TargetFingerprint, State string
	HardBlockingCount, ReviewCount                                                  int
	IdentityAddCount, IdentityReuseCount, EntityAddCount, EntityReuseCount          int
	ErrorCode, SafetyBackupID, CreatedAt, UpdatedAt                                 string
}

type PortableMergeConflict struct {
	IssueKey, IssueCode, Severity, EntityKind, IncomingUUID, LocalUUID, FieldKey, Decision string
}

type PortableMergeDecision struct {
	IssueKey string
	Decision string
}

func (db *Database) CreatePortableMergeSession(ctx context.Context, input PortableMergeSessionInput, now time.Time) error {
	if _, err := portableid.Parse(input.MergeID); err != nil {
		return err
	}
	if _, err := portableid.Parse(input.ExportID); err != nil {
		return err
	}
	if len(input.PackageSHA256) != 64 || len(input.TargetFingerprint) != 64 {
		return errors.New("portable merge digest is invalid")
	}
	if _, err := hex.DecodeString(input.PackageSHA256); err != nil {
		return errors.New("portable merge package digest is invalid")
	}
	if _, err := hex.DecodeString(input.TargetFingerprint); err != nil {
		return errors.New("portable merge target fingerprint is invalid")
	}
	if !portableMergeRelativePathPattern.MatchString(input.PackageRelativePath) || input.FormatVersion < 1 || input.IdentityAdd < 0 || input.IdentityReuse < 0 || input.EntityAdd < 0 || input.EntityReuse < 0 {
		return errors.New("portable merge session input is invalid")
	}
	hard, review := 0, 0
	seen := map[string]bool{}
	for _, issue := range input.Issues {
		if len(issue.IssueKey) != 64 || seen[issue.IssueKey] || issue.Decision != "" && issue.Decision != "UNRESOLVED" || len(issue.IssueCode) == 0 || len(issue.IssueCode) > 100 || len(issue.FieldKey) > 100 {
			return errors.New("portable merge conflict is invalid")
		}
		if _, err := hex.DecodeString(issue.IssueKey); err != nil {
			return errors.New("portable merge conflict key is invalid")
		}
		seen[issue.IssueKey] = true
		if _, err := portableid.Parse(issue.IncomingUUID); err != nil {
			return err
		}
		if issue.LocalUUID != "" {
			if _, err := portableid.Parse(issue.LocalUUID); err != nil {
				return err
			}
		}
		switch issue.Severity {
		case "BLOCKING":
			hard++
		case "REVIEW":
			review++
		default:
			return errors.New("portable merge conflict severity is invalid")
		}
	}
	state := "READY"
	if hard > 0 {
		state = "BLOCKED"
	} else if review > 0 {
		state = "DECISIONS_PENDING"
	}
	timestamp := formatTime(normalisedTime(now))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO portable_merge_sessions(
		merge_id,export_id,package_sha256,package_relative_path,target_fingerprint,format_version,state,
		hard_blocking_count,review_count,identity_add_count,identity_reuse_count,entity_add_count,entity_reuse_count,created_at_utc,updated_at_utc)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, input.MergeID, input.ExportID, input.PackageSHA256, input.PackageRelativePath, input.TargetFingerprint, input.FormatVersion, state,
		hard, review, input.IdentityAdd, input.IdentityReuse, input.EntityAdd, input.EntityReuse, timestamp, timestamp); err != nil {
		return err
	}
	for _, issue := range input.Issues {
		if _, err := tx.ExecContext(ctx, `INSERT INTO portable_merge_conflicts(merge_id,issue_key,issue_code,severity,entity_kind,incoming_uuid,local_uuid,field_key,updated_at_utc)
			VALUES(?,?,?,?,?,?,?,?,?)`, input.MergeID, issue.IssueKey, issue.IssueCode, issue.Severity, issue.EntityKind, issue.IncomingUUID, issue.LocalUUID, issue.FieldKey, timestamp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *Database) FindPortableMergeSession(ctx context.Context, mergeID string) (PortableMergeSession, error) {
	var value PortableMergeSession
	err := db.QueryRowContext(ctx, `SELECT merge_id,export_id,package_sha256,package_relative_path,target_fingerprint,state,hard_blocking_count,review_count,identity_add_count,identity_reuse_count,entity_add_count,entity_reuse_count,error_code,safety_backup_id,created_at_utc,updated_at_utc
		FROM portable_merge_sessions WHERE merge_id=?`, mergeID).Scan(&value.MergeID, &value.ExportID, &value.PackageSHA256, &value.PackageRelativePath, &value.TargetFingerprint, &value.State, &value.HardBlockingCount, &value.ReviewCount, &value.IdentityAddCount, &value.IdentityReuseCount, &value.EntityAddCount, &value.EntityReuseCount, &value.ErrorCode, &value.SafetyBackupID, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func (db *Database) ListPortableMergeSessions(ctx context.Context) ([]PortableMergeSession, error) {
	rows, err := db.QueryContext(ctx, `SELECT merge_id,export_id,package_sha256,package_relative_path,target_fingerprint,state,hard_blocking_count,review_count,identity_add_count,identity_reuse_count,entity_add_count,entity_reuse_count,error_code,safety_backup_id,created_at_utc,updated_at_utc FROM portable_merge_sessions ORDER BY created_at_utc DESC,merge_id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PortableMergeSession{}
	for rows.Next() {
		var value PortableMergeSession
		if err := rows.Scan(&value.MergeID, &value.ExportID, &value.PackageSHA256, &value.PackageRelativePath, &value.TargetFingerprint, &value.State, &value.HardBlockingCount, &value.ReviewCount, &value.IdentityAddCount, &value.IdentityReuseCount, &value.EntityAddCount, &value.EntityReuseCount, &value.ErrorCode, &value.SafetyBackupID, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (db *Database) ListPortableMergeConflicts(ctx context.Context, mergeID string) ([]PortableMergeConflict, error) {
	rows, err := db.QueryContext(ctx, `SELECT issue_key,issue_code,severity,entity_kind,incoming_uuid,local_uuid,field_key,decision
		FROM portable_merge_conflicts WHERE merge_id=? ORDER BY severity,issue_key`, mergeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PortableMergeConflict
	for rows.Next() {
		var value PortableMergeConflict
		if err := rows.Scan(&value.IssueKey, &value.IssueCode, &value.Severity, &value.EntityKind, &value.IncomingUUID, &value.LocalUUID, &value.FieldKey, &value.Decision); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (db *Database) SetPortableMergeDecisions(ctx context.Context, mergeID, expectedFingerprint string, decisions []PortableMergeDecision, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state, fingerprint string
	var reviewCount int
	if err := tx.QueryRowContext(ctx, `SELECT state,target_fingerprint,review_count FROM portable_merge_sessions WHERE merge_id=?`, mergeID).Scan(&state, &fingerprint, &reviewCount); err != nil {
		return err
	}
	if state != "DECISIONS_PENDING" || fingerprint != expectedFingerprint || len(decisions) != reviewCount {
		return errors.New("portable merge session is not ready for this complete decision set")
	}
	seen := map[string]bool{}
	timestamp := formatTime(normalisedTime(now))
	for _, decision := range decisions {
		if seen[decision.IssueKey] {
			return errors.New("portable merge decision is duplicated")
		}
		seen[decision.IssueKey] = true
		var code, severity string
		if err := tx.QueryRowContext(ctx, `SELECT issue_code,severity FROM portable_merge_conflicts WHERE merge_id=? AND issue_key=?`, mergeID, decision.IssueKey).Scan(&code, &severity); err != nil {
			return err
		}
		if severity != "REVIEW" || !portableMergeDecisionAllowed(code, decision.Decision) {
			return errors.New("portable merge decision is not allowed for this conflict")
		}
		result, err := tx.ExecContext(ctx, `UPDATE portable_merge_conflicts SET decision=?,updated_at_utc=? WHERE merge_id=? AND issue_key=? AND decision='UNRESOLVED'`, decision.Decision, timestamp, mergeID, decision.IssueKey)
		if err != nil {
			return err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return errors.New("portable merge conflict is no longer unresolved")
		}
	}
	var unresolved int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_merge_conflicts WHERE merge_id=? AND decision='UNRESOLVED'`, mergeID).Scan(&unresolved); err != nil {
		return err
	}
	if unresolved != 0 {
		return errors.New("portable merge decision set is incomplete")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE portable_merge_sessions SET state='READY',updated_at_utc=? WHERE merge_id=? AND state='DECISIONS_PENDING'`, timestamp, mergeID); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *Database) MarkPortableMergeStale(ctx context.Context, mergeID string, now time.Time) error {
	result, err := db.ExecContext(ctx, `UPDATE portable_merge_sessions SET state='STALE',error_code='PORTABLE_MERGE_TARGET_CHANGED',updated_at_utc=?
		WHERE merge_id=? AND state IN ('DECISIONS_PENDING','READY')`, formatTime(normalisedTime(now)), mergeID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable merge session cannot be marked stale")
	}
	return nil
}

func (db *Database) AbortPortableMerge(ctx context.Context, mergeID string, now time.Time) error {
	result, err := db.ExecContext(ctx, `UPDATE portable_merge_sessions SET state='ABORTED',error_code='',updated_at_utc=?
		WHERE merge_id=? AND state IN ('BLOCKED','DECISIONS_PENDING','READY','STALE','FAILED')
		AND NOT EXISTS(SELECT 1 FROM portable_import_sessions WHERE import_id=?)`, formatTime(normalisedTime(now)), mergeID, mergeID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable merge can no longer be aborted")
	}
	return nil
}

func portableMergeDecisionAllowed(code, decision string) bool {
	switch code {
	case "PORTABLE_CORE_ENTITY_CONTENT_CONFLICT", "PORTABLE_TAG_EDGE_POSITION_CONFLICT", "PORTABLE_TAG_EDGE_SLOT_CONFLICT", "PORTABLE_SOCIAL_ACCOUNT_POSITION_CONFLICT":
		return decision == "KEEP_LOCAL" || decision == "USE_INCOMING"
	case "PORTABLE_CORE_NAME_MATCH_REVIEW", "PORTABLE_SOCIAL_ACCOUNT_URL_REVIEW":
		return decision == "KEEP_SEPARATE" || decision == "MAP_TO_LOCAL"
	default:
		return false
	}
}
