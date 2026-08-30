// Package archivecheck validates supported archive sources without extracting them into
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
	"io"
	"math"
	"os"
	"path"
	"strings"

	"github.com/stashapp/stash/internal/archivefile"
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
	result := Result{}
	seenPaths := make(map[string]string)
	err := archivefile.Walk(filename, func(entry archivefile.Entry) error {
		result.EntryCount++
		validateEntry(&result, seenPaths, entry, limits)
		return nil
	})
	if err != nil {
		if archivefile.IsEncryptedError(err) {
			result.add("ENCRYPTED_ARCHIVE", "", true, "password-protected archives are not supported")
			return result, nil
		}
		return Result{}, err
	}
	finishValidation(&result, filename, limits)
	return result, nil
}

func Validate(files []*zip.File, limits Limits) Result {
	result := Result{}
	seenPaths := make(map[string]string, len(files))
	for _, file := range files {
		result.EntryCount++
		validateEntry(&result, seenPaths, archivefile.Entry{Name: file.Name, Mode: file.Mode(),
			UncompressedSize: file.UncompressedSize64, CompressedSize: file.CompressedSize64,
			Encrypted: file.Flags&0x1 != 0, OpenReader: file.Open}, limits)
	}
	finishValidation(&result, "", limits)
	return result
}

func validateEntry(result *Result, seenPaths map[string]string, entry archivefile.Entry, limits Limits) {
	entryName := entry.Name
	if entry.IsDir() {
		entryName = strings.TrimSuffix(entryName, "/")
	}
	entryPath, pathIssue := canonicalEntryPath(entryName)
	if pathIssue != nil {
		result.Issues = append(result.Issues, *pathIssue)
		entryPath = entryName
	}
	if prior, duplicate := seenPaths[strings.ToLower(norm.NFC.String(entryPath))]; duplicate {
		result.add("AMBIGUOUS_ENTRY_PATH", entryPath, true,
			fmt.Sprintf("entry conflicts with %q after NFC/case folding", prior))
	} else {
		seenPaths[strings.ToLower(norm.NFC.String(entryPath))] = entryPath
	}
	if entry.Encrypted {
		result.add("ENCRYPTED_ENTRY", entryPath, true, "password-protected archive entries are not supported")
	}
	if entry.Mode&os.ModeSymlink != 0 {
		result.add("SYMLINK_ENTRY", entryPath, true, "symbolic links are forbidden in archives")
	} else if !entry.IsDir() && !entry.Mode.IsRegular() {
		result.add("SPECIAL_ENTRY", entryPath, true, "special filesystem entries are forbidden in archives")
	}
	if isNestedArchive(entryPath) {
		result.add("NESTED_ARCHIVE", entryPath, true, "nested archives are not supported")
	}
	if isBlockedArchiveMedia(entryPath) {
		result.add("UNSUPPORTED_ARCHIVE_MEDIA", entryPath, false,
			"video, RAW and AVIF entries must be excluded or imported as a DIRECTORY source")
	}
	if math.MaxUint64-result.TotalUncompressed < entry.UncompressedSize {
		result.TotalUncompressed = math.MaxUint64
		result.add("TOTAL_SIZE_OVERFLOW", entryPath, false, "archive size total overflowed")
	} else {
		result.TotalUncompressed += entry.UncompressedSize
	}
	if entry.UncompressedSize > limits.MaxEntryUncompressed {
		result.add("ENTRY_SIZE_LIMIT", entryPath, false, "entry uncompressed size exceeds configured limit")
	}
	if !entry.IsDir() && entry.UncompressedSize > 0 && entry.CompressedSize > 0 &&
		float64(entry.UncompressedSize)/float64(entry.CompressedSize) > limits.MaxCompressionRatio {
		result.add("COMPRESSION_RATIO_LIMIT", entryPath, false, "entry compression ratio exceeds configured limit")
	}
	if entry.Encrypted || entry.IsDir() || !entry.Mode.IsRegular() || entry.UncompressedSize > limits.MaxEntryUncompressed {
		return
	}
	part, err := entry.Open()
	if err != nil {
		if archivefile.IsEncryptedError(err) {
			result.add("ENCRYPTED_ARCHIVE", entryPath, true, "password-protected archives are not supported")
		} else {
			result.add("ARCHIVE_ENTRY_UNREADABLE", entryPath, true, "archive entry could not be opened")
		}
		return
	}
	if !isProbeableImage(entryPath) {
		var probe [1]byte
		_, readErr := part.Read(probe[:])
		closeErr := part.Close()
		if archivefile.IsEncryptedError(readErr) || archivefile.IsEncryptedError(closeErr) {
			result.add("ENCRYPTED_ARCHIVE", entryPath, true, "password-protected archives are not supported")
		} else if readErr != nil && !errors.Is(readErr, io.EOF) {
			result.add("ARCHIVE_ENTRY_UNREADABLE", entryPath, true, "archive entry could not be read")
		}
		return
	}
	config, _, decodeErr := image.DecodeConfig(part)
	closeErr := part.Close()
	if archivefile.IsEncryptedError(decodeErr) || archivefile.IsEncryptedError(closeErr) {
		result.add("ENCRYPTED_ARCHIVE", entryPath, true, "password-protected archives are not supported")
		return
	}
	if decodeErr == nil && imagePixelsOverLimit(config.Width, config.Height, limits.MaxImagePixels) {
		result.add("IMAGE_PIXEL_LIMIT", entryPath, false, "image dimensions exceed configured pixel limit")
	}
}

func finishValidation(result *Result, filename string, limits Limits) {
	if result.EntryCount > limits.MaxEntries {
		result.add("ENTRY_COUNT_LIMIT", "", false, "archive entry count exceeds configured limit")
	}
	if result.TotalUncompressed > limits.MaxTotalUncompressed {
		result.add("TOTAL_SIZE_LIMIT", "", false, "archive total uncompressed size exceeds configured limit")
	}
	if filename == "" || result.TotalUncompressed == 0 {
		return
	}
	info, err := os.Stat(filename)
	if err == nil && info.Size() > 0 && float64(result.TotalUncompressed)/float64(info.Size()) > limits.MaxCompressionRatio {
		result.add("COMPRESSION_RATIO_LIMIT", "", false, "archive total compression ratio exceeds configured limit")
	}
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
	return archivefile.IsArchiveLikePath(value)
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
