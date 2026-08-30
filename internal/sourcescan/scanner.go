// Package sourcescan reads one GallerySource without mutating user media.
// Results are observations for the persistence staging transaction; media
// processing and derivative generation are intentionally separate jobs.
package sourcescan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/archivefile"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/zeebo/blake3"
	"golang.org/x/text/unicode/norm"
)

const sampleSize = 64 * 1024

type Observation struct {
	RelativePath     string
	MediaKind        gallery.MediaKind
	ContentFormat    gallery.ContentFormat
	ImageCategory    gallery.ImageCategory
	ByteSize         int64
	QuickFingerprint string
	FullFingerprint  string
	ProcessingState  gallery.ProcessingState
}

type Issue struct {
	Code         string
	RelativePath string
	Blocking     bool
	Message      string
}

type Result struct {
	Observations []Observation
	Issues       []Issue
	// Complete means the source was fully enumerated and is safe to commit as
	// a replacement snapshot. False results may update source diagnostics only.
	Complete bool
}

// ScanDirectory never follows symbolic links and only opens regular files
// located beneath root.
func ScanDirectory(ctx context.Context, root string) (Result, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	rootInfo, err := os.Lstat(absolute)
	if err != nil {
		return Result{}, err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return Result{}, errors.New("DIRECTORY GallerySource root must be a real directory, not a symbolic link")
	}

	result := Result{Complete: true}
	seen := make(map[string]string)
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
			relative, _ := filepath.Rel(absolute, filename)
			result.Issues = append(result.Issues, Issue{
				Code: "SYMLINK_IGNORED", RelativePath: filepath.ToSlash(relative), Blocking: true,
				Message: "symbolic links are not followed inside DIRECTORY sources",
			})
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
			result.Issues = append(result.Issues, Issue{
				Code: "SPECIAL_FILE_IGNORED", Blocking: true,
				Message: "special filesystem entries cannot become GalleryItems",
			})
			return nil
		}
		relative, err := canonicalRelativePath(absolute, filename)
		if err != nil {
			return err
		}
		if !supportedExtension(relative) {
			return nil
		}
		folded := strings.ToLower(relative)
		if prior, exists := seen[folded]; exists {
			result.Issues = append(result.Issues, Issue{
				Code: "AMBIGUOUS_MEDIA_PATH", RelativePath: relative, Blocking: true,
				Message: fmt.Sprintf("media path conflicts with %q after NFC/case folding", prior),
			})
			return nil
		}
		seen[folded] = relative
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		observation, issues, scanErr := observeReader(ctx, relative, info.Size(), file)
		closeErr := file.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return closeErr
		}
		result.Issues = append(result.Issues, issues...)
		if observation != nil {
			result.Observations = append(result.Observations, *observation)
		}
		return nil
	})
	return result, err
}

