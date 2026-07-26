package productserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/productauth"
)

func (s *Server) CreateFullBackup(ctx context.Context) (productdb.BackupRecord, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	state, err := s.Database.Operations().Maintenance(ctx)
	if err != nil {
		return productdb.BackupRecord{}, err
	}
	if state.Mode != productdb.MaintenanceNormal {
		return productdb.BackupRecord{}, errors.New("full backup is unavailable during maintenance")
	}
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return productdb.BackupRecord{}, err
	}
	version, _, _ := build.Version()
	return s.Database.Backups().CreateFull(ctx, productdb.FullBackupOptions{
		BackupRoot: roots.BackupRoot, CoserMetadataRoot: roots.CoserMetadataRoot,
		ProductVersion: version, StartupConfig: s.Config,
	}, time.Now())
}

func (s *Server) RestoreBackup(ctx context.Context, backupID string) (productdb.MaintenanceState, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if _, err := portableid.Parse(backupID); err != nil {
		return productdb.MaintenanceState{}, errors.New("invalid backup ID")
	}
	current := s.Database
	state, err := current.Operations().Maintenance(ctx)
	if err != nil {
		return productdb.MaintenanceState{}, err
	}
	if state.Mode != productdb.MaintenanceNormal {
		return productdb.MaintenanceState{}, errors.New("maintenance operation is already active")
	}
	roots, err := current.Operations().StorageRoots(ctx)
	if err != nil {
		return productdb.MaintenanceState{}, err
	}
	prepared, err := current.Backups().PrepareRestore(ctx, roots.BackupRoot, backupID)
	if err != nil {
		_ = current.Operations().Audit(ctx, "RESTORE_BACKUP", "BACKUP", backupID, "FAILURE", "RESTORE_PREPARE_FAILED", nil, time.Now())
		return productdb.MaintenanceState{}, err
	}
	defer os.RemoveAll(prepared.TemporaryRoot)
	version, _, _ := build.Version()
	safety, err := current.Backups().CreateSafetyFull(ctx, productdb.FullBackupOptions{
		BackupRoot: roots.BackupRoot, CoserMetadataRoot: roots.CoserMetadataRoot,
		ProductVersion: version, StartupConfig: s.Config,
	}, time.Now())
	if err != nil {
		_ = current.Operations().Audit(ctx, "RESTORE_BACKUP", "BACKUP", backupID, "FAILURE", "RESTORE_SAFETY_BACKUP_FAILED", nil, time.Now())
		return productdb.MaintenanceState{}, err
	}
	if err := current.Operations().SetMaintenance(ctx, productdb.MaintenanceRestoring, backupID, "", time.Now()); err != nil {
		return productdb.MaintenanceState{}, err
	}
	s.stopWorkers()

	databasePath := current.Path()
	stageDatabase := filepath.Join(filepath.Dir(databasePath), "."+filepath.Base(databasePath)+".restore-"+backupID)
	if err := copyApplicationFile(prepared.DatabasePath, stageDatabase); err != nil {
		return s.restoreBeforeSwapFailed(ctx, current, backupID, "RESTORE_DATABASE_STAGE_FAILED", err)
	}
	defer os.Remove(stageDatabase)
	validated, err := productdb.Open(ctx, stageDatabase)
	if err != nil {
		return s.restoreBeforeSwapFailed(ctx, current, backupID, "RESTORE_DATABASE_INVALID", err)
	}
	if err := validated.Close(); err != nil {
		return s.restoreBeforeSwapFailed(ctx, current, backupID, "RESTORE_DATABASE_INVALID", err)
	}

	var stageCoser string
	if prepared.CoserRoot != "" {
		stageCoser = filepath.Join(filepath.Dir(roots.CoserMetadataRoot), "."+filepath.Base(roots.CoserMetadataRoot)+".restore-"+backupID)
		if err := copyManagedDirectory(prepared.CoserRoot, stageCoser); err != nil {
			return s.restoreBeforeSwapFailed(ctx, current, backupID, "RESTORE_COSER_STAGE_FAILED", err)
		}
		defer os.RemoveAll(stageCoser)
	}
	_, _ = current.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	if err := current.Close(); err != nil {
		reopened, openErr := productdb.Open(context.Background(), current.Path())
		if openErr != nil {
			return productdb.MaintenanceState{}, errors.New("restore stopped before replacement and the live database could not be reopened")
		}
		_ = reopened.Operations().SetMaintenance(context.Background(), productdb.MaintenanceNormal, "", "RESTORE_DATABASE_CLOSE_FAILED", time.Now())
		_ = reopened.Operations().Audit(context.Background(), "RESTORE_BACKUP", "BACKUP", backupID, "FAILURE", "RESTORE_DATABASE_CLOSE_FAILED", nil, time.Now())
		s.Database, s.Auth = reopened, productauth.New(reopened)
		s.rebuildHandler()
		s.restartWorkers()
		return productdb.MaintenanceState{}, err
	}

	rollbackDatabase := databasePath + ".rollback-" + backupID
	rollbackCoser := roots.CoserMetadataRoot + ".rollback-" + backupID
	databaseSwapped, coserSwapped := false, false
	if err := moveDatabaseAside(databasePath, rollbackDatabase); err != nil {
		return s.rollbackRestore(ctx, current, nil, roots, backupID, safety, rollbackDatabase, rollbackCoser, databaseSwapped, coserSwapped, "RESTORE_DATABASE_SWAP_FAILED", err)
	}
	if err := os.Rename(stageDatabase, databasePath); err != nil {
		return s.rollbackRestore(ctx, current, nil, roots, backupID, safety, rollbackDatabase, rollbackCoser, databaseSwapped, coserSwapped, "RESTORE_DATABASE_SWAP_FAILED", err)
	}
	databaseSwapped = true
	if s.restoreAfterDatabaseSwapHook != nil {
		if err := s.restoreAfterDatabaseSwapHook(); err != nil {
			return s.rollbackRestore(ctx, current, nil, roots, backupID, safety, rollbackDatabase, rollbackCoser, databaseSwapped, coserSwapped, "RESTORE_INJECTED_SWAP_FAILURE", err)
		}
	}
	if stageCoser != "" {
		if err := moveDirectoryAside(roots.CoserMetadataRoot, rollbackCoser); err != nil {
			return s.rollbackRestore(ctx, current, nil, roots, backupID, safety, rollbackDatabase, rollbackCoser, databaseSwapped, coserSwapped, "RESTORE_COSER_SWAP_FAILED", err)
		}
		if err := os.Rename(stageCoser, roots.CoserMetadataRoot); err != nil {
			return s.rollbackRestore(ctx, current, nil, roots, backupID, safety, rollbackDatabase, rollbackCoser, databaseSwapped, coserSwapped, "RESTORE_COSER_SWAP_FAILED", err)
		}
		coserSwapped = true
	}
	replacement, err := productdb.Open(ctx, databasePath)
	if err != nil {
		return s.rollbackRestore(ctx, current, nil, roots, backupID, safety, rollbackDatabase, rollbackCoser, databaseSwapped, coserSwapped, "RESTORE_DATABASE_REOPEN_FAILED", err)
	}
	if _, err := replacement.ExecContext(ctx, `UPDATE product_setup SET coser_metadata_root=?,backup_root=? WHERE id=1`, roots.CoserMetadataRoot, roots.BackupRoot); err != nil {
		return s.rollbackRestore(ctx, current, replacement, roots, backupID, safety, rollbackDatabase, rollbackCoser, databaseSwapped, coserSwapped, "RESTORE_ENVIRONMENT_PRESERVE_FAILED", err)
	}
	if err := replacement.Backups().RegisterExisting(ctx, safety); err != nil {
		return s.rollbackRestore(ctx, current, replacement, roots, backupID, safety, rollbackDatabase, rollbackCoser, databaseSwapped, coserSwapped, "RESTORE_SAFETY_REGISTER_FAILED", err)
	}
	if err := replacement.Backups().RegisterExisting(ctx, prepared.Backup); err != nil {
		return s.rollbackRestore(ctx, current, replacement, roots, backupID, safety, rollbackDatabase, rollbackCoser, databaseSwapped, coserSwapped, "RESTORE_BACKUP_REGISTER_FAILED", err)
	}
	if err := replacement.Operations().FinalizeRestore(ctx, backupID, time.Now()); err != nil {
		return s.rollbackRestore(ctx, current, replacement, roots, backupID, safety, rollbackDatabase, rollbackCoser, databaseSwapped, coserSwapped, "RESTORE_FINALIZE_FAILED", err)
	}
	_ = replacement.Operations().Audit(ctx, "RESTORE_BACKUP", "BACKUP", backupID, "SUCCESS", "", map[string]any{"safety_backup_id": safety.ID}, time.Now())
	s.Database, s.Auth = replacement, productauth.New(replacement)
	s.rebuildHandler()
	removeDatabaseFiles(rollbackDatabase)
	if coserSwapped {
		_ = os.RemoveAll(rollbackCoser)
	}
	return replacement.Operations().Maintenance(ctx)
}

