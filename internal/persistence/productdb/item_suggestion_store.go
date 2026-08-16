package productdb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"golang.org/x/text/unicode/norm"
)

type ItemSuggestionKind string

const (
	ItemSuggestionSelfie       ItemSuggestionKind = "SELFIE_CATEGORY"
	ItemSuggestionCaptureDate  ItemSuggestionKind = "CAPTURE_DATE"
	ItemSuggestionRAWCompanion ItemSuggestionKind = "RAW_COMPANION"
)

type ItemSuggestion struct {
	ID            int64
	GalleryID     int64
	ItemUUID      string
	Kind          ItemSuggestionKind
	Value         string
	SourceKind    string
	Status        string
	CreatedAtUTC  time.Time
	ResolvedAtUTC *time.Time
}

type ItemSuggestionStore struct{ db *sql.DB }

func (db *Database) ItemSuggestions() *ItemSuggestionStore { return &ItemSuggestionStore{db: db.DB} }

func (s *ItemSuggestionStore) SuggestCaptureDate(ctx context.Context, itemUUID, value, sourceKind string, now time.Time) (ItemSuggestion, error) {
	if sourceKind != "EXIF" && sourceKind != "XMP" {
		return ItemSuggestion{}, errors.New("capture date suggestion source must be EXIF or XMP")
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return ItemSuggestion{}, errors.New("capture date suggestion must be YYYY-MM-DD")
	}
	var galleryID int64
	if err := s.db.QueryRowContext(ctx, `SELECT gallery_id FROM gallery_items WHERE item_uuid=?`, itemUUID).Scan(&galleryID); err != nil {
		return ItemSuggestion{}, err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO gallery_item_suggestions (gallery_id,item_uuid,suggestion_kind,value,source_kind,status,created_at_utc)
		VALUES (?,?,'CAPTURE_DATE',?,?,'PENDING',?) ON CONFLICT(item_uuid,suggestion_kind,value) DO NOTHING`, galleryID, itemUUID, value, sourceKind, formatTime(normalisedTime(now))); err != nil {
		return ItemSuggestion{}, err
	}
	return findItemSuggestionByKey(ctx, s.db, itemUUID, ItemSuggestionCaptureDate, value)
}

func (s *ItemSuggestionStore) Resolve(ctx context.Context, id int64, accept bool, expectedGalleryRevision int64, now time.Time) (ItemSuggestion, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ItemSuggestion{}, err
	}
	defer func() { _ = tx.Rollback() }()
	suggestion, err := findItemSuggestion(ctx, tx, id)
	if err != nil {
		return ItemSuggestion{}, err
	}
	if suggestion.Status != "PENDING" {
		return ItemSuggestion{}, errors.New("Item suggestion was already resolved")
	}
	status := "REJECTED"
	if accept {
		status = "ACCEPTED"
	}
	timestamp := formatTime(normalisedTime(now))
	if accept {
		switch suggestion.Kind {
		case ItemSuggestionSelfie:
			if _, err := tx.ExecContext(ctx, `UPDATE gallery_items SET image_category='SELFIE',updated_at_utc=?
				WHERE item_uuid=? AND media_kind='STATIC_IMAGE'`, timestamp, suggestion.ItemUUID); err != nil {
				return ItemSuggestion{}, err
			}
			if err := touchGalleryCoverMetadata(ctx, tx, suggestion.GalleryID, &expectedGalleryRevision, now); err != nil {
				return ItemSuggestion{}, err
			}
		case ItemSuggestionCaptureDate:
			result, err := tx.ExecContext(ctx, `UPDATE galleries SET shoot_date=?,shoot_date_precision='DAY',metadata_revision=metadata_revision+1,
				updated_at_utc=? WHERE id=? AND metadata_revision=?`, suggestion.Value, timestamp, suggestion.GalleryID, expectedGalleryRevision)
			if err != nil {
				return ItemSuggestion{}, err
			}
			if err := requireOneRevisionRow(result); err != nil {
				return ItemSuggestion{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE gallery_manifest_sync SET status=CASE WHEN status='CLEAN' THEN 'DB_DIRTY' ELSE status END WHERE gallery_id=?`, suggestion.GalleryID); err != nil {
				return ItemSuggestion{}, err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gallery_item_suggestions SET status=?,resolved_at_utc=? WHERE id=?`, status, timestamp, id); err != nil {
		return ItemSuggestion{}, err
	}
	if err := tx.Commit(); err != nil {
		return ItemSuggestion{}, err
	}
	return s.Find(ctx, id)
}

func (s *ItemSuggestionStore) Find(ctx context.Context, id int64) (ItemSuggestion, error) {
	return findItemSuggestion(ctx, s.db, id)
}

func syncScanItemSuggestions(ctx context.Context, tx *sql.Tx, galleryID, sourceID int64, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT item_uuid,relative_path,content_format FROM gallery_items WHERE source_id=? AND availability_state='AVAILABLE'`, sourceID)
	if err != nil {
		return err
	}
	type item struct {
		uuid, path string
		format     gallery.ContentFormat
	}
	var items []item
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.uuid, &value.path, &value.format); err != nil {
			rows.Close()
			return err
		}
		items = append(items, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	timestamp := formatTime(normalisedTime(now))
	byBase := map[string][]item{}
	for _, value := range items {
		base := strings.TrimSuffix(norm.NFC.String(value.path), filepath.Ext(value.path))
		byBase[strings.ToLower(base)] = append(byBase[strings.ToLower(base)], value)
	}
	for _, group := range byBase {
		for _, left := range group {
			for _, right := range group {
				if left.uuid == right.uuid || left.format == right.format || (left.format != gallery.ContentFormatRAW && right.format != gallery.ContentFormatRAW) {
					continue
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO gallery_item_suggestions (gallery_id,item_uuid,suggestion_kind,value,source_kind,status,created_at_utc)
					VALUES (?,?,'RAW_COMPANION',?,'RAW_COMPANION','PENDING',?) ON CONFLICT(item_uuid,suggestion_kind,value) DO NOTHING`, galleryID, left.uuid, right.uuid, timestamp); err != nil {
					return err
				}
			}
		}
	}
	return evaluateMediaClassificationForSource(ctx, tx, sourceID, now)
}

func findItemSuggestion(ctx context.Context, queryer galleryQueryer, id int64) (ItemSuggestion, error) {
	return scanItemSuggestion(queryer.QueryRowContext(ctx, `SELECT id,gallery_id,item_uuid,suggestion_kind,value,source_kind,status,created_at_utc,resolved_at_utc FROM gallery_item_suggestions WHERE id=?`, id))
}
func findItemSuggestionByKey(ctx context.Context, queryer galleryQueryer, itemUUID string, kind ItemSuggestionKind, value string) (ItemSuggestion, error) {
	return scanItemSuggestion(queryer.QueryRowContext(ctx, `SELECT id,gallery_id,item_uuid,suggestion_kind,value,source_kind,status,created_at_utc,resolved_at_utc FROM gallery_item_suggestions WHERE item_uuid=? AND suggestion_kind=? AND value=?`, itemUUID, kind, value))
}
func scanItemSuggestion(row rowScanner) (ItemSuggestion, error) {
	var result ItemSuggestion
	var created string
	var resolved sql.NullString
	if err := row.Scan(&result.ID, &result.GalleryID, &result.ItemUUID, &result.Kind, &result.Value, &result.SourceKind, &result.Status, &created, &resolved); err != nil {
		return ItemSuggestion{}, err
	}
	var err error
	result.CreatedAtUTC, err = parseTime(created)
	if err != nil {
		return ItemSuggestion{}, err
	}
	if resolved.Valid {
		value, err := parseTime(resolved.String)
		if err != nil {
			return ItemSuggestion{}, err
		}
		result.ResolvedAtUTC = &value
	}
	return result, nil
}
