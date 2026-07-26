package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/library"
)

var ErrMediaLibraryNotFound = errors.New("media library not found")

type LibraryStore struct {
	db *sql.DB
}

func (db *Database) Libraries() *LibraryStore {
	return &LibraryStore{db: db.DB}
}

type CreateLibraryInput struct {
	Name            string
	RootPath        string
	Enabled         bool
	ReadOnly        bool
	CaptureTimezone string
}

func (s *LibraryStore) Create(
	ctx context.Context,
	input CreateLibraryInput,
	now time.Time,
) (library.Library, error) {
	root, err := canonicalLibraryRoot(input.RootPath)
	if err != nil {
		return library.Library{}, err
	}
	if input.Name == "" || len([]rune(input.Name)) > 300 {
		return library.Library{}, errors.New("media library name must contain 1 to 300 characters")
	}
	if input.CaptureTimezone == "" {
		input.CaptureTimezone = "UTC"
	}
	if _, err := time.LoadLocation(input.CaptureTimezone); err != nil {
		return library.Library{}, fmt.Errorf("invalid capture timezone %q: %w", input.CaptureTimezone, err)
	}

	timestamp := formatTime(normalisedTime(now))
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO media_libraries (
			name, root_path, enabled, read_only, capture_timezone,
			created_at_utc, updated_at_utc
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, input.Name, root, input.Enabled, input.ReadOnly, input.CaptureTimezone, timestamp, timestamp)
	if err != nil {
		return library.Library{}, fmt.Errorf("creating media library: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return library.Library{}, err
	}
	return findLibrary(ctx, s.db, id)
}

func (s *LibraryStore) Find(ctx context.Context, id int64) (library.Library, error) {
	return findLibrary(ctx, s.db, id)
}

func (s *LibraryStore) List(ctx context.Context) ([]library.Library, error) {
	return s.list(ctx)
}

// OwnerForPath returns the most specific configured root. Disabled libraries
// intentionally remain ownership boundaries until explicitly removed.
func (s *LibraryStore) OwnerForPath(ctx context.Context, candidate string) (*library.Library, error) {
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return nil, err
	}
	absolute = filepath.Clean(absolute)
	libraries, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(libraries, func(i int, j int) bool {
		return len(libraries[i].RootPath) > len(libraries[j].RootPath)
	})
	for _, candidateLibrary := range libraries {
		if pathWithin(candidateLibrary.RootPath, absolute) {
			owned := candidateLibrary
			return &owned, nil
		}
	}
	return nil, nil
}

// ChildBoundaries returns configured child roots a parent scan must skip.
func (s *LibraryStore) ChildBoundaries(ctx context.Context, parentID int64) ([]library.Library, error) {
	parent, err := s.Find(ctx, parentID)
	if err != nil {
		return nil, err
	}
	libraries, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var children []library.Library
	for _, candidate := range libraries {
		if candidate.ID != parent.ID && pathWithin(parent.RootPath, candidate.RootPath) {
			children = append(children, candidate)
		}
	}
	sort.Slice(children, func(i int, j int) bool {
		return children[i].RootPath < children[j].RootPath
	})
	return children, nil
}

