package sourcescan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/archivefile"
)

type ModificationEvidence struct {
	AtUTC  string
	Status string
	Origin string
}

// ScanDirectoryModificationTimes is the lightweight historical backfill path.
// It reads directory metadata only and never opens or decodes media content.
func ScanDirectoryModificationTimes(ctx context.Context, root string) (map[string]ModificationEvidence, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	rootInfo, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, errors.New("DIRECTORY GallerySource root must be a real directory, not a symbolic link")
	}
	result := map[string]ModificationEvidence{}
	err = filepath.WalkDir(absolute, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if filename == absolute {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !supportedExtension(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := canonicalRelativePath(absolute, filename)
		if err != nil {
			return err
		}
		result[relative] = modificationEvidence(info.ModTime(), "FILESYSTEM")
		return nil
	})
	return result, err
}

// ScanArchiveModificationTimes enumerates one archive once and never opens a
// member body. Missing member timestamps remain explicit NONE evidence.
func ScanArchiveModificationTimes(ctx context.Context, filename string, limits archivecheck.Limits) (map[string]ModificationEvidence, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	result := map[string]ModificationEvidence{}
	entryCount := 0
	var total uint64
	err := archivefile.Walk(filename, func(entry archivefile.Entry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entryCount++
		if entryCount > limits.MaxEntries || entry.UncompressedSize > limits.MaxEntryUncompressed || entry.UncompressedSize > limits.MaxTotalUncompressed ||
			total > limits.MaxTotalUncompressed-entry.UncompressedSize {
			return errors.New("archive exceeds modification-time backfill limits")
		}
		total += entry.UncompressedSize
		if entry.Encrypted || entry.Mode&os.ModeSymlink != 0 || (!entry.IsDir() && !entry.Mode.IsRegular()) {
			return errors.New("archive is unsafe for modification-time backfill")
		}
		if entry.IsDir() || !supportedExtension(entry.Name) {
			return nil
		}
		if entry.ModifiedKnown {
			result[entry.Name] = modificationEvidence(entry.Modified, "ARCHIVE_ENTRY")
		} else {
			result[entry.Name] = ModificationEvidence{Status: "NONE"}
		}
		return nil
	})
	return result, err
}

func modificationEvidence(value time.Time, origin string) ModificationEvidence {
	if value.IsZero() {
		return ModificationEvidence{Status: "NONE"}
	}
	return ModificationEvidence{AtUTC: value.UTC().Format(time.RFC3339Nano), Status: "FOUND", Origin: origin}
}
