package productdb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/portableid"
)

var dailySnapshotSidecar = regexp.MustCompile(`^\d{8}T\d{6}Z-([0-9a-f]{8})\.sqlite-(?:wal|shm)$`)

func removeDailySnapshotFiles(root, name string) error {
	// Validate the group first. Remove the main database last, so sidecar
	// failures leave the record and main file available for retry.
	paths := []string{filepath.Join(root, name+"-wal"), filepath.Join(root, name+"-shm"), filepath.Join(root, name)}
	for _, path := range paths {
		if err := regularRetentionFile(path); err != nil {
			return err
		}
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func regularRetentionFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("backup retention refuses non-regular files")
	}
	return nil
}

func (s *BackupStore) cleanupRetiredDailySidecars(ctx context.Context, root string, records []BackupRecord, now time.Time) error {
	// Older versions removed only .sqlite. A successful retention audit is
	// required ownership evidence; an unknown filename is never sufficient.
	rows, err := s.database.QueryContext(ctx, `SELECT target_id FROM management_audit_events WHERE event_code='BACKUP_RETENTION_DELETE' AND target_kind='BACKUP' AND outcome='SUCCESS'`)
	if err != nil {
		return err
	}
	retired := map[string]string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if _, err := portableid.Parse(id); err == nil {
			retired[id[:8]] = id
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(retired) == 0 {
		return err
	}
	protected := map[string]bool{}
	for _, record := range records {
		protected[record.FileName] = true // Includes READY, CREATING and FAILED.
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var failures []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		match := dailySnapshotSidecar.FindStringSubmatch(name)
		if match == nil || retired[match[1]] == "" {
			continue
		}
		if _, err := time.Parse("20060102T150405Z", name[:16]); err != nil {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, "-wal"), "-shm")
		if protected[base] {
			continue
		}
		if _, err := os.Lstat(filepath.Join(root, base)); !errors.Is(err, os.ErrNotExist) {
			continue // Even an unregistered main file protects its sidecars.
		}
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || now.Sub(info.ModTime()) < 24*time.Hour {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, err)
		} else if err == nil {
			_ = s.database.Operations().Audit(ctx, "BACKUP_RETENTION_SIDECAR_DELETE", "BACKUP", retired[match[1]], "SUCCESS", "", map[string]any{"fileName": name, "bytes": info.Size()}, now)
		}
	}
	return errors.Join(failures...)
}
