package productdb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/product"
)

func TestFullBackupPackageAndPreparedRestoreContainOnlyManagedState(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC)
	coserRoot := t.TempDir()
	coserDirectory := filepath.Join(coserRoot, "11111111-1111-4111-8111-111111111111")
	if err := os.MkdirAll(coserDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coserDirectory, "coser.json"), []byte("{\"managed\":true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupRoot := t.TempDir()
	record, err := db.Backups().CreateFull(ctx, FullBackupOptions{
		BackupRoot: backupRoot, CoserMetadataRoot: coserRoot,
		ProductVersion: product.DevelopmentVersion,
		StartupConfig:  map[string]any{"listen": "127.0.0.1:9999"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "READY" || record.Kind != BackupManualFull || record.ByteSize == 0 || len(record.ArchiveSHA256) != 64 {
		t.Fatalf("full backup record = %#v", record)
	}
	prepared, err := db.Backups().PrepareRestore(ctx, backupRoot, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(prepared.TemporaryRoot)
	preparedDatabase, err := Open(ctx, prepared.DatabasePath)
	if err != nil {
		t.Fatalf("opening prepared database: %v", err)
	}
	if err := preparedDatabase.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(prepared.CoserRoot, "11111111-1111-4111-8111-111111111111", "coser.json"))
	if err != nil || string(data) != "{\"managed\":true}\n" {
		t.Fatalf("prepared Coser metadata = %q, %v", data, err)
	}
	events, err := db.Operations().AuditPage(ctx, 1)
	if err != nil || len(events.Items) == 0 || events.Items[0].EventCode != "BACKUP_CREATE" {
		t.Fatalf("backup audit = %#v, %v", events, err)
	}
	archive, err := os.OpenFile(filepath.Join(backupRoot, record.FileName), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write([]byte("corruption")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Backups().PrepareRestore(ctx, backupRoot, record.ID); err == nil {
		t.Fatal("corrupted full backup passed SHA-256 validation")
	}
}

func TestRestoreFinalizationRevokesWorkAndRequiresExplicitResume(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx, `INSERT INTO owner_sessions(token_hash,auth_revision,created_at_utc,last_seen_at_utc,expires_at_utc) VALUES(zeroblob(32),1,?,?,?)`,
		formatTime(now), formatTime(now), formatTime(now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	job, err := db.ProcessingJobs().Enqueue(ctx, EnqueueJobInput{Key: "restore-test", Kind: mediaprocessing.JobCache, Priority: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Operations().FinalizeRestore(ctx, "11111111-1111-4111-8111-111111111111", now); err != nil {
		t.Fatal(err)
	}
	state, err := db.Operations().Maintenance(ctx)
	if err != nil || state.Mode != MaintenanceWaitingValidation {
		t.Fatalf("maintenance after restore = %#v, %v", state, err)
	}
	var jobStatus string
	var activeSessions, suspended int
	if err := db.QueryRowContext(ctx, `SELECT status FROM processing_jobs WHERE id=?`, job.ID).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM owner_sessions WHERE revoked_at_utc IS NULL`).Scan(&activeSessions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT automatic_schedules_suspended FROM runtime_settings WHERE id=1`).Scan(&suspended); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "CANCELLED" || activeSessions != 0 || suspended != 1 {
		t.Fatalf("restore finalization = job %s, sessions %d, suspended %d", jobStatus, activeSessions, suspended)
	}
	if err := db.Operations().ResumeAfterValidation(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	state, _ = db.Operations().Maintenance(ctx)
	if state.Mode != MaintenanceNormal {
		t.Fatalf("maintenance after resume = %#v", state)
	}
}
