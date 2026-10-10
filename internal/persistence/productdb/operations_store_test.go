package productdb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
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
	if err != nil || state.Mode != MaintenanceWaitingValidation || state.LastErrorCode != RestorePathMappingRequired {
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
	if err := db.Operations().ResumeAfterValidation(ctx, now.Add(time.Minute)); err == nil {
		t.Fatal("maintenance resumed before restored paths were confirmed")
	}
	if err := db.Operations().ApplyRestorePathMappings(ctx, nil, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.Operations().ResumeAfterValidation(ctx, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	state, _ = db.Operations().Maintenance(ctx)
	if state.Mode != MaintenanceNormal {
		t.Fatalf("maintenance after resume = %#v", state)
	}
}

func TestRestorePathMappingsRemapWindowsPathsWithoutScanning(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	temporaryRoot := t.TempDir()
	mediaLibrary, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Restored", RootPath: temporaryRoot, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Mapped set"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &mediaLibrary.ID, Type: gallery.SourceTypeDirectory,
		Path: temporaryRoot + string(filepath.Separator) + "Event", Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE media_libraries SET root_path='C:\Photos' WHERE id=?`, mediaLibrary.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_sources SET source_path='c:\photos\Event',reconcile_state='IN_SYNC' WHERE id=?`, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO ignored_gallery_sources(library_id,source_path,created_at_utc) VALUES(?,'C:\PHOTOS\Ignored',?)`,
		mediaLibrary.ID, formatTime(now)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gallery_manifest_sync(
			gallery_id,manifest_path,status,schema_version,manifest_revision,file_hash,baseline_json,
			baseline_metadata_revision,checked_at_utc,last_error_code
		) VALUES(?,'C:\Photos\Event\cosplay-gallery.json','CLEAN',1,1,'hash','{}',1,?,'')`,
		created.ID, formatTime(now)); err != nil {
		t.Fatal(err)
	}
	if err := db.Operations().FinalizeRestore(ctx, "11111111-1111-4111-8111-111111111111", now); err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(t.TempDir(), "media")
	if err := db.Operations().ApplyRestorePathMappings(ctx, []RestorePathMapping{{
		LibraryID: mediaLibrary.ID, ExpectedRootPath: `C:\Photos`, RootPath: newRoot,
	}}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var libraryRoot, sourcePath, availability, reconcile, ignoredPath, manifestPath, manifestStatus string
	if err := db.QueryRowContext(ctx, `SELECT root_path FROM media_libraries WHERE id=?`, mediaLibrary.ID).Scan(&libraryRoot); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT source_path,availability_state,reconcile_state FROM gallery_sources WHERE id=?`, source.ID).
		Scan(&sourcePath, &availability, &reconcile); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT source_path FROM ignored_gallery_sources WHERE library_id=?`, mediaLibrary.ID).Scan(&ignoredPath); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT manifest_path,status FROM gallery_manifest_sync WHERE gallery_id=?`, created.ID).
		Scan(&manifestPath, &manifestStatus); err != nil {
		t.Fatal(err)
	}
	if libraryRoot != newRoot || sourcePath != filepath.Join(newRoot, "Event") ||
		ignoredPath != filepath.Join(newRoot, "Ignored") || manifestPath != filepath.Join(newRoot, "Event", "cosplay-gallery.json") {
		t.Fatalf("mapped paths = library %q, source %q, ignored %q, manifest %q", libraryRoot, sourcePath, ignoredPath, manifestPath)
	}
	if availability != "MISSING" || reconcile != "NEEDS_RESCAN" || manifestStatus != "MISSING" {
		t.Fatalf("post-map states = %s/%s/%s", availability, reconcile, manifestStatus)
	}
}

func TestRestorePathMappingsRequireCompleteCurrentDecisions(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC)
	mediaLibrary, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Restored", RootPath: t.TempDir(), Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Operations().FinalizeRestore(ctx, "11111111-1111-4111-8111-111111111111", now); err != nil {
		t.Fatal(err)
	}
	if err := db.Operations().ApplyRestorePathMappings(ctx, nil, now); err == nil {
		t.Fatal("incomplete mapping decisions were accepted")
	}
	if err := db.Operations().ApplyRestorePathMappings(ctx, []RestorePathMapping{{
		LibraryID: mediaLibrary.ID, ExpectedRootPath: "stale", Disable: true,
	}}, now); err == nil {
		t.Fatal("stale mapping decision was accepted")
	}
	current, err := db.Libraries().Find(ctx, mediaLibrary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Operations().ApplyRestorePathMappings(ctx, []RestorePathMapping{{
		LibraryID: mediaLibrary.ID, ExpectedRootPath: current.RootPath, Disable: true,
	}}, now); err != nil {
		t.Fatal(err)
	}
	current, err = db.Libraries().Find(ctx, mediaLibrary.ID)
	if err != nil || current.Enabled {
		t.Fatalf("disabled restored library = %#v, %v", current, err)
	}
}

func TestMapStoredPathSupportsPortableAbsolutePaths(t *testing.T) {
	tests := []struct {
		name, root, child, target, want string
	}{
		{"drive", `D:\Media`, `d:\media\Set\Photo.jpg`, "/mnt/library", filepath.FromSlash("/mnt/library/Set/Photo.jpg")},
		{"unc", `\\NAS\Share\Cosplay`, `//nas/share/cosplay/Set`, "/srv/media", filepath.FromSlash("/srv/media/Set")},
		{"posix", "/old/media", "/old/media/Set", "/new/media", filepath.FromSlash("/new/media/Set")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := mapStoredPath(test.root, test.target, test.child)
			if err != nil || got != test.want {
				t.Fatalf("mapStoredPath() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
	if _, err := mapStoredPath(`C:\Media`, "/new", `C:\Other\Set`); err == nil {
		t.Fatal("path outside restored root was mapped")
	}
}

func TestPreserveRestoredStorageRootsRemapsCoserManifests(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "Restored Coser"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	oldRoot := `E:\CGM\Cosers`
	oldManifest := oldRoot + `\` + coser.UUID + `\coser.json`
	if _, err := db.ExecContext(ctx, `UPDATE product_setup SET coser_metadata_root=?,backup_root=? WHERE id=1`, oldRoot, `E:\CGM\Backups`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO coser_manifest_sync(
			coser_uuid,manifest_path,status,schema_version,manifest_revision,file_hash,baseline_json,
			baseline_metadata_revision,checked_at_utc,last_error_code
		) VALUES(?,?,'CLEAN',1,1,'hash','{}',1,?,'')`, coser.UUID, oldManifest, formatTime(now)); err != nil {
		t.Fatal(err)
	}
	roots := StorageRoots{CoserMetadataRoot: filepath.Join(t.TempDir(), "cosers"), BackupRoot: filepath.Join(t.TempDir(), "backups")}
	if err := db.Operations().PreserveRestoredStorageRoots(ctx, roots, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var coserRoot, backupRoot, manifestPath, status string
	if err := db.QueryRowContext(ctx, `SELECT coser_metadata_root,backup_root FROM product_setup WHERE id=1`).Scan(&coserRoot, &backupRoot); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT manifest_path,status FROM coser_manifest_sync WHERE coser_uuid=?`, coser.UUID).Scan(&manifestPath, &status); err != nil {
		t.Fatal(err)
	}
	if coserRoot != roots.CoserMetadataRoot || backupRoot != roots.BackupRoot ||
		manifestPath != filepath.Join(roots.CoserMetadataRoot, coser.UUID, "coser.json") || status != "MISSING" {
		t.Fatalf("preserved roots/Manifest = %q, %q, %q, %q", coserRoot, backupRoot, manifestPath, status)
	}
}
