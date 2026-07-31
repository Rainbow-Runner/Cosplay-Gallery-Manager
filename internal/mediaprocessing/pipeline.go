package mediaprocessing

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/zeebo/blake3"
)

type GenerateRequest struct {
	ItemUUID        string
	MediaKind       gallery.MediaKind
	ContentFormat   gallery.ContentFormat
	ContentRevision int64
	Variant         string
	ProfileHash     string
	SourcePath      string
	DestinationPath string
}

type GenerateResult struct {
	MIMEType string
	Width    int
	Height   int
	ByteSize int64
}

// Generator is the internal code-level extension boundary retained after the
// original runtime plugin system was removed. Implementations wrap the reused
// Stash image/FFmpeg/LibRaw capabilities and never choose business identity.
type Generator interface {
	Supports(GenerateRequest) bool
	Generate(context.Context, GenerateRequest) (GenerateResult, error)
}

type PrimaryPlan struct {
	Variant   string
	CacheTier CacheTier
	Static    bool
}

func PlanPrimary(kind gallery.MediaKind, format gallery.ContentFormat) (PrimaryPlan, error) {
	switch {
	case kind == gallery.MediaKindStaticImage && format == gallery.ContentFormatRAW:
		return PrimaryPlan{Variant: VariantCard480, CacheTier: CacheBase, Static: true}, nil
	case kind == gallery.MediaKindStaticImage && format == gallery.ContentFormatImage:
		return PrimaryPlan{Variant: VariantCard480, CacheTier: CacheBase, Static: true}, nil
	case kind == gallery.MediaKindAnimatedImage && format == gallery.ContentFormatImage:
		return PrimaryPlan{Variant: VariantStaticPoster, CacheTier: CacheBase, Static: true}, nil
	case kind == gallery.MediaKindVideo && format == gallery.ContentFormatVideo:
		return PrimaryPlan{Variant: VariantStaticPoster, CacheTier: CacheBase, Static: true}, nil
	default:
		return PrimaryPlan{}, errors.New("unsupported media kind/content format combination")
	}
}

func IsScrubberVariant(kind gallery.MediaKind, variant string) bool {
	if kind == gallery.MediaKindStaticImage {
		return variant == VariantCard480
	}
	return (kind == gallery.MediaKindAnimatedImage || kind == gallery.MediaKindVideo) && variant == VariantStaticPoster
}

var extensionPattern = regexp.MustCompile(`^[a-z0-9]{1,10}$`)

type CacheWriter struct{ Root string }

// OpenGenerated opens a database-authorized application cache artifact. It
// refuses symbolic links in both the file and every parent component so an
// opaque resource URL can never be turned into a user-media path escape.
func (writer CacheWriter) OpenGenerated(relative string) (*os.File, os.FileInfo, error) {
	root, target, err := writer.resolve(relative)
	if err != nil {
		return nil, nil, err
	}
	if err := validateExistingRealDirectory(root, filepath.Dir(target)); err != nil {
		return nil, nil, err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, errors.New("cache target is not a regular non-symlink file")
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, nil, errors.New("cache target changed while opening")
	}
	return file, opened, nil
}

func (writer CacheWriter) RelativePath(itemUUID string, contentRevision int64, variant, profileHash, extension string) (string, error) {
	if _, err := portableid.Parse(itemUUID); err != nil {
		return "", err
	}
	if contentRevision <= 0 || variant == "" || len(variant) > 100 || profileHash == "" || !extensionPattern.MatchString(extension) {
		return "", errors.New("invalid derivative cache identity")
	}
	profileSum := blake3.Sum256([]byte(profileHash))
	profileKey := hex.EncodeToString(profileSum[:8])
	variantKey := strings.ToLower(strings.ReplaceAll(variant, "_", "-"))
	return filepath.ToSlash(filepath.Join("items", itemUUID[:2], itemUUID, fmt.Sprintf("r%d", contentRevision), profileKey, variantKey+"."+extension)), nil
}

