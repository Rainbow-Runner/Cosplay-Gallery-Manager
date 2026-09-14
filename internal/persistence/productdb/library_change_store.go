package productdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/stashapp/stash/internal/library"
)

func canonicalProposedRoot(root string) (string, error) {
	if root == "" {
		return "", nil
	}
	return canonicalLibraryRoot(root)
}

// The preview and application use the same transaction-local inventory. Its
// token covers every object that would be remapped, deleted or change ownership.
func loadLibraryChangePreview(ctx context.Context, tx *sql.Tx, libraryID int64, newRoot string) (library.ChangePreview, error) {
	current, err := findLibrary(ctx, tx, libraryID)
	if err != nil {
		return library.ChangePreview{}, err
	}
	p := library.ChangePreview{LibraryID: libraryID, CurrentRoot: current.RootPath, NewRoot: newRoot}
	rows, err := tx.QueryContext(ctx, `SELECT s.id,s.gallery_id,g.title,s.source_path FROM gallery_sources s JOIN galleries g ON g.id=s.gallery_id WHERE s.library_id=? ORDER BY s.id`, libraryID)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		v := library.SourceImpact{CurrentLibrary: libraryID}
		if err := rows.Scan(&v.SourceID, &v.GalleryID, &v.GalleryTitle, &v.SourcePath); err != nil {
			rows.Close()
			return p, err
		}
		p.Impacts = append(p.Impacts, v)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return p, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT id,source_path,reason FROM ignored_gallery_sources WHERE library_id=? ORDER BY id`, libraryID)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var v library.IgnoredSourceImpact
		if err := rows.Scan(&v.ID, &v.Path, &v.Reason); err != nil {
			rows.Close()
			return p, err
		}
		p.IgnoredSources = append(p.IgnoredSources, v)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return p, err
	}
	p.IgnoredSourceCount = len(p.IgnoredSources)
	rows, err = tx.QueryContext(ctx, `SELECT source_path FROM gallery_sources WHERE library_id IS NULL ORDER BY source_path`)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var sourcePath string
		if err := rows.Scan(&sourcePath); err != nil {
			rows.Close()
			return p, err
		}
		if pathWithin(current.RootPath, sourcePath) {
			p.UnassignedSourcePaths = append(p.UnassignedSourcePaths, sourcePath)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return p, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_sources WHERE library_id=? AND reconcile_state='SCANNING'`, libraryID).Scan(&p.ScanningSourceCount); err != nil {
		return p, err
	}
	for _, spec := range []struct {
		query string
		dest  *[]library.RuleImpact
	}{
		{`SELECT id,name,updated_at_utc FROM gallery_recognition_rules WHERE library_id=? ORDER BY id`, &p.RecognitionRules},
		{`SELECT id,name,revision FROM media_classification_rules WHERE library_id=? ORDER BY id`, &p.ClassificationRules},
		{`SELECT id,name,revision FROM media_exclusion_rules WHERE library_id=? ORDER BY id`, &p.ExclusionRules},
	} {
		rows, err = tx.QueryContext(ctx, spec.query, libraryID)
		if err != nil {
			return p, err
		}
		for rows.Next() {
			var v library.RuleImpact
			var revision any
			if err := rows.Scan(&v.ID, &v.Name, &revision); err != nil {
				rows.Close()
				return p, err
			}
			v.Revision = fmt.Sprint(revision)
			*spec.dest = append(*spec.dest, v)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return p, err
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT mode,revision FROM library_automation_policies WHERE library_id=?`, libraryID).Scan(&p.AutomationMode, &p.AutomationPolicyRevision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return p, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		p.AutomationMode = "MANUAL"
	}
	rows, err = tx.QueryContext(ctx, `SELECT id,status FROM library_automation_runs WHERE library_id=? ORDER BY id`, libraryID)
	if err != nil {
		return p, err
	}
	var runs []string
	for rows.Next() {
		var id int64
		var status string
		if err := rows.Scan(&id, &status); err != nil {
			rows.Close()
			return p, err
		}
		p.AutomationRunCount++
		if status == "QUEUED" || status == "RUNNING" {
			p.ActiveRunCount++
		}
		runs = append(runs, strconv.FormatInt(id, 10)+":"+status)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return p, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_library_mappings WHERE target_library_id=?`, libraryID).Scan(&p.PortableMappingCount); err != nil {
		return p, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT id,root_path FROM media_libraries WHERE id<>? ORDER BY id`, libraryID)
	if err != nil {
		return p, err
	}
	var others []library.Library
	for rows.Next() {
		var v library.Library
		if err := rows.Scan(&v.ID, &v.RootPath); err != nil {
			rows.Close()
			return p, err
		}
		if pathWithin(current.RootPath, v.RootPath) {
			p.ChildRoots = append(p.ChildRoots, v.RootPath)
		}
		if newRoot != "" && pathWithin(newRoot, v.RootPath) {
			p.ProposedBoundaryConflicts = append(p.ProposedBoundaryConflicts, v.RootPath)
		}
		others = append(others, v)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return p, err
	}
	if newRoot != "" {
		others = append(others, library.Library{ID: libraryID, RootPath: newRoot})
	}
	sort.Slice(others, func(i, j int) bool { return len(others[i].RootPath) > len(others[j].RootPath) })
	for i := range p.Impacts {
		if owner := ownerFromSortedLibraries(others, p.Impacts[i].SourcePath); owner != nil {
			id := owner.ID
			p.Impacts[i].SuggestedOwner = &id
		}
	}
	// A status transition on an old run is material even when counts are stable.
	fingerprint, err := json.Marshal(struct {
		Current library.Library
		Preview library.ChangePreview
		Runs    []string
	}{current, p, runs})
	if err != nil {
		return p, err
	}
	digest := sha256.Sum256(fingerprint)
	p.RevisionToken = hex.EncodeToString(digest[:])
	return p, nil
}

// ApplyChange never moves or deletes user media. Root relocation remaps CGM
// paths and marks sources for revalidation; deletion requires every Gallery
// Source to have been explicitly transferred first.
func (s *LibraryStore) ApplyChange(ctx context.Context, libraryID int64, newRoot, expectedToken string, now time.Time) error {
	root, err := canonicalProposedRoot(newRoot)
	if err != nil {
		return err
	}
	if len(expectedToken) != 64 {
		return errors.New("library change preview token is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, err := loadLibraryChangePreview(ctx, tx, libraryID, root)
	if err != nil {
		return err
	}
	if p.RevisionToken != expectedToken {
		return errors.New("library change preview is stale; refresh before applying")
	}
	if p.ActiveRunCount != 0 {
		return errors.New("cancel active library automation before changing media library")
	}
	if p.ScanningSourceCount != 0 {
		return errors.New("wait for active GallerySource scans before changing media library")
	}
	if p.PortableMappingCount != 0 {
		return errors.New("media library is referenced by a portable migration mapping")
	}
	if root == "" {
		if len(p.Impacts) != 0 {
			return errors.New("transfer every GallerySource before deleting media library")
		}
		// Preserve exact-path ignores as global tombstones. Otherwise removing a
		// child boundary could make a parent scan silently rediscover them.
		if _, err := tx.ExecContext(ctx, `UPDATE ignored_gallery_sources SET library_id=NULL WHERE library_id=?`, libraryID); err != nil {
			return err
		}
		for _, sourcePath := range p.UnassignedSourcePaths {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO ignored_gallery_sources(library_id,source_path,reason,created_at_utc) VALUES(NULL,?,'LIBRARY_DELETED_UNASSIGNED_SOURCE',?)`, sourcePath, formatTime(normalisedTime(now))); err != nil {
				return err
			}
			var ignoredOwner sql.NullInt64
			if err := tx.QueryRowContext(ctx, `SELECT library_id FROM ignored_gallery_sources WHERE source_path=?`, sourcePath).Scan(&ignoredOwner); err != nil {
				return err
			}
			if ignoredOwner.Valid {
				return errors.New("unassigned Source ignore belongs to another media library")
			}
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM media_libraries WHERE id=? AND root_path=?`, libraryID, p.CurrentRoot)
		if err != nil {
			return err
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			return errors.New("media library changed before deletion")
		}
	} else {
		if root == p.CurrentRoot {
			return errors.New("new media library root must differ from current root")
		}
		if len(p.ChildRoots) != 0 {
			return errors.New("move child media libraries separately before changing parent root")
		}
		if len(p.ProposedBoundaryConflicts) != 0 {
			return errors.New("proposed root contains another media library")
		}
		// Another library beneath the proposed root would change its ownership boundary.
		rows, err := tx.QueryContext(ctx, `SELECT root_path FROM media_libraries WHERE id<>?`, libraryID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var other string
			if err := rows.Scan(&other); err != nil {
				rows.Close()
				return err
			}
			if pathWithin(root, other) {
				rows.Close()
				return errors.New("proposed root contains another media library")
			}
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return err
		}
		timestamp := formatTime(normalisedTime(now))
		if err := remapLibraryPaths(ctx, tx, libraryID, p.CurrentRoot, root, timestamp); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE media_libraries SET root_path=?,updated_at_utc=? WHERE id=? AND root_path=?`, root, timestamp, libraryID, p.CurrentRoot)
		if err != nil {
			return err
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			return errors.New("media library changed before root update")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_sources SET availability_state='MISSING',reconcile_state='NEEDS_RESCAN',updated_at_utc=? WHERE library_id=?`, timestamp, libraryID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_manifest_sync SET last_error_code='LIBRARY_ROOT_CHANGED' WHERE gallery_id IN (SELECT gallery_id FROM gallery_sources WHERE library_id=?)`, libraryID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM discovery_snapshots WHERE library_id=?`, libraryID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
