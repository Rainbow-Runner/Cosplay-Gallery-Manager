package manifest

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/zeebo/blake3"
	"golang.org/x/text/unicode/norm"
)

func GalleryPath(sourceType gallery.SourceType, sourcePath string) (string, error) {
	absolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", err
	}
	if sourceType == gallery.SourceTypeDirectory {
		return filepath.Join(absolute, ".cosplay.json"), nil
	}
	if sourceType == gallery.SourceTypeArchive {
		return absolute + ".cosplay.json", nil
	}
	return "", fmt.Errorf("unsupported GallerySource type %q", sourceType)
}

func GalleryManagedCoverPath(sourceType gallery.SourceType, sourcePath, extension string) (string, error) {
	extension = strings.ToLower(strings.TrimPrefix(extension, "."))
	if extension != "jpg" && extension != "jpeg" && extension != "png" && extension != "webp" {
		return "", errors.New("managed cover must use JPEG, PNG or static WebP")
	}
	absolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", err
	}
	if sourceType == gallery.SourceTypeDirectory {
		return filepath.Join(absolute, ".cosplay-assets", "cover."+extension), nil
	}
	if sourceType == gallery.SourceTypeArchive {
		return filepath.Join(absolute+".cosplay-assets", "cover."+extension), nil
	}
	return "", fmt.Errorf("unsupported GallerySource type %q", sourceType)
}

func CoserPath(metadataRoot string, coserUUID string) (string, error) {
	if _, err := portableid.Parse(coserUUID); err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(metadataRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(absolute, coserUUID, "coser.json"), nil
}

func CoserRedirectPath(metadataRoot string, coserUUID string) (string, error) {
	manifestPath, err := CoserPath(metadataRoot, coserUUID)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(manifestPath), "redirect_to_uuid"), nil
}

// WriteCoserRedirect leaves the retired Coser directory as a portable pointer
// to the permanent UUID Alias target. The file is intentionally independent
// from coser.json.
func WriteCoserRedirect(metadataRoot, sourceUUID, targetUUID string) (string, error) {
	if sourceUUID == targetUUID {
		return "", errors.New("Coser redirect source and target must differ")
	}
	if _, err := portableid.Parse(targetUUID); err != nil {
		return "", err
	}
	if _, err := EnsureCoserDirectory(metadataRoot, sourceUUID); err != nil {
		return "", err
	}
	filename, err := CoserRedirectPath(metadataRoot, sourceUUID)
	if err != nil {
		return "", err
	}
	_, err = WriteAtomic(filename, []byte(targetUUID+"\n"), 128)
	return filename, err
}

func ReadCoserRedirect(metadataRoot, sourceUUID string) (string, error) {
	filename, err := CoserRedirectPath(metadataRoot, sourceUUID)
	if err != nil {
		return "", err
	}
	data, _, err := ReadFile(filename, 128)
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(string(data))
	if _, err := portableid.Parse(target); err != nil {
		return "", err
	}
	return target, nil
}

func EnsureCoserDirectory(metadataRoot string, coserUUID string) (string, error) {
	filename, err := CoserPath(metadataRoot, coserUUID)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(metadataRoot)
	if err != nil {
		return "", err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", errors.New("Coser metadata root must be a real directory")
	}
	directory := filepath.Dir(filename)
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("Coser metadata directory must be a real directory")
	}
	return filename, nil
}

func ReadFile(filename string, maximum int64) ([]byte, string, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, "", errors.New("Manifest must be a regular non-symlink file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	data, err := readLimited(file, maximum)
	if err != nil {
		return nil, "", err
	}
	return data, Hash(data), nil
}

func Hash(data []byte) string {
	sum := blake3.Sum256(data)
	return "blake3-v1:" + hex.EncodeToString(sum[:])
}

// WriteAtomic writes an application-managed Manifest snapshot beside its
// source. It retains exactly one .bak and never rewrites an archive.
func WriteAtomic(filename string, data []byte, maximum int64) (string, error) {
	if int64(len(data)) > maximum {
		return "", fmt.Errorf("Manifest exceeds %d byte limit", maximum)
	}
	if filename != norm.NFC.String(filename) {
		return "", errors.New("Manifest path must use NFC")
	}
	directory := filepath.Dir(filename)
	info, err := os.Lstat(directory)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("Manifest parent must be a real directory")
	}
	if existing, err := os.Lstat(filename); err == nil {
		if existing.Mode()&os.ModeSymlink != 0 || !existing.Mode().IsRegular() {
			return "", errors.New("existing Manifest must be a regular non-symlink file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	temporary, err := os.CreateTemp(directory, ".cosplay-manifest-*.tmp")
	if err != nil {
		return "", err
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return "", err
	}
	if _, err := temporary.Write(data); err != nil {
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}

	backup := filename + ".bak"
	hadExisting := false
	if _, err := os.Lstat(filename); err == nil {
		hadExisting = true
		if backupInfo, backupErr := os.Lstat(backup); backupErr == nil {
			if backupInfo.Mode()&os.ModeSymlink != 0 || !backupInfo.Mode().IsRegular() {
				return "", errors.New("Manifest backup must be a regular non-symlink file")
			}
			if err := os.Remove(backup); err != nil {
				return "", err
			}
		} else if !errors.Is(backupErr, os.ErrNotExist) {
			return "", backupErr
		}
		if err := os.Rename(filename, backup); err != nil {
			return "", err
		}
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		if hadExisting {
			_ = os.Rename(backup, filename)
		}
		return "", err
	}
	removeTemporary = false
	if err := syncDirectory(directory); err != nil {
		return "", err
	}
	return Hash(data), nil
}

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func ValidateManagedRelativeAsset(value string) error {
	if err := validateRelativePath(value); err != nil {
		return err
	}
	if strings.HasPrefix(value, ".cosplay.json") {
		return errors.New("asset path collides with Manifest filename")
	}
	return nil
}
