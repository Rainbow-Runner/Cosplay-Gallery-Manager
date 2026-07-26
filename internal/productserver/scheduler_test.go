package productserver

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/persistence/productdb"
)

func TestDailyBackupUsesPersistentDueLeaseAndRetention(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	backupRoot := root + "/backups"
	coserRoot := root + "/cosers"
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, coserRoot, backupRoot, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	settings, err := server.Database.Settings().Find(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.DailyBackupRetention = 1
	if _, err := server.Database.Settings().Update(ctx, settings.Revision, settings, time.Now()); err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	if err := server.RunDailyBackupOnce(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := server.RunDailyBackupOnce(ctx, first.Add(23*time.Hour)); !errors.Is(err, productdb.ErrScheduledOperationNotDue) {
		t.Fatalf("second snapshot before due = %v", err)
	}
	if err := server.RunDailyBackupOnce(ctx, first.Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	records, err := server.Database.Backups().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var daily []productdb.BackupRecord
	for _, record := range records {
		if record.Kind == productdb.BackupDailySnapshot {
			daily = append(daily, record)
			if _, err := os.Stat(backupRoot + "/" + record.FileName); err != nil {
				t.Fatalf("retained snapshot is missing: %v", err)
			}
		}
	}
	if len(daily) != 1 {
		t.Fatalf("daily snapshots after retention = %d, want 1", len(daily))
	}
}

func TestAutomaticScanIsOptInPersistentAndRunsDiscovery(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 6, 0, 0, 0, time.UTC)
	libraryRoot := t.TempDir()
	mediaLibrary, err := server.Database.Libraries().Create(ctx, productdb.CreateLibraryInput{
		Name: "Automatic", RootPath: libraryRoot, Enabled: true, ReadOnly: true, CaptureTimezone: "UTC",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.RunAutomaticScanOnce(ctx, now); !errors.Is(err, productdb.ErrScheduledOperationNotDue) {
		t.Fatalf("disabled automatic scan = %v", err)
	}
	runtime, err := server.Database.Settings().Find(ctx)
	if err != nil {
		t.Fatal(err)
	}
	runtime.AutomaticScanEnabled = true
	if _, err := server.Database.Settings().Update(ctx, runtime.Revision, runtime, now); err != nil {
		t.Fatal(err)
	}
	if err := server.RunAutomaticScanOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := server.Database.CandidateDiscovery().LatestSnapshot(ctx, mediaLibrary.ID)
	if err != nil || snapshot.ID == 0 {
		t.Fatalf("automatic discovery snapshot = %#v/%v", snapshot, err)
	}
	if err := server.RunAutomaticScanOnce(ctx, now.Add(time.Hour)); !errors.Is(err, productdb.ErrScheduledOperationNotDue) {
		t.Fatalf("automatic scan lease/due guard = %v", err)
	}
}
