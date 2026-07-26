// Package archivecheck validates ZIP/CBZ sources without extracting them into
// a user media library. Structural checks are unconditional; resource limits
// are configurable but cannot disable path and entry safety rules.
package archivecheck

import (
	"archive/zip"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp"
	"golang.org/x/text/unicode/norm"
)

type Limits struct {
	MaxEntries           int
	MaxEntryUncompressed uint64
	MaxTotalUncompressed uint64
	MaxCompressionRatio  float64
	MaxImagePixels       uint64
}

func DefaultLimits() Limits {
	return Limits{
		MaxEntries:           20_000,
		MaxEntryUncompressed: 2 * 1024 * 1024 * 1024,
		MaxTotalUncompressed: 100 * 1024 * 1024 * 1024,
		MaxCompressionRatio:  1_000,
		MaxImagePixels:       200_000_000,
	}
}

func (limits Limits) Validate() error {
	if limits.MaxEntries <= 0 || limits.MaxEntryUncompressed == 0 ||
		limits.MaxTotalUncompressed == 0 || limits.MaxCompressionRatio <= 0 ||
		limits.MaxImagePixels == 0 {
		return errors.New("archive resource limits must all be positive")
	}
	return nil
}

type Issue struct {
	Code       string
	EntryPath  string
	Structural bool
	Message    string
}

type Result struct {
	EntryCount        int
	TotalUncompressed uint64
	Issues            []Issue
}

func (result Result) Safe() bool {
	return len(result.Issues) == 0
}

func ValidateFile(filename string, limits Limits) (Result, error) {
	if err := limits.Validate(); err != nil {
		return Result{}, err
	}
	extension := strings.ToLower(filepath.Ext(filename))
	if extension != ".zip" && extension != ".cbz" {
		return Result{}, fmt.Errorf("unsupported archive extension %q", extension)
	}
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return Result{}, fmt.Errorf("opening ZIP/CBZ: %w", err)
	}
	defer reader.Close()
	return Validate(reader.File, limits), nil
}

func Validate(files []*zip.File, limits Limits) Result {
	result := Result{EntryCount: len(files)}
	if len(files) > limits.MaxEntries {
		result.add("ENTRY_COUNT_LIMIT", "", false, "archive entry count exceeds configured limit")
	}
	seenPaths := make(map[string]string, len(files))
	for _, file := range files {
		entryPath, pathIssue := canonicalEntryPath(file.Name)
		if pathIssue != nil {
			result.Issues = append(result.Issues, *pathIssue)
			entryPath = file.Name
		}
		if prior, duplicate := seenPaths[strings.ToLower(norm.NFC.String(entryPath))]; duplicate {
			result.add("AMBIGUOUS_ENTRY_PATH", entryPath, true,
				fmt.Sprintf("entry conflicts with %q after NFC/case folding", prior))
		} else {
			seenPaths[strings.ToLower(norm.NFC.String(entryPath))] = entryPath
		}

		if file.Flags&0x1 != 0 {
			result.add("ENCRYPTED_ENTRY", entryPath, true, "encrypted ZIP entries are not supported")
		}
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 {
			result.add("SYMLINK_ENTRY", entryPath, true, "symbolic links are forbidden in archives")
		} else if !file.FileInfo().IsDir() && !mode.IsRegular() {
			result.add("SPECIAL_ENTRY", entryPath, true, "special filesystem entries are forbidden in archives")
		}
		if isNestedArchive(entryPath) {
			result.add("NESTED_ARCHIVE", entryPath, true, "nested archives are not supported")
		}
		if isBlockedArchiveMedia(entryPath) {
			result.add("UNSUPPORTED_ARCHIVE_MEDIA", entryPath, false,
				"video, RAW and AVIF entries must be excluded or imported as a DIRECTORY source")
		}

		uncompressed := file.UncompressedSize64
		if math.MaxUint64-result.TotalUncompressed < uncompressed {
			result.TotalUncompressed = math.MaxUint64
			result.add("TOTAL_SIZE_OVERFLOW", entryPath, false, "archive size total overflowed")
		} else {
			result.TotalUncompressed += uncompressed
		}
		if uncompressed > limits.MaxEntryUncompressed {
			result.add("ENTRY_SIZE_LIMIT", entryPath, false, "entry uncompressed size exceeds configured limit")
		}
		if !file.FileInfo().IsDir() && uncompressed > 0 {
			if file.CompressedSize64 == 0 || float64(uncompressed)/float64(file.CompressedSize64) > limits.MaxCompressionRatio {
				result.add("COMPRESSION_RATIO_LIMIT", entryPath, false, "entry compression ratio exceeds configured limit")
			}
		}
		if isProbeableImage(entryPath) && uncompressed <= limits.MaxEntryUncompressed {
			part, err := file.Open()
			if err == nil {
				config, _, decodeErr := image.DecodeConfig(part)
				_ = part.Close()
				if decodeErr == nil && imagePixelsOverLimit(config.Width, config.Height, limits.MaxImagePixels) {
					result.add("IMAGE_PIXEL_LIMIT", entryPath, false, "image dimensions exceed configured pixel limit")
				}
			}
		}
	}
	if result.TotalUncompressed > limits.MaxTotalUncompressed {
		result.add("TOTAL_SIZE_LIMIT", "", false, "archive total uncompressed size exceeds configured limit")
	}
	return result
}

func isProbeableImage(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func imagePixelsOverLimit(width int, height int, limit uint64) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	return uint64(width) > limit/uint64(height)
}

func canonicalEntryPath(value string) (string, *Issue) {
	normalized := norm.NFC.String(value)
	if value == "" || normalized != value || strings.Contains(value, "\\") ||
		strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") ||
		hasWindowsVolumePrefix(value) || path.Clean(value) != value ||
		value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return "", &Issue{
			Code: "UNSAFE_ENTRY_PATH", EntryPath: value, Structural: true,
			Message: "archive entry path must be an NFC forward-slash relative path without traversal",
		}
	}
	return normalized, nil
}

func hasWindowsVolumePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') ||
		(value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':'
}

func isNestedArchive(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".zip", ".cbz", ".rar", ".7z":
		return true
	default:
		return false
	}
}

func isBlockedArchiveMedia(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".avif", ".mp4", ".m4v", ".mkv", ".mov", ".avi", ".webm", ".wmv",
		".3gp", ".mts", ".m2ts", ".cr2", ".cr3", ".nef", ".nrw", ".arw",
		".dng", ".raf", ".rw2", ".orf", ".pef", ".srw", ".raw":
		return true
	default:
		return false
	}
}

func (result *Result) add(code string, entryPath string, structural bool, message string) {
	result.Issues = append(result.Issues, Issue{
		Code: code, EntryPath: entryPath, Structural: structural, Message: message,
	})
}
