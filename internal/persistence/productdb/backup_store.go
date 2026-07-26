package productdb

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/product"
)

type BackupKind string

const (
	BackupDailySnapshot  BackupKind = "DAILY_SNAPSHOT"
	BackupManualFull     BackupKind = "MANUAL_FULL"
	BackupSafetySnapshot BackupKind = "SAFETY_SNAPSHOT"
)

type BackupRecord struct {
	ID                     string
	Kind                   BackupKind
	FileName               string
	Status                 string
	ByteSize               int64
	ArchiveSHA256          string
	ProductVersion         string
	DatabaseSchemaVersion  int
	ManifestSchemaVersion  int
	MediaProcessingVersion int
	CreatedAtUTC           time.Time
	CompletedAtUTC         *time.Time
	LastErrorCode          string
}

type FullBackupOptions struct {
	BackupRoot        string
	CoserMetadataRoot string
	ProductVersion    string
	StartupConfig     any
}

type PreparedRestore struct {
	Backup        BackupRecord
	DatabasePath  string
	CoserRoot     string
	TemporaryRoot string
}

type BackupStore struct{ database *Database }

func (db *Database) Backups() *BackupStore { return &BackupStore{database: db} }

func (s *BackupStore) List(ctx context.Context) ([]BackupRecord, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT backup_id,backup_kind,file_name,status,byte_size,archive_sha256,product_version,database_schema_version,manifest_schema_version,media_processing_version,created_at_utc,completed_at_utc,last_error_code FROM backup_records ORDER BY created_at_utc DESC,backup_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []BackupRecord
	for rows.Next() {
		value, err := scanBackupRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *BackupStore) Find(ctx context.Context, id string) (BackupRecord, error) {
	return scanBackupRecord(s.database.QueryRowContext(ctx, `SELECT backup_id,backup_kind,file_name,status,byte_size,archive_sha256,product_version,database_schema_version,manifest_schema_version,media_processing_version,created_at_utc,completed_at_utc,last_error_code FROM backup_records WHERE backup_id=?`, id))
}

func (s *BackupStore) CreateDatabaseSnapshot(ctx context.Context, backupRoot string, kind BackupKind, productVersion string, now time.Time) (BackupRecord, error) {
	if kind != BackupDailySnapshot {
		return BackupRecord{}, errors.New("invalid database snapshot kind")
	}
	if err := validateBackupRoot(backupRoot); err != nil {
		return BackupRecord{}, err
	}
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return BackupRecord{}, err
	}
	backupID := portableid.New()
	fileName := backupFileName(now, backupID, ".sqlite")
	record, err := s.beginRecord(ctx, backupID, kind, fileName, productVersion, now)
	if err != nil {
		return BackupRecord{}, err
	}
	target := filepath.Join(backupRoot, fileName)
	if err := s.database.Backup(ctx, target); err != nil {
		_ = s.failRecord(ctx, backupID, "BACKUP_SQLITE_FAILED", time.Now())
		_ = s.database.Operations().Audit(ctx, "BACKUP_CREATE", "BACKUP", backupID, "FAILURE", "BACKUP_SQLITE_FAILED", map[string]any{"kind": kind}, time.Now())
		return BackupRecord{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		_ = s.failRecord(ctx, backupID, "BACKUP_STAT_FAILED", time.Now())
		return BackupRecord{}, err
	}
	digest, err := applicationFileSHA256(target)
	if err != nil {
		_ = s.failRecord(ctx, backupID, "BACKUP_HASH_FAILED", time.Now())
		return BackupRecord{}, err
	}
	if err := s.completeRecord(ctx, backupID, info.Size(), digest, time.Now()); err != nil {
		return BackupRecord{}, err
	}
	_ = s.database.Operations().Audit(ctx, "BACKUP_CREATE", "BACKUP", backupID, "SUCCESS", "", map[string]any{"kind": kind, "bytes": info.Size()}, time.Now())
	record.Status, record.ByteSize, record.ArchiveSHA256 = "READY", info.Size(), digest
	completed := normalisedTime(time.Now())
	record.CompletedAtUTC = &completed
	return record, nil
}

func (s *BackupStore) CreateFull(ctx context.Context, options FullBackupOptions, now time.Time) (BackupRecord, error) {
	return s.createFull(ctx, BackupManualFull, options, now)
}

func (s *BackupStore) CreateSafetyFull(ctx context.Context, options FullBackupOptions, now time.Time) (BackupRecord, error) {
	return s.createFull(ctx, BackupSafetySnapshot, options, now)
}

func (s *BackupStore) createFull(ctx context.Context, kind BackupKind, options FullBackupOptions, now time.Time) (BackupRecord, error) {
	if kind != BackupManualFull && kind != BackupSafetySnapshot {
		return BackupRecord{}, errors.New("invalid full backup kind")
	}
	if err := validateBackupRoot(options.BackupRoot); err != nil {
		return BackupRecord{}, err
	}
	if options.CoserMetadataRoot == "" || !filepath.IsAbs(options.CoserMetadataRoot) {
		return BackupRecord{}, errors.New("Coser metadata root must be absolute")
	}
	if err := os.MkdirAll(options.BackupRoot, 0o700); err != nil {
		return BackupRecord{}, err
	}
	backupID := portableid.New()
	fileName := backupFileName(now, backupID, ".cgm-backup.zip")
	record, err := s.beginRecord(ctx, backupID, kind, fileName, options.ProductVersion, now)
	if err != nil {
		return BackupRecord{}, err
	}
	temporaryRoot, err := os.MkdirTemp(options.BackupRoot, ".cgm-backup-")
	if err != nil {
		_ = s.failRecord(ctx, backupID, "BACKUP_TEMP_FAILED", time.Now())
		return BackupRecord{}, err
	}
	defer os.RemoveAll(temporaryRoot)
	databasePath := filepath.Join(temporaryRoot, "database.sqlite")
	if err := s.database.Backup(ctx, databasePath); err != nil {
		_ = s.failRecord(ctx, backupID, "BACKUP_SQLITE_FAILED", time.Now())
		return BackupRecord{}, err
	}
	target := filepath.Join(options.BackupRoot, fileName)
	if err := writeFullBackupArchive(ctx, target, databasePath, options, backupID, now); err != nil {
		_ = os.Remove(target)
		_ = s.failRecord(ctx, backupID, "BACKUP_ARCHIVE_FAILED", time.Now())
		_ = s.database.Operations().Audit(ctx, "BACKUP_CREATE", "BACKUP", backupID, "FAILURE", "BACKUP_ARCHIVE_FAILED", map[string]any{"kind": kind}, time.Now())
		return BackupRecord{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		_ = s.failRecord(ctx, backupID, "BACKUP_STAT_FAILED", time.Now())
		return BackupRecord{}, err
	}
	digest, err := applicationFileSHA256(target)
	if err != nil {
		_ = s.failRecord(ctx, backupID, "BACKUP_HASH_FAILED", time.Now())
		return BackupRecord{}, err
	}
	if err := s.completeRecord(ctx, backupID, info.Size(), digest, time.Now()); err != nil {
		return BackupRecord{}, err
	}
	_ = s.database.Operations().Audit(ctx, "BACKUP_CREATE", "BACKUP", backupID, "SUCCESS", "", map[string]any{"kind": kind, "bytes": info.Size()}, time.Now())
	record.Status, record.ByteSize, record.ArchiveSHA256 = "READY", info.Size(), digest
	completed := normalisedTime(time.Now())
	record.CompletedAtUTC = &completed
	return record, nil
}

func (s *BackupStore) ApplyDailyRetention(ctx context.Context, backupRoot string, keep int) error {
	if keep < 1 || keep > 365 {
		return errors.New("daily backup retention must be between 1 and 365")
	}
	records, err := s.List(ctx)
	if err != nil {
		return err
	}
	var daily []BackupRecord
	for _, record := range records {
		if record.Kind == BackupDailySnapshot && record.Status == "READY" {
			daily = append(daily, record)
		}
	}
	for _, record := range daily[minimum(keep, len(daily)):] {
		if err := os.Remove(filepath.Join(backupRoot, record.FileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, err := s.database.ExecContext(ctx, `DELETE FROM backup_records WHERE backup_id=? AND backup_kind='DAILY_SNAPSHOT'`, record.ID); err != nil {
			return err
		}
		_ = s.database.Operations().Audit(ctx, "BACKUP_RETENTION_DELETE", "BACKUP", record.ID, "SUCCESS", "", map[string]any{"kind": record.Kind}, time.Now())
	}
	return nil
}

func (s *BackupStore) PrepareRestore(ctx context.Context, backupRoot, backupID string) (PreparedRestore, error) {
	record, err := s.Find(ctx, backupID)
	if err != nil {
		return PreparedRestore{}, err
	}
	if record.Status != "READY" {
		return PreparedRestore{}, errors.New("backup is not ready")
	}
	if record.DatabaseSchemaVersion != int(product.DatabaseSchemaVersion) {
		return PreparedRestore{}, errors.New("backup database schema version is incompatible")
	}
	source := filepath.Join(backupRoot, record.FileName)
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() != record.ByteSize {
		return PreparedRestore{}, errors.New("backup file size or type does not match its record")
	}
	digest, err := applicationFileSHA256(source)
	if err != nil {
		return PreparedRestore{}, err
	}
	if record.ArchiveSHA256 == "" || digest != record.ArchiveSHA256 {
		return PreparedRestore{}, errors.New("backup file SHA-256 does not match its record")
	}
	temporaryRoot, err := os.MkdirTemp(backupRoot, ".cgm-restore-")
	if err != nil {
		return PreparedRestore{}, err
	}
	prepared := PreparedRestore{Backup: record, TemporaryRoot: temporaryRoot, DatabasePath: filepath.Join(temporaryRoot, "database.sqlite")}
	cleanup := func(returnErr error) (PreparedRestore, error) {
		_ = os.RemoveAll(temporaryRoot)
		return PreparedRestore{}, returnErr
	}
	if strings.HasSuffix(record.FileName, ".cgm-backup.zip") {
		prepared.CoserRoot = filepath.Join(temporaryRoot, "coser-metadata")
		if err := extractFullBackup(source, temporaryRoot, record); err != nil {
			return cleanup(err)
		}
		if err := os.MkdirAll(prepared.CoserRoot, 0o700); err != nil {
			return cleanup(err)
		}
	} else {
		if err := copyRegularFile(source, prepared.DatabasePath); err != nil {
			return cleanup(err)
		}
	}
	inspection, err := inspectFile(ctx, prepared.DatabasePath)
	if err != nil || inspection.Kind != KindProduct {
		if err == nil {
			err = errors.New("prepared restore database has invalid product identity")
		}
		return cleanup(err)
	}
	return prepared, nil
}

func (s *BackupStore) RegisterExisting(ctx context.Context, record BackupRecord) error {
	var completed any
	if record.CompletedAtUTC != nil {
		completed = formatTime(*record.CompletedAtUTC)
	}
	_, err := s.database.ExecContext(ctx, `INSERT INTO backup_records(backup_id,backup_kind,file_name,status,byte_size,archive_sha256,product_version,database_schema_version,manifest_schema_version,media_processing_version,created_at_utc,completed_at_utc,last_error_code)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(backup_id) DO UPDATE SET
		file_name=excluded.file_name,status=excluded.status,byte_size=excluded.byte_size,archive_sha256=excluded.archive_sha256,
		product_version=excluded.product_version,database_schema_version=excluded.database_schema_version,
		manifest_schema_version=excluded.manifest_schema_version,media_processing_version=excluded.media_processing_version,
		completed_at_utc=excluded.completed_at_utc,last_error_code=excluded.last_error_code`,
		record.ID, record.Kind, record.FileName, record.Status, record.ByteSize, record.ArchiveSHA256,
		record.ProductVersion, record.DatabaseSchemaVersion, record.ManifestSchemaVersion, record.MediaProcessingVersion,
		formatTime(record.CreatedAtUTC), completed, record.LastErrorCode)
	return err
}

func (s *BackupStore) beginRecord(ctx context.Context, id string, kind BackupKind, fileName, productVersion string, now time.Time) (BackupRecord, error) {
	versions := product.CurrentVersions(productVersion)
	_, err := s.database.ExecContext(ctx, `INSERT INTO backup_records(backup_id,backup_kind,file_name,status,product_version,database_schema_version,manifest_schema_version,media_processing_version,created_at_utc) VALUES(?,?,?,'CREATING',?,?,?,?,?)`, id, kind, fileName, versions.Product, versions.DatabaseSchema, versions.ManifestSchema, versions.MediaProcessing, formatTime(normalisedTime(now)))
	if err != nil {
		return BackupRecord{}, err
	}
	return BackupRecord{ID: id, Kind: kind, FileName: fileName, Status: "CREATING", ProductVersion: versions.Product, DatabaseSchemaVersion: int(versions.DatabaseSchema), ManifestSchemaVersion: int(versions.ManifestSchema), MediaProcessingVersion: int(versions.MediaProcessing), CreatedAtUTC: normalisedTime(now)}, nil
}

func (s *BackupStore) completeRecord(ctx context.Context, id string, size int64, digest string, now time.Time) error {
	_, err := s.database.ExecContext(ctx, `UPDATE backup_records SET status='READY',byte_size=?,archive_sha256=?,completed_at_utc=?,last_error_code='' WHERE backup_id=? AND status='CREATING'`, size, digest, formatTime(normalisedTime(now)), id)
	return err
}

func (s *BackupStore) failRecord(ctx context.Context, id, code string, now time.Time) error {
	_, err := s.database.ExecContext(ctx, `UPDATE backup_records SET status='FAILED',last_error_code=?,completed_at_utc=? WHERE backup_id=? AND status='CREATING'`, code, formatTime(normalisedTime(now)), id)
	return err
}

type backupScanner interface{ Scan(...any) error }

func scanBackupRecord(scanner backupScanner) (BackupRecord, error) {
	var result BackupRecord
	var created string
	var completed sql.NullString
	err := scanner.Scan(&result.ID, &result.Kind, &result.FileName, &result.Status, &result.ByteSize, &result.ArchiveSHA256, &result.ProductVersion, &result.DatabaseSchemaVersion, &result.ManifestSchemaVersion, &result.MediaProcessingVersion, &created, &completed, &result.LastErrorCode)
	if err != nil {
		return BackupRecord{}, err
	}
	result.CreatedAtUTC, err = parseTime(created)
	if err != nil {
		return BackupRecord{}, err
	}
	if completed.Valid {
		value, err := parseTime(completed.String)
		if err != nil {
			return BackupRecord{}, err
		}
		result.CompletedAtUTC = &value
	}
	return result, nil
}

func writeFullBackupArchive(ctx context.Context, target, databasePath string, options FullBackupOptions, backupID string, now time.Time) (returnErr error) {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	defer func() {
		if err := archive.Close(); returnErr == nil && err != nil {
			returnErr = err
		}
		if err := file.Close(); returnErr == nil && err != nil {
			returnErr = err
		}
	}()
	versions := product.CurrentVersions(options.ProductVersion)
	manifestData, err := json.MarshalIndent(map[string]any{"format": "cgm-full-backup-v1", "backup_id": backupID, "created_at": normalisedTime(now).Format(time.RFC3339), "versions": versions, "includes": []string{"database.sqlite", "coser-metadata", "startup-config.json"}}, "", "  ")
	if err != nil {
		return err
	}
	if err := writeZipBytes(archive, "backup.json", append(manifestData, '\n')); err != nil {
		return err
	}
	configData, err := json.MarshalIndent(options.StartupConfig, "", "  ")
	if err != nil {
		return err
	}
	if err := writeZipBytes(archive, "startup-config.json", append(configData, '\n')); err != nil {
		return err
	}
	if err := writeZipFile(archive, "database.sqlite", databasePath); err != nil {
		return err
	}
	info, err := os.Lstat(options.CoserMetadataRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Coser metadata root is not a safe directory")
	}
	count, total := 0, int64(0)
	return filepath.WalkDir(options.CoserMetadataRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		count++
		total += info.Size()
		if count > 100000 || total > 10*1024*1024*1024 {
			return errors.New("Coser metadata backup exceeds safety limit")
		}
		relative, err := filepath.Rel(options.CoserMetadataRoot, path)
		if err != nil {
			return err
		}
		name := "coser-metadata/" + filepath.ToSlash(relative)
		if strings.Contains(name, "../") {
			return errors.New("invalid Coser metadata relative path")
		}
		return writeZipFile(archive, name, path)
	})
}

func extractFullBackup(source, destination string, record BackupRecord) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer archive.Close()
	if len(archive.File) > 100005 {
		return errors.New("backup archive has too many entries")
	}
	var manifestFound, databaseFound bool
	for _, entry := range archive.File {
		name := filepath.ToSlash(entry.Name)
		clean := filepath.ToSlash(filepath.Clean(name))
		if name == "" || strings.HasPrefix(name, "/") || clean != name || clean == ".." || strings.HasPrefix(clean, "../") || entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return errors.New("backup archive contains an unsafe path")
		}
		if name != "backup.json" && name != "database.sqlite" && name != "startup-config.json" && !strings.HasPrefix(name, "coser-metadata/") {
			return errors.New("backup archive contains an unsupported entry")
		}
		if entry.UncompressedSize64 > 10*1024*1024*1024 {
			return errors.New("backup archive entry is too large")
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, io.LimitReader(input, 10*1024*1024*1024+1))
		closeErr := output.Close()
		input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if name == "backup.json" {
			data, err := os.ReadFile(target)
			if err != nil {
				return err
			}
			var value struct {
				Format   string `json:"format"`
				BackupID string `json:"backup_id"`
			}
			if err := json.Unmarshal(data, &value); err != nil || value.Format != "cgm-full-backup-v1" || value.BackupID != record.ID {
				return errors.New("backup manifest does not match selected backup")
			}
			manifestFound = true
		}
		if name == "database.sqlite" {
			databaseFound = true
		}
	}
	if !manifestFound || !databaseFound {
		return errors.New("backup archive is incomplete")
	}
	return nil
}

func writeZipBytes(archive *zip.Writer, name string, data []byte) error {
	entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = entry.Write(data)
	return err
}

func writeZipFile(archive *zip.Writer, name, path string) error {
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o600)
	entry, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, input)
	return err
}

func copyRegularFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("backup source is not a regular file")
	}
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

func applicationFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validateBackupRoot(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return errors.New("backup root must be absolute")
	}
	return nil
}

func backupFileName(now time.Time, id, extension string) string {
	return normalisedTime(now).Format("20060102T150405Z") + "-" + id[:8] + extension
}

func minimum(left, right int) int {
	if left < right {
		return left
	}
	return right
}