func (s *Server) restoreBeforeSwapFailed(ctx context.Context, current *productdb.Database, backupID, code string, cause error) (productdb.MaintenanceState, error) {
	_ = current.Operations().SetMaintenance(ctx, productdb.MaintenanceNormal, "", code, time.Now())
	_ = current.Operations().Audit(ctx, "RESTORE_BACKUP", "BACKUP", backupID, "FAILURE", code, nil, time.Now())
	s.restartWorkers()
	return productdb.MaintenanceState{}, cause
}

func (s *Server) rollbackRestore(ctx context.Context, previous, replacement *productdb.Database, roots productdb.StorageRoots, backupID string, safety productdb.BackupRecord, rollbackDatabase, rollbackCoser string, databaseSwapped, coserSwapped bool, code string, cause error) (productdb.MaintenanceState, error) {
	if replacement != nil {
		_ = replacement.Close()
	}
	if databaseSwapped {
		removeDatabaseFiles(previous.Path())
	}
	if _, err := os.Stat(rollbackDatabase); err == nil {
		_ = restoreDatabaseAside(rollbackDatabase, previous.Path())
	}
	if coserSwapped {
		_ = os.RemoveAll(roots.CoserMetadataRoot)
	}
	if _, err := os.Stat(rollbackCoser); err == nil {
		_ = os.Rename(rollbackCoser, roots.CoserMetadataRoot)
	}
	reopened, openErr := productdb.Open(context.Background(), previous.Path())
	if openErr != nil {
		return productdb.MaintenanceState{}, errors.New("restore failed and automatic rollback could not reopen the original database")
	}
	_ = reopened.Backups().RegisterExisting(context.Background(), safety)
	_ = reopened.Operations().SetMaintenance(context.Background(), productdb.MaintenanceNormal, "", code, time.Now())
	_ = reopened.Operations().Audit(context.Background(), "RESTORE_BACKUP", "BACKUP", backupID, "FAILURE", code, nil, time.Now())
	s.Database, s.Auth = reopened, productauth.New(reopened)
	s.rebuildHandler()
	s.restartWorkers()
	return productdb.MaintenanceState{}, cause
}

