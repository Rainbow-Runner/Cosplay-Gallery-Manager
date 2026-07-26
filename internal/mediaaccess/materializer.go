// Package mediaaccess provides internal-only, read-only access to one
// GalleryItem. It never exposes physical paths through an HTTP contract.
package mediaaccess

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/internal/gallery"
	"golang.org/x/text/unicode/norm"
)

type Source struct {
	Type         gallery.SourceType
	Path         string
	RelativePath string
}

type Materialized struct {
	Path    string
	cleanup func() error
}

func (value Materialized) Close() error {
	if value.cleanup == nil {
		return nil
	}
	return value.cleanup()
}

type Materializer struct {
	TemporaryRoot string
	MaximumBytes  int64
}

func (m Materializer) Open(ctx context.Context, source Source) (Materialized, error) {
	if err := validateRelative(source.RelativePath); err != nil {
		return Materialized{}, err
	}
	if source.Type == gallery.SourceTypeDirectory {
		return openDirectory(source)
	}
	if source.Type == gallery.SourceTypeArchive {
		return m.openArchive(ctx, source)
	}
	return Materialized{}, fmt.Errorf("unsupported GallerySource type %q", source.Type)
}

func openDirectory(source Source) (Materialized, error) {
	root, err := filepath.Abs(source.Path)
	if err != nil {
		return Materialized{}, err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return Materialized{}, err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return Materialized{}, errors.New("DIRECTORY source root must be a real directory")
	}
	target := filepath.Join(root, filepath.FromSlash(source.RelativePath))
	within, err := filepath.Rel(root, target)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return Materialized{}, errors.New("GalleryItem path escaped source")
	}
	for current := target; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return Materialized{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return Materialized{}, errors.New("GalleryItem path contains a symbolic link")
		}
		if current == target && !info.Mode().IsRegular() {
			return Materialized{}, errors.New("GalleryItem source is not a regular file")
		}
		if current == root {
			break
		}
		if filepath.Dir(current) == current {
			return Materialized{}, errors.New("GalleryItem path escaped source")
		}
	}
	return Materialized{Path: target}, nil
}

func (m Materializer) openArchive(ctx context.Context, source Source) (Materialized, error) {
	archiveInfo, err := os.Lstat(source.Path)
	if err != nil {
		return Materialized{}, err
	}
	if archiveInfo.Mode()&os.ModeSymlink != 0 || !archiveInfo.Mode().IsRegular() {
		return Materialized{}, errors.New("archive source must be a regular non-symlink file")
	}
	reader, err := zip.OpenReader(source.Path)
	if err != nil {
		return Materialized{}, err
	}
	defer reader.Close()
	var entry *zip.File
	for _, candidate := range reader.File {
		if norm.NFC.String(candidate.Name) == source.RelativePath {
			if entry != nil {
				return Materialized{}, errors.New("archive contains duplicate GalleryItem path")
			}
			entry = candidate
		}
	}
	if entry == nil {
		return Materialized{}, os.ErrNotExist
	}
	if entry.FileInfo().IsDir() || entry.Mode()&os.ModeSymlink != 0 {
		return Materialized{}, errors.New("archive GalleryItem is not a regular file")
	}
	maximum := m.MaximumBytes
	if maximum <= 0 {
		maximum = 2 << 30
	}
	if int64(entry.UncompressedSize64) > maximum {
		return Materialized{}, errors.New("archive GalleryItem exceeds materialization limit")
	}
	temporaryRoot, err := filepath.Abs(m.TemporaryRoot)
	if err != nil {
		return Materialized{}, err
	}
	rootInfo, err := os.Lstat(temporaryRoot)
	if err != nil {
		return Materialized{}, err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return Materialized{}, errors.New("temporary root must be a real directory")
	}
	input, err := entry.Open()
	if err != nil {
		return Materialized{}, err
	}
	defer input.Close()
	extension := filepath.Ext(source.RelativePath)
	temporary, err := os.CreateTemp(temporaryRoot, "cgm-media-*"+extension)
	if err != nil {
		return Materialized{}, err
	}
	name := temporary.Name()
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(name)
		}
	}()
	buffer := make([]byte, 128*1024)
	written := int64(0)
	for {
		if err := ctx.Err(); err != nil {
			return Materialized{}, err
		}
		count, readErr := input.Read(buffer)
		if count > 0 {
			written += int64(count)
			if written > maximum {
				return Materialized{}, errors.New("archive GalleryItem exceeds materialization limit")
			}
			if _, err := temporary.Write(buffer[:count]); err != nil {
				return Materialized{}, err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return Materialized{}, readErr
		}
	}
	if err := temporary.Sync(); err != nil {
		return Materialized{}, err
	}
	if err := temporary.Close(); err != nil {
		return Materialized{}, err
	}
	cleanup = false
	return Materialized{Path: name, cleanup: func() error { return os.Remove(name) }}, nil
}

func validateRelative(value string) error {
	if value == "" || value != norm.NFC.String(value) || strings.Contains(value, "\\") || filepath.IsAbs(value) || filepath.ToSlash(filepath.Clean(value)) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return errors.New("GalleryItem relative path is invalid")
	}
	return nil
}
