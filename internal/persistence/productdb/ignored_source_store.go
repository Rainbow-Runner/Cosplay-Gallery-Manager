package productdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type IgnoredSourceRecord struct {
	ID        int64
	LibraryID *int64
	SetID     *string
	Path      string
	Reason    string
	CreatedAt string
}

type IgnoredSourcePage struct {
	Items    []IgnoredSourceRecord
	Page     int
	PageSize int
	Total    int
}

type IgnoredSourceRemovalPreview struct {
	Record             IgnoredSourceRecord
	AffectedLibraryIDs []int64
	ActiveRunCount     int
	BoundSourceCount   int
	RevisionToken      string
}

var ErrIgnoredSourcePreviewStale = errors.New("ignored source removal preview is stale")
var ErrIgnoredSourceAutomationActive = errors.New("affected library automation is active")

func (s *LibraryStore) ListIgnoredSources(ctx context.Context, libraryID *int64, page int, query string) (IgnoredSourcePage, error) {
	if page < 1 || page > 100000 {
		return IgnoredSourcePage{}, errors.New("ignored source page is out of range")
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 300 {
		return IgnoredSourcePage{}, errors.New("ignored source search is too long")
	}
	result := IgnoredSourcePage{Page: page, PageSize: 50}
	filter := `library_id IS NULL`
	args := []any{}
	if libraryID != nil {
		filter = `library_id=?`
		args = append(args, *libraryID)
	}
	filter += ` AND instr(lower(source_path),lower(?))>0`
	args = append(args, query)
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ignored_gallery_sources WHERE `+filter, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,library_id,set_id,source_path,reason,created_at_utc FROM ignored_gallery_sources WHERE `+filter+` ORDER BY id DESC LIMIT 50 OFFSET ?`, append(args, (page-1)*50)...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item IgnoredSourceRecord
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.SetID, &item.Path, &item.Reason, &item.CreatedAt); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func (s *LibraryStore) PreviewIgnoredSourceRemoval(ctx context.Context, id int64) (IgnoredSourceRemovalPreview, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IgnoredSourceRemovalPreview{}, err
	}
	defer tx.Rollback()
	p, err := loadIgnoredSourceRemovalPreview(ctx, tx, id)
	if err != nil {
		return p, err
	}
	return p, tx.Commit()
}

func loadIgnoredSourceRemovalPreview(ctx context.Context, tx *sql.Tx, id int64) (IgnoredSourceRemovalPreview, error) {
	p := IgnoredSourceRemovalPreview{}
	err := tx.QueryRowContext(ctx, `SELECT id,library_id,set_id,source_path,reason,created_at_utc FROM ignored_gallery_sources WHERE id=?`, id).Scan(&p.Record.ID, &p.Record.LibraryID, &p.Record.SetID, &p.Record.Path, &p.Record.Reason, &p.Record.CreatedAt)
	if err != nil {
		return p, err
	}
	if p.Record.SetID != nil {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM media_libraries ORDER BY id`)
		if err != nil {
			return p, err
		}
		for rows.Next() {
			var libID int64
			if err := rows.Scan(&libID); err != nil {
				rows.Close()
				return p, err
			}
			p.AffectedLibraryIDs = append(p.AffectedLibraryIDs, libID)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return p, err
		}
	} else if p.Record.LibraryID != nil {
		p.AffectedLibraryIDs = append(p.AffectedLibraryIDs, *p.Record.LibraryID)
	} else {
		rows, err := tx.QueryContext(ctx, `SELECT id,root_path FROM media_libraries ORDER BY id`)
		if err != nil {
			return p, err
		}
		for rows.Next() {
			var libID int64
			var root string
			if err := rows.Scan(&libID, &root); err != nil {
				rows.Close()
				return p, err
			}
			if pathWithin(root, p.Record.Path) {
				p.AffectedLibraryIDs = append(p.AffectedLibraryIDs, libID)
			}
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return p, err
		}
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_sources WHERE source_path=?`, p.Record.Path).Scan(&p.BoundSourceCount); err != nil {
		return p, err
	}
	for _, libID := range p.AffectedLibraryIDs {
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_automation_runs WHERE library_id=? AND status IN ('QUEUED','RUNNING')`, libID).Scan(&active); err != nil {
			return p, err
		}
		p.ActiveRunCount += active
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	digest := sha256.Sum256(encoded)
	p.RevisionToken = hex.EncodeToString(digest[:])
	return p, nil
}

// RevokeIgnoredSource only removes the CGM ignore row. It never scans, imports,
// deletes, moves or writes user media; the next discovery may see the path.
func (s *LibraryStore) RevokeIgnoredSource(ctx context.Context, id int64, expectedToken string) error {
	if len(expectedToken) != 64 {
		return errors.New("ignored source removal preview token is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, err := loadIgnoredSourceRemovalPreview(ctx, tx, id)
	if err != nil {
		return err
	}
	if p.RevisionToken != expectedToken {
		return ErrIgnoredSourcePreviewStale
	}
	if p.ActiveRunCount != 0 {
		return ErrIgnoredSourceAutomationActive
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM ignored_gallery_sources WHERE id=? AND source_path=?`, id, p.Record.Path)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return fmt.Errorf("ignored source changed before removal")
	}
	for _, libID := range p.AffectedLibraryIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM discovery_snapshots WHERE library_id=?`, libID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
