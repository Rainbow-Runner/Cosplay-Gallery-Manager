package productdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"time"
)

type MaintenanceMode string

const (
	MaintenanceNormal            MaintenanceMode = "NORMAL"
	MaintenanceRestoring         MaintenanceMode = "RESTORING"
	MaintenanceWaitingValidation MaintenanceMode = "WAITING_VALIDATION"
)

type MaintenanceState struct {
	Mode            MaintenanceMode
	RestoreBackupID string
	LastErrorCode   string
	UpdatedAtUTC    time.Time
}

type StorageRoots struct {
	CoserMetadataRoot string
	BackupRoot        string
}

type AuditEvent struct {
	ID           int64
	EventCode    string
	TargetKind   string
	TargetID     string
	Outcome      string
	ErrorCode    string
	SummaryJSON  string
	CreatedAtUTC time.Time
}

type AuditPage struct {
	Items      []AuditEvent
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}

type OperationsStore struct{ db *sql.DB }

func (db *Database) Operations() *OperationsStore { return &OperationsStore{db: db.DB} }

var ErrScheduledOperationNotDue = errors.New("scheduled operation is not due or is already leased")

func (s *OperationsStore) StorageRoots(ctx context.Context) (StorageRoots, error) {
	var result StorageRoots
	err := s.db.QueryRowContext(ctx, `SELECT coser_metadata_root,backup_root FROM product_setup WHERE id=1 AND complete=1`).Scan(&result.CoserMetadataRoot, &result.BackupRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return StorageRoots{}, errors.New("product Setup is incomplete")
	}
	return result, err
}

func (s *OperationsStore) Maintenance(ctx context.Context) (MaintenanceState, error) {
	var result MaintenanceState
	var updated string
	err := s.db.QueryRowContext(ctx, `SELECT state,COALESCE(restore_backup_id,''),last_error_code,updated_at_utc FROM maintenance_state WHERE id=1`).Scan(&result.Mode, &result.RestoreBackupID, &result.LastErrorCode, &updated)
	if err != nil {
		return MaintenanceState{}, err
	}
	result.UpdatedAtUTC, err = parseTime(updated)
	return result, err
}

func (s *OperationsStore) SetMaintenance(ctx context.Context, mode MaintenanceMode, restoreBackupID, errorCode string, now time.Time) error {
	if mode != MaintenanceNormal && mode != MaintenanceRestoring && mode != MaintenanceWaitingValidation || len(errorCode) > 100 {
		return errors.New("invalid maintenance state")
	}
	if mode == MaintenanceNormal {
		restoreBackupID = ""
	}
	_, err := s.db.ExecContext(ctx, `UPDATE maintenance_state SET state=?,restore_backup_id=NULLIF(?,''),last_error_code=?,updated_at_utc=? WHERE id=1`, mode, restoreBackupID, errorCode, formatTime(normalisedTime(now)))
	return err
}

// FinalizeRestore invalidates state copied from the backup before normal
// handlers or workers are allowed to operate on the restored database.
func (s *OperationsStore) FinalizeRestore(ctx context.Context, backupID string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `UPDATE owner_sessions SET revoked_at_utc=? WHERE revoked_at_utc IS NULL`, timestamp); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE processing_jobs SET status='CANCELLED',lease_owner=NULL,lease_expires_at_utc=NULL,last_heartbeat_at_utc=NULL,updated_at_utc=? WHERE status IN ('PENDING','RUNNING','RETRY_WAIT','PAUSED')`, timestamp); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE scheduled_operations SET lease_owner=NULL,lease_expires_at_utc=NULL,last_error_code='RESTORE_CANCELLED',updated_at_utc=? WHERE lease_owner IS NOT NULL`, timestamp); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runtime_settings SET settings_revision=settings_revision+1,automatic_schedules_suspended=1,updated_at_utc=? WHERE id=1`, timestamp); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE maintenance_state SET state='WAITING_VALIDATION',restore_backup_id=?,last_error_code='',updated_at_utc=? WHERE id=1`, backupID, timestamp); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE backup_records SET status='READY',completed_at_utc=COALESCE(completed_at_utc,?),last_error_code='' WHERE backup_id=?`, timestamp, backupID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *OperationsStore) ResumeAfterValidation(ctx context.Context, now time.Time) error {
	timestamp := formatTime(normalisedTime(now))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE maintenance_state SET state='NORMAL',restore_backup_id=NULL,last_error_code='',updated_at_utc=? WHERE id=1 AND state='WAITING_VALIDATION'`, timestamp)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return errors.New("maintenance validation is not pending")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runtime_settings SET settings_revision=settings_revision+1,automatic_schedules_suspended=0,updated_at_utc=? WHERE id=1`, timestamp); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *OperationsStore) Audit(ctx context.Context, eventCode, targetKind, targetID, outcome, errorCode string, summary map[string]any, now time.Time) error {
	if eventCode == "" || len(eventCode) > 100 || len(targetKind) > 50 || len(targetID) > 100 || len(errorCode) > 100 || (outcome != "SUCCESS" && outcome != "FAILURE") {
		return errors.New("invalid management audit event")
	}
	data, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	if len(data) > 16*1024 {
		return errors.New("management audit summary exceeds 16 KiB")
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO management_audit_events(event_code,target_kind,target_id,outcome,error_code,summary_json,created_at_utc) VALUES(?,?,?,?,?,?,?)`, eventCode, targetKind, targetID, outcome, errorCode, data, formatTime(normalisedTime(now)))
	return err
}