func (s *Server) restartWorkers() {
	s.workerMu.Lock()
	root := s.workerRoot
	s.workerMu.Unlock()
	if root != nil && root.Err() == nil {
		_ = s.startWorkers(root)
	}
}

func (s *Server) maintenanceGate(database *productdb.Database, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/healthz", "/readyz", "/session/login", "/session/logout", "/session/status", "/maintenance/status", "/maintenance/resume":
			next.ServeHTTP(response, request)
			return
		}
		state, err := database.Operations().Maintenance(request.Context())
		if err != nil || state.Mode != productdb.MaintenanceNormal {
			http.Error(response, "maintenance mode", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func (s *Server) maintenanceStatusHandler(database *productdb.Database, auth *productauth.Service) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !auth.AuthorizeRequest(request) {
			http.Error(response, "authentication required", http.StatusUnauthorized)
			return
		}
		state, err := database.Operations().Maintenance(request.Context())
		if err != nil {
			http.Error(response, "maintenance status unavailable", http.StatusInternalServerError)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(response).Encode(map[string]any{"state": state.Mode, "requiresValidation": state.Mode == productdb.MaintenanceWaitingValidation})
	})
}

func (s *Server) maintenanceResumeHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !s.Auth.AuthorizeRequest(request) {
			http.Error(response, "authentication required", http.StatusUnauthorized)
			return
		}
		if err := s.ResumeMaintenance(request.Context()); err != nil {
			http.Error(response, "maintenance resume failed", http.StatusConflict)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	})
}

