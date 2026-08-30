// Package mediaaccess provides internal-only, read-only access to one
// GalleryItem. It never exposes physical paths through an HTTP contract.
package mediaaccess

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/internal/archivefile"
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

// OpenDirectoryFile validates the complete DIRECTORY path and returns the
// already-open regular file. Callers stream this descriptor rather than
// reopening the path after authorization, limiting path-swap races.
func OpenDirectoryFile(source Source) (*os.File, os.FileInfo, error) {
	if err := validateRelative(source.RelativePath); err != nil {
		return nil, nil, err
	}
	if source.Type != gallery.SourceTypeDirectory {
		return nil, nil, errors.New("direct media access requires a DIRECTORY source")
	}
	materialized, err := openDirectory(source)
	if err != nil {
		return nil, nil, err
	}
	before, err := os.Lstat(materialized.Path)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(materialized.Path)
	if err != nil {
		return nil, nil, err
	}
	after, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, nil, errors.New("media source changed while opening")
	}
	return file, after, nil
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
	maximum := m.MaximumBytes
	if maximum <= 0 {
		maximum = 2 << 30
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
	matched := false
	name := ""
	defer func() {
		if name != "" {
			_ = os.Remove(name)
		}
	}()
	err = archivefile.Walk(source.Path, func(entry archivefile.Entry) error {
		if norm.NFC.String(entry.Name) != source.RelativePath {
			return nil
		}
		if matched {
			return errors.New("archive contains duplicate GalleryItem path")
		}
		matched = true
		if entry.IsDir() || entry.Mode&os.ModeSymlink != 0 || !entry.Mode.IsRegular() {
			return errors.New("archive GalleryItem is not a regular file")
		}
		if entry.UncompressedSize > uint64(maximum) {
			return errors.New("archive GalleryItem exceeds materialization limit")
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		defer input.Close()
		temporary, err := os.CreateTemp(temporaryRoot, "cgm-media-*"+filepath.Ext(source.RelativePath))
		if err != nil {
			return err
		}
		name = temporary.Name()
		buffer := make([]byte, 128*1024)
		written := int64(0)
		for {
			if err := ctx.Err(); err != nil {
				_ = temporary.Close()
				return err
			}
			count, readErr := input.Read(buffer)
			if count > 0 {
				written += int64(count)
				if written > maximum {
					_ = temporary.Close()
					return errors.New("archive GalleryItem exceeds materialization limit")
				}
				if _, err := temporary.Write(buffer[:count]); err != nil {
					_ = temporary.Close()
					return err
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				_ = temporary.Close()
				return readErr
			}
		}
		if err := temporary.Sync(); err != nil {
			_ = temporary.Close()
			return err
		}
		return temporary.Close()
	})
	if err != nil {
		return Materialized{}, err
	}
	if !matched {
		return Materialized{}, os.ErrNotExist
	}
	materializedPath := name
	result := Materialized{Path: materializedPath, cleanup: func() error { return os.Remove(materializedPath) }}
	name = ""
	return result, nil
}

func validateRelative(value string) error {
	if value == "" || value != norm.NFC.String(value) || strings.Contains(value, "\\") || filepath.IsAbs(value) || filepath.ToSlash(filepath.Clean(value)) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return errors.New("GalleryItem relative path is invalid")
	}
	return nil
}
