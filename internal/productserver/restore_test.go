package productserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

func TestFullRestoreRevokesSessionsCancelsJobsAndRequiresValidation(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	coserRoot, backupRoot := root+"/cosers", root+"/backups"
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, coserRoot, backupRoot, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	const password = "correct horse battery staple"
	if err := server.Auth.ConfigurePassword(ctx, password); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Before backup"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	job, err := server.Database.ProcessingJobs().Enqueue(ctx, productdb.EnqueueJobInput{Key: "restore-test-job", Kind: mediaprocessing.JobBackup}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	oldToken, _, err := server.Auth.CreateSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := server.CreateFullBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "After backup"}, time.Now()); err != nil {
		t.Fatal(err)
	}

	state, err := server.RestoreBackup(ctx, backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Mode != productdb.MaintenanceWaitingValidation {
		t.Fatalf("maintenance = %s", state.Mode)
	}
	var count int
	if err := server.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM galleries WHERE title='Before backup'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("pre-backup gallery count/error = %d/%v", count, err)
	}
	if err := server.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM galleries WHERE title='After backup'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("post-backup gallery count/error = %d/%v", count, err)
	}
	if authenticated, err := server.Auth.AuthenticateToken(ctx, oldToken); err != nil || authenticated {
		t.Fatalf("old session authenticated/error = %v/%v", authenticated, err)
	}
	if err := server.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE id=? AND status='CANCELLED'`, job.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("restored job cancellation count/error = %d/%v", count, err)
	}
	backups, err := server.Database.Backups().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	hasSafety := false
	for _, value := range backups {
		hasSafety = hasSafety || value.Kind == productdb.BackupSafetySnapshot
	}
	if !hasSafety {
		t.Fatal("restore did not register its safety backup")
	}

	login := httptest.NewRequest(http.MethodPost, "/session/login", strings.NewReader(`{"password":"`+password+`"}`))
	login.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusNoContent || len(loginResponse.Result().Cookies()) != 1 {
		t.Fatalf("restored login = %d %s", loginResponse.Code, loginResponse.Body.String())
	}
	resume := httptest.NewRequest(http.MethodPost, "/maintenance/resume", nil)
	resume.AddCookie(loginResponse.Result().Cookies()[0])
	resumeResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(resumeResponse, resume)
	if resumeResponse.Code != http.StatusNoContent {
		t.Fatalf("maintenance resume = %d %s", resumeResponse.Code, resumeResponse.Body.String())
	}
	state, err = server.Database.Operations().Maintenance(ctx)
	if err != nil || state.Mode != productdb.MaintenanceNormal {
		t.Fatalf("maintenance after resume = %s/%v", state.Mode, err)
	}
}

func TestRestoreFailureAfterDatabaseSwapRollsBackOriginalState(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	coserRoot, backupRoot := root+"/cosers", root+"/backups"
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, coserRoot, backupRoot, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Backup state"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	backup, err := server.CreateFullBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Must survive rollback"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	server.restoreAfterDatabaseSwapHook = func() error { return errors.New("injected swap failure") }
	if _, err := server.RestoreBackup(ctx, backup.ID); err == nil {
		t.Fatal("injected restore failure was ignored")
	}
	server.restoreAfterDatabaseSwapHook = nil
	var count int
	if err := server.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM galleries WHERE title='Must survive rollback'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("live database was not rolled back: count/error = %d/%v", count, err)
	}
	state, err := server.Database.Operations().Maintenance(ctx)
	if err != nil || state.Mode != productdb.MaintenanceNormal {
		t.Fatalf("maintenance after rollback = %s/%v", state.Mode, err)
	}
	events, err := server.Database.Operations().AuditPage(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	foundFailure := false
	for _, event := range events.Items {
		foundFailure = foundFailure || event.EventCode == "RESTORE_BACKUP" && event.Outcome == "FAILURE"
	}
	if !foundFailure {
		t.Fatal("automatic rollback did not leave a failure audit event")
	}
}