func (s *OperationsStore) AuditPage(ctx context.Context, page int) (AuditPage, error) {
	if page < 1 || page > 1_000_000 {
		return AuditPage{}, errors.New("audit page is out of range")
	}
	result := AuditPage{Page: page, PageSize: 50}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM management_audit_events`).Scan(&result.TotalItems); err != nil {
		return AuditPage{}, err
	}
	result.TotalPages = int(math.Ceil(float64(result.TotalItems) / float64(result.PageSize)))
	rows, err := s.db.QueryContext(ctx, `SELECT id,event_code,target_kind,target_id,outcome,error_code,CAST(summary_json AS TEXT),created_at_utc FROM management_audit_events ORDER BY id DESC LIMIT ? OFFSET ?`, result.PageSize, (page-1)*result.PageSize)
	if err != nil {
		return AuditPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var value AuditEvent
		var created string
		if err := rows.Scan(&value.ID, &value.EventCode, &value.TargetKind, &value.TargetID, &value.Outcome, &value.ErrorCode, &value.SummaryJSON, &created); err != nil {
			return AuditPage{}, err
		}
		value.CreatedAtUTC, err = parseTime(created)
		if err != nil {
			return AuditPage{}, err
		}
		result.Items = append(result.Items, value)
	}
	return result, rows.Err()
}

func (s *OperationsStore) ClaimScheduled(ctx context.Context, taskKey, owner string, dueBefore time.Time, leaseDuration time.Duration, now time.Time) error {
	if taskKey == "" || len(taskKey) > 100 || owner == "" || len(owner) > 200 || leaseDuration <= 0 {
		return errors.New("invalid scheduled operation lease")
	}
	timestamp := normalisedTime(now)
	result, err := s.db.ExecContext(ctx, `UPDATE scheduled_operations
		SET last_started_at_utc=?,lease_owner=?,lease_expires_at_utc=?,last_error_code='',updated_at_utc=?
		WHERE task_key=? AND (last_completed_at_utc IS NULL OR last_completed_at_utc<=?)
		AND (lease_expires_at_utc IS NULL OR lease_expires_at_utc<=?)`,
		formatTime(timestamp), owner, formatTime(timestamp.Add(leaseDuration)), formatTime(timestamp),
		taskKey, formatTime(normalisedTime(dueBefore)), formatTime(timestamp))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrScheduledOperationNotDue
	}
	return nil
}

func (s *OperationsStore) CompleteScheduled(ctx context.Context, taskKey, owner string, now time.Time) error {
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `UPDATE scheduled_operations
		SET last_completed_at_utc=?,lease_owner=NULL,lease_expires_at_utc=NULL,last_error_code='',updated_at_utc=?
		WHERE task_key=? AND lease_owner=?`, timestamp, timestamp, taskKey, owner)
	return requireOneScheduledRow(result, err)
}

func (s *OperationsStore) FailScheduled(ctx context.Context, taskKey, owner, errorCode string, now time.Time) error {
	if len(errorCode) > 100 {
		return errors.New("scheduled operation error code exceeds 100 characters")
	}
	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `UPDATE scheduled_operations
		SET lease_owner=NULL,lease_expires_at_utc=NULL,last_error_code=?,updated_at_utc=?
		WHERE task_key=? AND lease_owner=?`, errorCode, timestamp, taskKey, owner)
	return requireOneScheduledRow(result, err)
}

func requireOneScheduledRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("scheduled operation lease was lost")
	}
	return nil
}