// WriteAtomic publishes only application-generated cache bytes. Existing
// current resources remain intact until the final same-directory rename.
func (writer CacheWriter) WriteAtomic(relative string, produce func(io.Writer) error) (string, int64, error) {
	return writer.writeAtomic(relative, false, func(file *os.File, _ string) error { return produce(file) })
}

// WriteAtomicPath supports reused Stash/FFmpeg generators that require an
// output filename. The path is a private temporary cache path and is renamed
// into place only after successful generation and fsync.
func (writer CacheWriter) WriteAtomicPath(relative string, produce func(string) error) (string, int64, error) {
	return writer.writeAtomic(relative, true, func(file *os.File, path string) error {
		if err := file.Close(); err != nil {
			return err
		}
		return produce(path)
	})
}

func (writer CacheWriter) writeAtomic(relative string, pathProducer bool, produce func(*os.File, string) error) (string, int64, error) {
	root, target, err := writer.resolve(relative)
	if err != nil {
		return "", 0, err
	}
	if err := ensureRealDirectory(root, filepath.Dir(target)); err != nil {
		return "", 0, err
	}
	temporaryPattern := ".derivative-*.tmp"
	if pathProducer {
		temporaryPattern = ".derivative-*" + filepath.Ext(target)
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), temporaryPattern)
	if err != nil {
		return "", 0, err
	}
	temporaryName := temporary.Name()
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return "", 0, err
	}
	if err := produce(temporary, temporaryName); err != nil {
		return "", 0, err
	}
	if pathProducer {
		info, err := os.Lstat(temporaryName)
		if err != nil {
			return "", 0, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", 0, errors.New("generated derivative is not a regular file")
		}
		reopened, err := os.OpenFile(temporaryName, os.O_RDWR, 0)
		if err != nil {
			return "", 0, err
		}
		temporary = reopened
	}
	if err := temporary.Sync(); err != nil {
		return "", 0, err
	}
	info, err := temporary.Stat()
	if err != nil {
		return "", 0, err
	}
	if err := temporary.Close(); err != nil {
		return "", 0, err
	}
	if existing, err := os.Lstat(target); err == nil && (existing.Mode()&os.ModeSymlink != 0 || !existing.Mode().IsRegular()) {
		return "", 0, errors.New("derivative target is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", 0, err
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return "", 0, err
	}
	cleanup = false
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return "", 0, err
	}
	err = directory.Sync()
	_ = directory.Close()
	if err != nil {
		return "", 0, err
	}
	return target, info.Size(), nil
}

// RemoveEnhanced is intentionally the only deletion helper here. Its caller
// must first prove the database record belongs to the ENHANCED cache tier.
func (writer CacheWriter) RemoveEnhanced(relative string) error {
	return writer.removeGenerated(relative)
}

// RemoveUnpublished cleans an application-generated artifact when database
// publication fails. It is never called for a user media source.
func (writer CacheWriter) RemoveUnpublished(relative string) error {
	return writer.removeGenerated(relative)
}

func (writer CacheWriter) removeGenerated(relative string) error {
	_, target, err := writer.resolve(relative)
	if err != nil {
		return err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("cache target is not a regular non-symlink file")
	}
	return os.Remove(target)
}

func (writer CacheWriter) resolve(relative string) (string, string, error) {
	root, err := filepath.Abs(writer.Root)
	if err != nil {
		return "", "", err
	}
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") || filepath.Clean(relative) != relative || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", errors.New("cache path must be a canonical relative path")
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	within, err := filepath.Rel(root, target)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", "", errors.New("cache path escaped root")
	}
	return root, target, nil
}

func ensureRealDirectory(root, directory string) error {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("cache root must be a real directory")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	current := directory
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("cache path contains a non-directory or symbolic link")
		}
		if current == root {
			return nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("cache directory escaped root")
		}
		current = parent
	}
}

func validateExistingRealDirectory(root, directory string) error {
	current := directory
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("cache path contains a non-directory or symbolic link")
		}
		if current == root {
			return nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("cache directory escaped root")
		}
		current = parent
	}
}