// ScanArchive validates the complete archive directory before reading media.
// ZIP records per-entry compressed bytes; TAR and solid 7z use uncompressed
// bytes because those formats do not expose a meaningful per-entry size.
func ScanArchive(ctx context.Context, filename string, limits archivecheck.Limits) (Result, error) {
	validation, err := archivecheck.ValidateFile(filename, limits)
	if err != nil {
		return Result{}, err
	}
	var result Result
	unsafeToRead := false
	for _, issue := range validation.Issues {
		result.Issues = append(result.Issues, Issue{
			Code: issue.Code, RelativePath: issue.EntryPath, Blocking: true, Message: issue.Message,
		})
		if issue.Code != "UNSUPPORTED_ARCHIVE_MEDIA" {
			unsafeToRead = true
		}
	}
	if unsafeToRead {
		return result, nil
	}
	result.Complete = true

	err = archivefile.Walk(filename, func(entry archivefile.Entry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || !supportedExtension(entry.Name) {
			return nil
		}
		part, err := entry.Open()
		if err != nil {
			return fmt.Errorf("opening archive Entry %q: %w", entry.Name, err)
		}
		storedSize := entry.CompressedSize
		if storedSize == 0 {
			storedSize = entry.UncompressedSize
		}
		if storedSize > math.MaxInt64 {
			_ = part.Close()
			return fmt.Errorf("archive Entry %q size exceeds supported range", entry.Name)
		}
		observation, issues, scanErr := observeReader(ctx, entry.Name, int64(storedSize), part)
		closeErr := part.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return closeErr
		}
		result.Issues = append(result.Issues, issues...)
		if observation != nil {
			result.Observations = append(result.Observations, *observation)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func observeReader(ctx context.Context, relative string, storedSize int64, reader io.Reader) (*Observation, []Issue, error) {
	header, quick, full, err := hashContent(ctx, reader, storedSize)
	if err != nil {
		return nil, nil, err
	}
	kind, contentFormat, recognized := detectContent(header, filepath.Ext(relative))
	if !recognized {
		return nil, []Issue{{
			Code: "UNRECOGNIZED_MEDIA_CONTENT", RelativePath: relative, Blocking: true,
			Message: "supported extension does not contain a recognized image, RAW or video format",
		}}, nil
	}
	var issues []Issue
	expected := extensionFamily(filepath.Ext(relative))
	actual := kindFamily(kind)
	if expected != "" && expected != actual && !(expected == "IMAGE" && actual == "ANIMATED") {
		issues = append(issues, Issue{
			Code: "CONTENT_EXTENSION_MISMATCH", RelativePath: relative, Blocking: true,
			Message: "actual media content does not match its filename extension and requires confirmation",
		})
	}
	category := gallery.ImageCategory("")
	if kind == gallery.MediaKindStaticImage {
		category = gallery.ImageCategoryPhoto
	}
	return &Observation{
		RelativePath: relative, MediaKind: kind, ContentFormat: contentFormat, ImageCategory: category,
		ByteSize: storedSize, QuickFingerprint: "blake3-sample-v1:" + quick,
		FullFingerprint: "blake3-v1:" + full, ProcessingState: gallery.ProcessingPending,
	}, issues, nil
}

func hashContent(ctx context.Context, reader io.Reader, storedSize int64) ([]byte, string, string, error) {
	fullHasher := blake3.New()
	first := make([]byte, 0, sampleSize)
	last := make([]byte, sampleSize)
	lastLength := 0
	total := int64(0)
	header := make([]byte, 0, 512)
	buffer := make([]byte, 128*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, "", "", err
		}
		count, readErr := reader.Read(buffer)
		if count > 0 {
			part := buffer[:count]
			_, _ = fullHasher.Write(part)
			if len(header) < cap(header) {
				take := min(count, cap(header)-len(header))
				header = append(header, part[:take]...)
			}
			if len(first) < cap(first) {
				take := min(count, cap(first)-len(first))
				first = append(first, part[:take]...)
			}
			if count >= sampleSize {
				copy(last, part[count-sampleSize:])
				lastLength = sampleSize
			} else {
				if lastLength+count > sampleSize {
					drop := lastLength + count - sampleSize
					copy(last, last[drop:lastLength])
					lastLength -= drop
				}
				copy(last[lastLength:], part)
				lastLength += count
			}
			total += int64(count)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, "", "", readErr
		}
	}
	quickHasher := blake3.New()
	var sizes [16]byte
	binary.LittleEndian.PutUint64(sizes[:8], uint64(total))
	binary.LittleEndian.PutUint64(sizes[8:], uint64(storedSize))
	_, _ = quickHasher.Write(sizes[:])
	_, _ = quickHasher.Write(first)
	_, _ = quickHasher.Write(last[:lastLength])
	return header,
		hex.EncodeToString(quickHasher.Sum(nil)),
		hex.EncodeToString(fullHasher.Sum(nil)), nil
}

func canonicalRelativePath(root string, filename string) (string, error) {
	relative, err := filepath.Rel(root, filename)
	if err != nil {
		return "", err
	}
	relative = norm.NFC.String(filepath.ToSlash(relative))
	if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || strings.Contains(relative, "/../") {
		return "", errors.New("media path escaped DIRECTORY GallerySource")
	}
	return relative, nil
}

// IsSupportedMediaPath reports whether a path is eligible for content
// inspection. Discovery uses it only to count potential media; ScanDirectory
// still verifies the actual bytes before creating GalleryItems.
func IsSupportedMediaPath(value string) bool {
	extension := strings.ToLower(filepath.Ext(value))
	_, ok := supportedExtensions[extension]
	return ok
}

func supportedExtension(value string) bool { return IsSupportedMediaPath(value) }

var supportedExtensions = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {}, ".avif": {}, ".jxl": {},
	".m4v": {}, ".mp4": {}, ".mov": {}, ".wmv": {}, ".avi": {}, ".mpg": {}, ".mpeg": {},
	".rmvb": {}, ".rm": {}, ".flv": {}, ".asf": {}, ".mkv": {}, ".webm": {}, ".f4v": {},
	".cr2": {}, ".cr3": {}, ".nef": {}, ".nrw": {}, ".arw": {}, ".dng": {}, ".raf": {},
	".rw2": {}, ".orf": {}, ".pef": {}, ".srw": {}, ".raw": {},
}

func detectContent(header []byte, extension string) (gallery.MediaKind, gallery.ContentFormat, bool) {
	extension = strings.ToLower(extension)
	if bytes.HasPrefix(header, []byte{0xff, 0xd8, 0xff}) || bytes.HasPrefix(header, []byte("\x89PNG\r\n\x1a\n")) {
		return gallery.MediaKindStaticImage, gallery.ContentFormatImage, true
	}
	if bytes.HasPrefix(header, []byte("GIF87a")) || bytes.HasPrefix(header, []byte("GIF89a")) {
		return gallery.MediaKindAnimatedImage, gallery.ContentFormatImage, true
	}
	if len(header) >= 12 && bytes.Equal(header[:4], []byte("RIFF")) && bytes.Equal(header[8:12], []byte("WEBP")) {
		if bytes.Contains(header[12:], []byte("ANIM")) {
			return gallery.MediaKindAnimatedImage, gallery.ContentFormatImage, true
		}
		return gallery.MediaKindStaticImage, gallery.ContentFormatImage, true
	}
	if isAVIF(header) || bytes.HasPrefix(header, []byte{0xff, 0x0a}) ||
		bytes.HasPrefix(header, []byte("\x00\x00\x00\x0cJXL \r\n\x87\n")) {
		return gallery.MediaKindStaticImage, gallery.ContentFormatImage, true
	}
	if isRawExtension(extension) && looksLikeRAW(header, extension) {
		return gallery.MediaKindStaticImage, gallery.ContentFormatRAW, true
	}
	if looksLikeVideo(header, extension) {
		return gallery.MediaKindVideo, gallery.ContentFormatVideo, true
	}
	return "", "", false
}

func isAVIF(header []byte) bool {
	return len(header) >= 12 && bytes.Equal(header[4:8], []byte("ftyp")) &&
		(bytes.Equal(header[8:12], []byte("avif")) || bytes.Equal(header[8:12], []byte("avis")))
}

func looksLikeRAW(header []byte, extension string) bool {
	if extension == ".cr3" {
		return len(header) >= 12 && bytes.Equal(header[4:8], []byte("ftyp")) && bytes.Equal(header[8:11], []byte("crx"))
	}
	return bytes.HasPrefix(header, []byte("II*\x00")) || bytes.HasPrefix(header, []byte("MM\x00*")) ||
		bytes.HasPrefix(header, []byte("FUJIFILMCCD-RAW")) || len(header) >= 16
}

func looksLikeVideo(header []byte, extension string) bool {
	if len(header) >= 12 && bytes.Equal(header[4:8], []byte("ftyp")) && !isAVIF(header) {
		return true
	}
	if bytes.HasPrefix(header, []byte{0x1a, 0x45, 0xdf, 0xa3}) || bytes.HasPrefix(header, []byte("FLV")) {
		return true
	}
	if len(header) >= 12 && bytes.Equal(header[:4], []byte("RIFF")) && bytes.Equal(header[8:12], []byte("AVI ")) {
		return true
	}
	if bytes.HasPrefix(header, []byte{0x00, 0x00, 0x01, 0xba}) || bytes.HasPrefix(header, []byte{0x47}) {
		return true
	}
	return extensionFamily(extension) == "VIDEO" && len(header) >= 4
}

func extensionFamily(extension string) string {
	extension = strings.ToLower(extension)
	if isRawExtension(extension) {
		return "IMAGE"
	}
	switch extension {
	case ".png", ".jpg", ".jpeg", ".webp", ".avif", ".jxl":
		return "IMAGE"
	case ".gif":
		return "ANIMATED"
	default:
		if _, ok := supportedExtensions[extension]; ok {
			return "VIDEO"
		}
	}
	return ""
}

func kindFamily(kind gallery.MediaKind) string {
	switch kind {
	case gallery.MediaKindStaticImage:
		return "IMAGE"
	case gallery.MediaKindAnimatedImage:
		return "ANIMATED"
	default:
		return "VIDEO"
	}
}

func isRawExtension(extension string) bool {
	switch strings.ToLower(extension) {
	case ".cr2", ".cr3", ".nef", ".nrw", ".arw", ".dng", ".raf", ".rw2", ".orf", ".pef", ".srw", ".raw":
		return true
	default:
		return false
	}
}
