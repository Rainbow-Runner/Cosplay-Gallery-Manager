package productdb

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/product"
)

type backupFixtureEntry struct {
	name string
	mode os.FileMode
	data []byte
}

func TestFullBackupRejectsCrossPlatformCorruptionSamples(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	backupRoot, coserRoot := t.TempDir(), t.TempDir()
	record, err := db.Backups().CreateFull(ctx, FullBackupOptions{
		BackupRoot: backupRoot, CoserMetadataRoot: coserRoot,
		ProductVersion: product.DevelopmentVersion, StartupConfig: map[string]any{"listen": "127.0.0.1:9999"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	original, err := readBackupFixture(filepath.Join(backupRoot, record.FileName))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		mutate   func([]backupFixtureEntry) []backupFixtureEntry
		truncate bool
	}{
		{"parent traversal", appendFixture("../outside", 0o600, []byte("x")), false},
		{"backslash separator", appendFixture(`coser-metadata\outside`, 0o600, []byte("x")), false},
		{"drive absolute path", appendFixture("C:/outside", 0o600, []byte("x")), false},
		{"duplicate entry", appendFixture("backup.json", 0o600, []byte("{}")), false},
		{"case-fold collision", appendFixture("BACKUP.JSON", 0o600, []byte("{}")), false},
		{"non-NFC path", appendFixture("coser-metadata/e\u0301.json", 0o600, []byte("{}")), false},
		{"Windows reserved name", appendFixture("coser-metadata/CON.json", 0o600, []byte("{}")), false},
		{"symbolic link", appendFixture("coser-metadata/link", os.ModeSymlink|0o777, []byte("target")), false},
		{"unsupported entry", appendFixture("media/file.jpg", 0o600, []byte("x")), false},
		{"malformed startup config", replaceFixture("startup-config.json", []byte("[]")), false},
		{"mismatched manifest", replaceFixture("backup.json", []byte("{}")), false},
		{"missing database", removeFixture("database.sqlite"), false},
		{"invalid SQLite identity", replaceFixture("database.sqlite", []byte("not a product database")), false},
		{"excessive compression ratio", appendFixture("coser-metadata/bomb.json", 0o600, bytes.Repeat([]byte{0}, 8*1024*1024)), false},
		{"truncated archive", func(entries []backupFixtureEntry) []backupFixtureEntry { return entries }, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(backupRoot, record.FileName)
			if err := writeBackupFixture(path, test.mutate(cloneFixture(original))); err != nil {
				t.Fatal(err)
			}
			if test.truncate {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data[:len(data)-16], 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := updateBackupFixtureRecord(ctx, db, record.ID, path); err != nil {
				t.Fatal(err)
			}
			if prepared, err := db.Backups().PrepareRestore(ctx, backupRoot, record.ID); err == nil {
				_ = os.RemoveAll(prepared.TemporaryRoot)
				t.Fatal("corrupted cross-platform backup sample was accepted")
			}
		})
	}
}

func TestPortableArchiveNameValidation(t *testing.T) {
	seen := make(map[string]string)
	for _, name := range []string{"backup.json", "startup-config.json", "database.sqlite", "coser-metadata/人物/资料.json"} {
		if err := registerPortableArchiveName(seen, name, false); err != nil {
			t.Fatalf("portable name %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{
		"/absolute", "../parent", `coser-metadata\file`, "C:/drive", "coser-metadata/trailing.",
		"coser-metadata/NUL.txt", "coser-metadata/e\u0301.json", "coser-metadata/control\u0001",
	} {
		if err := registerPortableArchiveName(make(map[string]string), name, false); err == nil {
			t.Fatalf("non-portable name %q accepted", name)
		}
	}
	collision := make(map[string]string)
	if err := registerPortableArchiveName(collision, "coser-metadata/Avatar.jpg", false); err != nil {
		t.Fatal(err)
	}
	if err := registerPortableArchiveName(collision, "coser-metadata/avatar.jpg", false); err == nil {
		t.Fatal("case-fold collision was accepted")
	}
}

func readBackupFixture(path string) ([]backupFixtureEntry, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	result := make([]backupFixtureEntry, 0, len(archive.File))
	for _, entry := range archive.File {
		reader, err := entry.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		result = append(result, backupFixtureEntry{name: entry.Name, mode: entry.Mode(), data: data})
	}
	return result, nil
}

func cloneFixture(entries []backupFixtureEntry) []backupFixtureEntry {
	result := make([]backupFixtureEntry, len(entries))
	for index, entry := range entries {
		result[index] = backupFixtureEntry{name: entry.name, mode: entry.mode, data: append([]byte(nil), entry.data...)}
	}
	return result
}

func appendFixture(name string, mode os.FileMode, data []byte) func([]backupFixtureEntry) []backupFixtureEntry {
	return func(entries []backupFixtureEntry) []backupFixtureEntry {
		return append(entries, backupFixtureEntry{name: name, mode: mode, data: data})
	}
}

func replaceFixture(name string, data []byte) func([]backupFixtureEntry) []backupFixtureEntry {
	return func(entries []backupFixtureEntry) []backupFixtureEntry {
		for index := range entries {
			if entries[index].name == name {
				entries[index].data = data
			}
		}
		return entries
	}
}

func removeFixture(name string) func([]backupFixtureEntry) []backupFixtureEntry {
	return func(entries []backupFixtureEntry) []backupFixtureEntry {
		result := entries[:0]
		for _, entry := range entries {
			if entry.name != name {
				result = append(result, entry)
			}
		}
		return result
	}
}

func writeBackupFixture(path string, entries []backupFixtureEntry) (returnErr error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
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
	for _, value := range entries {
		header := &zip.FileHeader{Name: value.name, Method: zip.Deflate}
		header.SetMode(value.mode)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := writer.Write(value.data); err != nil {
			return err
		}
	}
	return nil
}

func updateBackupFixtureRecord(ctx context.Context, db *Database, backupID, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	digest, err := applicationFileSHA256(path)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE backup_records SET byte_size=?,archive_sha256=? WHERE backup_id=?`, info.Size(), digest, backupID)
	return err
}