// PreviewChange is read-only. A root change or deletion cannot silently move
// GallerySources to a parent library.
func (s *LibraryStore) PreviewChange(
	ctx context.Context,
	libraryID int64,
	newRoot string,
) (library.ChangePreview, error) {
	current, err := s.Find(ctx, libraryID)
	if err != nil {
		return library.ChangePreview{}, err
	}
	if newRoot != "" {
		newRoot, err = canonicalLibraryRoot(newRoot)
		if err != nil {
			return library.ChangePreview{}, err
		}
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, gallery_id, source_path
		FROM gallery_sources WHERE library_id = ? ORDER BY id
	`, libraryID)
	if err != nil {
		return library.ChangePreview{}, err
	}
	defer rows.Close()

	preview := library.ChangePreview{LibraryID: current.ID, NewRoot: newRoot}
	for rows.Next() {
		var impact library.SourceImpact
		impact.CurrentLibrary = current.ID
		if err := rows.Scan(&impact.SourceID, &impact.GalleryID, &impact.SourcePath); err != nil {
			return library.ChangePreview{}, err
		}
		preview.Impacts = append(preview.Impacts, impact)
	}
	if err := rows.Err(); err != nil {
		return library.ChangePreview{}, err
	}
	if err := rows.Close(); err != nil {
		return library.ChangePreview{}, err
	}
	for index := range preview.Impacts {
		owner, err := s.ownerExcluding(ctx, preview.Impacts[index].SourcePath, libraryID, newRoot)
		if err != nil {
			return library.ChangePreview{}, err
		}
		if owner != nil {
			value := owner.ID
			preview.Impacts[index].SuggestedOwner = &value
		}
	}
	return preview, nil
}

// AssignSource performs the explicit transfer selected after preview. Passing
// nil leaves the Source unassigned; it is never adopted by a parent implicitly.
func (s *LibraryStore) AssignSource(ctx context.Context, sourceID int64, targetLibraryID *int64) error {
	var sourcePath string
	if err := s.db.QueryRowContext(ctx, `SELECT source_path FROM gallery_sources WHERE id = ?`, sourceID).Scan(&sourcePath); err != nil {
		return err
	}
	if targetLibraryID != nil {
		target, err := s.Find(ctx, *targetLibraryID)
		if err != nil {
			return err
		}
		if !pathWithin(target.RootPath, sourcePath) {
			return fmt.Errorf("GallerySource path %q is outside target media library %q", sourcePath, target.RootPath)
		}
	}
	_, err := s.db.ExecContext(ctx, `UPDATE gallery_sources SET library_id = ? WHERE id = ?`, targetLibraryID, sourceID)
	return err
}

func (s *LibraryStore) ownerExcluding(
	ctx context.Context,
	candidate string,
	excludedID int64,
	replacementRoot string,
) (*library.Library, error) {
	libraries, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	for index := range libraries {
		if libraries[index].ID == excludedID {
			libraries[index].RootPath = replacementRoot
		}
	}
	sort.Slice(libraries, func(i int, j int) bool {
		return len(libraries[i].RootPath) > len(libraries[j].RootPath)
	})
	for _, candidateLibrary := range libraries {
		if candidateLibrary.RootPath != "" && pathWithin(candidateLibrary.RootPath, candidate) {
			owned := candidateLibrary
			return &owned, nil
		}
	}
	return nil, nil
}

func (s *LibraryStore) list(ctx context.Context) ([]library.Library, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, root_path, enabled, read_only, capture_timezone,
			created_at_utc, updated_at_utc
		FROM media_libraries ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []library.Library
	for rows.Next() {
		value, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, value)
	}
	return results, rows.Err()
}

func findLibrary(ctx context.Context, queryer galleryQueryer, id int64) (library.Library, error) {
	row := queryer.QueryRowContext(ctx, `
		SELECT id, name, root_path, enabled, read_only, capture_timezone,
			created_at_utc, updated_at_utc
		FROM media_libraries WHERE id = ?
	`, id)
	result, err := scanLibrary(row)
	if errors.Is(err, sql.ErrNoRows) {
		return library.Library{}, ErrMediaLibraryNotFound
	}
	return result, err
}

type libraryScanner interface {
	Scan(...interface{}) error
}

func scanLibrary(scanner libraryScanner) (library.Library, error) {
	var result library.Library
	var enabled, readOnly int
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&result.ID, &result.Name, &result.RootPath, &enabled, &readOnly,
		&result.CaptureTimezone, &createdAt, &updatedAt,
	); err != nil {
		return library.Library{}, err
	}
	result.Enabled = enabled == 1
	result.ReadOnly = readOnly == 1
	var err error
	if result.CreatedAtUTC, err = parseTime(createdAt); err != nil {
		return library.Library{}, err
	}
	if result.UpdatedAtUTC, err = parseTime(updatedAt); err != nil {
		return library.Library{}, err
	}
	return result, nil
}

func canonicalLibraryRoot(value string) (string, error) {
	if value == "" {
		return "", errors.New("media library root is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if len([]rune(absolute)) > 4096 {
		return "", errors.New("media library root exceeds 4096 characters")
	}
	info, err := os.Lstat(absolute)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("media library root cannot be a symbolic link")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return absolute, nil
}

func pathWithin(root string, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !startsWithParent(relative)
}

func startsWithParent(relative string) bool {
	return len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator)
}