func (s *Server) ResumeMaintenance(ctx context.Context) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if err := s.validateRuntimeEnvironment(); err != nil {
		_ = s.Database.Operations().Audit(ctx, "MAINTENANCE_RESUME", "SYSTEM", "", "FAILURE", "ENVIRONMENT_VALIDATION_FAILED", nil, time.Now())
		return err
	}
	if err := s.Database.Operations().ResumeAfterValidation(ctx, time.Now()); err != nil {
		_ = s.Database.Operations().Audit(ctx, "MAINTENANCE_RESUME", "SYSTEM", "", "FAILURE", "MAINTENANCE_RESUME_FAILED", nil, time.Now())
		return err
	}
	_ = s.Database.Operations().Audit(ctx, "MAINTENANCE_RESUME", "SYSTEM", "", "SUCCESS", "", nil, time.Now())
	s.restartWorkers()
	s.rebuildHandler()
	return nil
}

func (s *Server) validateRuntimeEnvironment() error {
	roots, err := s.Database.Operations().StorageRoots(context.Background())
	if err != nil {
		return err
	}
	for _, root := range []string{roots.BackupRoot, roots.CoserMetadataRoot, s.Config.CachePath} {
		if root == "" {
			return errors.New("required storage root is empty")
		}
		if err := os.MkdirAll(root, 0o700); err != nil {
			return err
		}
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("required storage root is unsafe")
		}
		probe, err := os.CreateTemp(root, ".cgm-write-check-")
		if err != nil {
			return err
		}
		name := probe.Name()
		if err := probe.Close(); err != nil {
			return err
		}
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	for _, executable := range []string{s.Config.FFmpegPath, s.Config.LibRawPath} {
		if executable == "" {
			continue
		}
		info, err := os.Stat(executable)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			return errors.New("configured media executable is unavailable")
		}
	}
	return nil
}

func copyApplicationFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func copyManagedDirectory(source, target string) error {
	if err := os.Mkdir(target, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("managed metadata tree contains a symbolic link")
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("managed metadata path escapes its root")
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("managed metadata tree contains a non-regular file")
		}
		return copyApplicationFile(path, destination)
	})
}

func moveDatabaseAside(path, rollback string) error {
	if err := os.Rename(path, rollback); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(path + suffix); err == nil {
			if err := os.Rename(path+suffix, rollback+suffix); err != nil {
				return err
			}
		}
	}
	return nil
}

func restoreDatabaseAside(rollback, target string) error {
	if err := os.Rename(rollback, target); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(rollback + suffix); err == nil {
			if err := os.Rename(rollback+suffix, target+suffix); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeDatabaseFiles(path string) {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		_ = os.Remove(path + suffix)
	}
}

func moveDirectoryAside(path, rollback string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Coser metadata root is unsafe")
	}
	return os.Rename(path, rollback)
}
