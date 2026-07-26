package productdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type MaintenanceMode string

const (
	MaintenanceNormal            MaintenanceMode = "NORMAL"
	MaintenanceRestoring         MaintenanceMode = "RESTORING"
	MaintenanceWaitingValidation MaintenanceMode = "WAITING_VALIDATION"
	RestorePathMappingRequired                   = "RESTORE_PATH_MAPPING_REQUIRED"
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

type RestorePathMapping struct {
	LibraryID        int64
	ExpectedRootPath string
	RootPath         string
	Disable          bool
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
	if _, err := tx.ExecContext(ctx, `UPDATE maintenance_state SET state='WAITING_VALIDATION',restore_backup_id=?,last_error_code=?,updated_at_utc=? WHERE id=1`, backupID, RestorePathMappingRequired, timestamp); err != nil {
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
	result, err := tx.ExecContext(ctx, `UPDATE maintenance_state SET state='NORMAL',restore_backup_id=NULL,last_error_code='',updated_at_utc=? WHERE id=1 AND state='WAITING_VALIDATION' AND last_error_code=''`, timestamp)
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

// PreserveRestoredStorageRoots keeps machine-local managed roots and remaps
// Coser Manifest paths into the restored managed metadata tree.
func (s *OperationsStore) PreserveRestoredStorageRoots(ctx context.Context, roots StorageRoots, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var oldCoserRoot string
	if err := tx.QueryRowContext(ctx, `SELECT coser_metadata_root FROM product_setup WHERE id=1`).Scan(&oldCoserRoot); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT coser_uuid,manifest_path FROM coser_manifest_sync ORDER BY coser_uuid`)
	if err != nil {
		return err
	}
	var manifests [][2]string
	for rows.Next() {
		var value [2]string
		if err := rows.Scan(&value[0], &value[1]); err != nil {
			_ = rows.Close()
			return err
		}
		manifests = append(manifests, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	timestamp := formatTime(normalisedTime(now))
	for _, manifest := range manifests {
		mapped, err := mapStoredPath(oldCoserRoot, roots.CoserMetadataRoot, manifest[1])
		if err != nil {
			return fmt.Errorf("Coser Manifest %s cannot be mapped to this machine: %w", manifest[0], err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE coser_manifest_sync SET manifest_path=?,status='MISSING',last_error_code='RESTORE_REVALIDATION_REQUIRED',checked_at_utc=? WHERE coser_uuid=?`,
			mapped, timestamp, manifest[0]); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE product_setup SET coser_metadata_root=?,backup_root=? WHERE id=1`,
		roots.CoserMetadataRoot, roots.BackupRoot); err != nil {
		return err
	}
	return tx.Commit()
}

// ApplyRestorePathMappings atomically resolves every restored media-library
// root. It only updates configuration and invalidates scan state; it never
// reads media or starts discovery.
func (s *OperationsStore) ApplyRestorePathMappings(ctx context.Context, mappings []RestorePathMapping, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var mode MaintenanceMode
	var errorCode string
	if err := tx.QueryRowContext(ctx, `SELECT state,last_error_code FROM maintenance_state WHERE id=1`).Scan(&mode, &errorCode); err != nil {
		return err
	}
	if mode != MaintenanceWaitingValidation || errorCode != RestorePathMappingRequired {
		return errors.New("restore path mapping is not pending")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,root_path,enabled FROM media_libraries ORDER BY id`)
	if err != nil {
		return err
	}
	type restoredLibrary struct {
		id      int64
		root    string
		enabled bool
	}
	var libraries []restoredLibrary
	for rows.Next() {
		var value restoredLibrary
		if err := rows.Scan(&value.id, &value.root, &value.enabled); err != nil {
			_ = rows.Close()
			return err
		}
		libraries = append(libraries, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(mappings) != len(libraries) {
		return errors.New("every restored media library requires one path decision")
	}
	byID := make(map[int64]RestorePathMapping, len(mappings))
	for _, mapping := range mappings {
		if mapping.LibraryID <= 0 {
			return errors.New("invalid media library mapping")
		}
		if _, exists := byID[mapping.LibraryID]; exists {
			return errors.New("duplicate media library mapping")
		}
		if mapping.Disable {
			if mapping.RootPath != "" {
				return errors.New("disabled media library mapping cannot include a new root")
			}
		} else if mapping.RootPath == "" || !filepath.IsAbs(mapping.RootPath) || filepath.Clean(mapping.RootPath) != mapping.RootPath {
			return errors.New("mapped media library root must be a canonical absolute path")
		}
		byID[mapping.LibraryID] = mapping
	}
	timestamp := formatTime(normalisedTime(now))
	for _, current := range libraries {
		mapping, exists := byID[current.id]
		if !exists || mapping.ExpectedRootPath != current.root {
			return errors.New("restored media library mapping is stale")
		}
		if mapping.Disable {
			if _, err := tx.ExecContext(ctx, `UPDATE media_libraries SET enabled=0,updated_at_utc=? WHERE id=?`, timestamp, current.id); err != nil {
				return err
			}
		} else {
			if err := remapLibraryPaths(ctx, tx, current.id, current.root, mapping.RootPath, timestamp); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE media_libraries SET root_path=?,updated_at_utc=? WHERE id=?`, mapping.RootPath, timestamp, current.id); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_sources SET availability_state='MISSING',reconcile_state='NEEDS_RESCAN',updated_at_utc=? WHERE library_id=?`, timestamp, current.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_manifest_sync
			SET status='MISSING',last_error_code='RESTORE_REVALIDATION_REQUIRED',checked_at_utc=?
			WHERE gallery_id IN (SELECT gallery_id FROM gallery_sources WHERE library_id=?)`, timestamp, current.id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM discovery_snapshots`); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE maintenance_state SET last_error_code='',updated_at_utc=? WHERE id=1 AND state='WAITING_VALIDATION' AND last_error_code=?`,
		timestamp, RestorePathMappingRequired)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return errors.New("restore path mapping state changed concurrently")
	}
	return tx.Commit()
}

func remapLibraryPaths(ctx context.Context, tx *sql.Tx, libraryID int64, oldRoot, newRoot, timestamp string) error {
	type pathRow struct {
		id   int64
		path string
	}
	remapRows := func(query, update string, arguments ...any) error {
		rows, err := tx.QueryContext(ctx, query, arguments...)
		if err != nil {
			return err
		}
		var values []pathRow
		for rows.Next() {
			var value pathRow
			if err := rows.Scan(&value.id, &value.path); err != nil {
				_ = rows.Close()
				return err
			}
			values = append(values, value)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, value := range values {
			mapped, err := mapStoredPath(oldRoot, newRoot, value.path)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, update, mapped, value.id); err != nil {
				return err
			}
		}
		return nil
	}
	if err := remapRows(`SELECT id,source_path FROM gallery_sources WHERE library_id=? ORDER BY id`,
		`UPDATE gallery_sources SET source_path=? WHERE id=?`, libraryID); err != nil {
		return fmt.Errorf("mapping GallerySource paths: %w", err)
	}
	if err := remapRows(`SELECT id,source_path FROM ignored_gallery_sources WHERE library_id=? ORDER BY id`,
		`UPDATE ignored_gallery_sources SET source_path=? WHERE id=?`, libraryID); err != nil {
		return fmt.Errorf("mapping ignored GallerySource paths: %w", err)
	}
	if err := remapRows(`SELECT sync.gallery_id,sync.manifest_path FROM gallery_manifest_sync sync
			JOIN gallery_sources source ON source.gallery_id=sync.gallery_id WHERE source.library_id=? ORDER BY sync.gallery_id`,
		`UPDATE gallery_manifest_sync SET manifest_path=? WHERE gallery_id=?`,
		libraryID); err != nil {
		return fmt.Errorf("mapping Gallery Manifest paths: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gallery_manifest_sync
		SET status='MISSING',last_error_code='RESTORE_REVALIDATION_REQUIRED',checked_at_utc=?
		WHERE gallery_id IN (SELECT gallery_id FROM gallery_sources WHERE library_id=?)`, timestamp, libraryID); err != nil {
		return err
	}
	return nil
}

type storedAbsolutePath struct {
	kind     string
	volume   string
	segments []string
}

func parseStoredAbsolutePath(value string) (storedAbsolutePath, error) {
	value = strings.ReplaceAll(value, `\`, "/")
	result := storedAbsolutePath{}
	switch {
	case strings.HasPrefix(value, "//"):
		result.kind = "windows"
		parts := strings.Split(strings.TrimPrefix(value, "//"), "/")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return result, errors.New("invalid UNC path")
		}
		result.volume = "//" + strings.ToLower(parts[0]) + "/" + strings.ToLower(parts[1])
		result.segments = parts[2:]
	case len(value) >= 3 && value[1] == ':' && value[2] == '/' &&
		((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')):
		result.kind = "windows"
		result.volume = strings.ToLower(value[:2])
		result.segments = strings.Split(value[3:], "/")
	case strings.HasPrefix(value, "/"):
		result.kind = "posix"
		result.volume = "/"
		result.segments = strings.Split(strings.TrimPrefix(value, "/"), "/")
	default:
		return result, errors.New("path is not absolute")
	}
	cleaned := result.segments[:0]
	for _, segment := range result.segments {
		switch segment {
		case "", ".":
			continue
		case "..":
			if len(cleaned) == 0 {
				return result, errors.New("path escapes its root")
			}
			cleaned = cleaned[:len(cleaned)-1]
		default:
			cleaned = append(cleaned, segment)
		}
	}
	result.segments = cleaned
	return result, nil
}

func mapStoredPath(oldRoot, newRoot, candidate string) (string, error) {
	root, err := parseStoredAbsolutePath(oldRoot)
	if err != nil {
		return "", err
	}
	child, err := parseStoredAbsolutePath(candidate)
	if err != nil {
		return "", err
	}
	equal := func(left, right string) bool {
		if root.kind == "windows" {
			return strings.EqualFold(left, right)
		}
		return left == right
	}
	if root.kind != child.kind || !equal(root.volume, child.volume) || len(child.segments) < len(root.segments) {
		return "", errors.New("path is outside its restored root")
	}
	for index := range root.segments {
		if !equal(root.segments[index], child.segments[index]) {
			return "", errors.New("path is outside its restored root")
		}
	}
	relative := path.Join(child.segments[len(root.segments):]...)
	if relative == "." {
		return newRoot, nil
	}
	return filepath.Join(newRoot, filepath.FromSlash(relative)), nil
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
