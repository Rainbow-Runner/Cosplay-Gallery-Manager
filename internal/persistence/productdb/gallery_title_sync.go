package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/stashapp/stash/internal/archivefile"
	"github.com/stashapp/stash/internal/gallery"
)

var errSourceTitleUnavailable = errors.New("source name is unavailable or invalid")

func deriveSourceTitle(ctx context.Context, tx *sql.Tx, id int64, rule string) (string, string, error) {
	var path string
	var kind gallery.SourceType
	var availability gallery.AvailabilityState
	if err := tx.QueryRowContext(ctx, `SELECT source_path,source_type,availability_state FROM gallery_sources WHERE gallery_id=?`, id).Scan(&path, &kind, &availability); err != nil {
		return "", rule, err
	}
	if availability != gallery.AvailabilityAvailable {
		return "", rule, errSourceTitleUnavailable
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", rule, fmt.Errorf("%w: %v", errSourceTitleUnavailable, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", rule, fmt.Errorf("%w: source title cannot follow a symlink", errSourceTitleUnavailable)
	}
	if kind == gallery.SourceTypeArchive {
		if !info.Mode().IsRegular() {
			return "", rule, fmt.Errorf("%w: archive source is not a regular file", errSourceTitleUnavailable)
		}
		title, err := validateMarkerTitle(archivefile.BaseName(path))
		if err != nil {
			return "", rule, fmt.Errorf("%w: %v", errSourceTitleUnavailable, err)
		}
		return title, "ARCHIVE", err
	}
	if !info.IsDir() {
		return "", rule, fmt.Errorf("%w: directory source is not a directory", errSourceTitleUnavailable)
	}
	if rule == "MARKER" {
		title, err := markerGalleryTitle(path)
		if err != nil {
			return "", rule, fmt.Errorf("%w: %v", errSourceTitleUnavailable, err)
		}
		return title, rule, err
	}
	title, err := validateMarkerTitle(filepath.Base(filepath.Clean(path)))
	if err != nil {
		return "", rule, fmt.Errorf("%w: %v", errSourceTitleUnavailable, err)
	}
	return title, "ROOT", err
}

// Synchronize only a known, unchanged automatic baseline. Unknown historical
// ownership and any user/Manifest title edit are never inferred or overwritten.
func syncGallerySourceTitle(ctx context.Context, tx *sql.Tx, id int64, now time.Time) error {
	var title, origin, baseline, rule string
	if err := tx.QueryRowContext(ctx, `SELECT title,title_origin,source_title_baseline,title_source_rule FROM galleries WHERE id=?`, id).Scan(&title, &origin, &baseline, &rule); err != nil {
		return err
	}
	if origin != "AUTO" {
		return nil
	}
	if title != baseline || baseline == "" {
		if _, err := tx.ExecContext(ctx, `UPDATE galleries SET title_origin='MANUAL',metadata_revision=metadata_revision+1,updated_at_utc=? WHERE id=?`, formatTime(normalisedTime(now)), id); err != nil {
			return err
		}
		return markGalleryManifestDBDirty(ctx, tx, id)
	}
	next, nextRule, err := deriveSourceTitle(ctx, tx, id, rule)
	if err != nil {
		// Missing, unreadable or invalid source names cannot authorize a title edit.
		if errors.Is(err, errSourceTitleUnavailable) {
			return nil
		}
		return err
	}
	if next == title {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE galleries SET title=?,source_title_baseline=?,title_source_rule=?,metadata_revision=metadata_revision+1,updated_at_utc=? WHERE id=?`, next, next, nextRule, formatTime(normalisedTime(now)), id); err != nil {
		return err
	}
	return markGalleryManifestDBDirty(ctx, tx, id)
}

func prepareGalleryTitleEdit(ctx context.Context, tx *sql.Tx, id int64, input *UpdateGalleryMetadataInput) error {
	var current, origin, rule string
	if err := tx.QueryRowContext(ctx, `SELECT title,title_origin,title_source_rule FROM galleries WHERE id=?`, id).Scan(&current, &origin, &rule); err != nil {
		return err
	}
	if input.FollowSourceTitle != nil && *input.FollowSourceTitle && origin != "AUTO" {
		// Explicit opt-in is required for legacy/manual titles. Preserve the
		// marker rule when known; never guess the provenance of an old Gallery.
		title, nextRule, err := deriveSourceTitle(ctx, tx, id, rule)
		if err != nil {
			return err
		}
		input.Title = title
		_, err = tx.ExecContext(ctx, `UPDATE galleries SET title_origin='AUTO',source_title_baseline=?,title_source_rule=? WHERE id=?`, title, nextRule, id)
		return err
	}
	if input.Title != current || input.FollowSourceTitle != nil && !*input.FollowSourceTitle {
		_, err := tx.ExecContext(ctx, `UPDATE galleries SET title_origin='MANUAL' WHERE id=?`, id)
		return err
	}
	return nil
}
