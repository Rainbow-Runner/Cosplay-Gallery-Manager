package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

type DerivativeStore struct{ db *sql.DB }

func (db *Database) Derivatives() *DerivativeStore { return &DerivativeStore{db: db.DB} }

type PublishDerivativeInput struct {
	ItemUUID          string
	Variant           string
	CacheTier         mediaprocessing.CacheTier
	ContentRevision   int64
	ProfileHash       string
	CacheRelativePath string
	MIMEType          string
	ByteSize          int64
	Width             int
	Height            int
}

func (s *DerivativeStore) Publish(ctx context.Context, input PublishDerivativeInput, now time.Time) (mediaprocessing.Derivative, error) {
	if input.ItemUUID == "" || input.Variant == "" || len(input.Variant) > 100 || input.ContentRevision <= 0 ||
		input.ProfileHash == "" || len(input.ProfileHash) > 200 || input.MIMEType == "" || len(input.MIMEType) > 100 ||
		input.ByteSize < 0 || input.Width < 0 || input.Height < 0 ||
		(input.CacheTier != mediaprocessing.CacheBase && input.CacheTier != mediaprocessing.CacheEnhanced) {
		return mediaprocessing.Derivative{}, errors.New("invalid derivative metadata")
	}
	requiredTier, knownVariant := mediaprocessing.RequiredCacheTier(input.Variant)
	if !knownVariant || input.CacheTier != requiredTier {
		return mediaprocessing.Derivative{}, errors.New("derivative cache tier does not match the variant retention policy")
	}
	if err := manifest.ValidateManagedRelativeAsset(input.CacheRelativePath); err != nil {
		return mediaprocessing.Derivative{}, fmt.Errorf("invalid derivative cache path: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return mediaprocessing.Derivative{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var actualRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT content_revision FROM gallery_items WHERE item_uuid=?`, input.ItemUUID).Scan(&actualRevision); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	if actualRevision != input.ContentRevision {
		return mediaprocessing.Derivative{}, errors.New("derivative content revision is stale")
	}
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `UPDATE media_derivatives SET is_current=0,state=CASE WHEN state='HARD_INVALID' THEN state ELSE 'STALE' END
		WHERE item_uuid=? AND variant=? AND is_current=1`, input.ItemUUID, input.Variant); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO media_derivatives (item_uuid,variant,cache_tier,content_revision,
		profile_hash,state,is_current,cache_relative_path,mime_type,byte_size,width,height,created_at_utc,last_accessed_at_utc)
		VALUES (?,?,?,?,?,'READY',1,?,?,?,?,?,?,?)
		ON CONFLICT(item_uuid,variant,content_revision,profile_hash) DO UPDATE SET cache_tier=excluded.cache_tier,
		state='READY',is_current=1,cache_relative_path=excluded.cache_relative_path,mime_type=excluded.mime_type,
		byte_size=excluded.byte_size,width=excluded.width,height=excluded.height,last_accessed_at_utc=excluded.last_accessed_at_utc`,
		input.ItemUUID, input.Variant, input.CacheTier, input.ContentRevision, input.ProfileHash,
		input.CacheRelativePath, input.MIMEType, input.ByteSize, input.Width, input.Height, timestamp, timestamp); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	result, err := findDerivative(ctx, tx, input.ItemUUID, input.Variant, true)
	if err != nil {
		return mediaprocessing.Derivative{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gallery_items SET processing_state='READY',updated_at_utc=? WHERE item_uuid=?`, timestamp, input.ItemUUID); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE galleries SET scrubber_revision=scrubber_revision+1
		WHERE id=(SELECT gallery_id FROM gallery_items WHERE item_uuid=?)`, input.ItemUUID); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	if err := tx.Commit(); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	return result, nil
}

func (s *DerivativeStore) Current(ctx context.Context, itemUUID, variant string, now time.Time) (mediaprocessing.Derivative, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return mediaprocessing.Derivative{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := findDerivative(ctx, tx, itemUUID, variant, true)
	if err != nil {
		return mediaprocessing.Derivative{}, err
	}
	if result.State == mediaprocessing.DerivativeHardInvalid {
		return mediaprocessing.Derivative{}, errors.New("derivative is hard-invalid")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_derivatives SET last_accessed_at_utc=? WHERE id=?`, formatTime(normalisedTime(now)), result.ID); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	if err := tx.Commit(); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	result.LastAccessedUTC = normalisedTime(now)
	return result, nil
}

// MarkProfileStale keeps the old current resource serviceable until Publish
// atomically promotes the replacement.
func (s *DerivativeStore) MarkProfileStale(ctx context.Context, itemUUID, variant, currentProfileHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE media_derivatives SET state='STALE'
		WHERE item_uuid=? AND variant=? AND is_current=1 AND profile_hash<>? AND state='READY'`, itemUUID, variant, currentProfileHash)
	return err
}

// InvalidateContent prevents any resource from an older byte identity from
// being served after scanner content_revision changes.
func (s *DerivativeStore) InvalidateContent(ctx context.Context, itemUUID string, currentContentRevision int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE media_derivatives SET state='HARD_INVALID',is_current=0
		WHERE item_uuid=? AND content_revision<>? AND (state<>'HARD_INVALID' OR is_current<>0)`, itemUUID, currentContentRevision)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE galleries SET scrubber_revision=scrubber_revision+1
			WHERE id=(SELECT gallery_id FROM gallery_items WHERE item_uuid=?)`, itemUUID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *DerivativeStore) EnhancedLRUCandidates(ctx context.Context, bytesToFree int64, limit int) ([]mediaprocessing.Derivative, error) {
	if bytesToFree <= 0 || limit <= 0 || limit > 10000 {
		return nil, errors.New("invalid cache cleanup target")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,item_uuid,variant,cache_tier,content_revision,profile_hash,state,is_current,
		cache_relative_path,mime_type,byte_size,width,height,created_at_utc,last_accessed_at_utc
		FROM media_derivatives WHERE cache_tier='ENHANCED' ORDER BY
		CASE WHEN state='HARD_INVALID' THEN 0 WHEN is_current=0 THEN 1 ELSE 2 END,last_accessed_at_utc,id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []mediaprocessing.Derivative
	var total int64
	for rows.Next() && total < bytesToFree {
		entry, err := scanDerivative(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
		total += entry.ByteSize
	}
	return result, rows.Err()
}

// ForgetGenerated removes only an ENHANCED derivative database record. The
// caller may then remove the now-unreferenced application-generated file;
// BASE resources are never accepted by this maintenance operation.
func (s *DerivativeStore) ForgetGenerated(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM media_derivatives WHERE id=? AND cache_tier='ENHANCED'`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("derivative is missing or belongs to the permanent BASE tier")
	}
	return nil
}

type ScrubberEntry struct {
	Ordinal         int
	ItemUUID        string
	ContentRevision int64
	Variant         string
	ProfileHash     string
	MIMEType        string
	MediaKind       gallery.MediaKind
	ImageCategory   gallery.ImageCategory
}

func (s *DerivativeStore) GalleryScrubberIndex(ctx context.Context, galleryID int64) ([]ScrubberEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT item.item_uuid,item.content_revision,derivative.variant,
		derivative.profile_hash,derivative.mime_type,item.media_kind,item.image_category,item.position
		FROM gallery_items item JOIN media_derivatives derivative ON derivative.item_uuid=item.item_uuid
		WHERE item.gallery_id=? AND item.excluded=0 AND item.availability_state='AVAILABLE'
		AND derivative.is_current=1 AND derivative.state IN ('READY','STALE')
		AND ((item.media_kind='STATIC_IMAGE' AND derivative.variant='CARD_480') OR
			(item.media_kind IN ('ANIMATED_IMAGE','VIDEO') AND derivative.variant='STATIC_POSTER'))`, galleryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type sortable struct {
		entry    ScrubberEntry
		position int64
	}
	var values []sortable
	for rows.Next() {
		var value sortable
		var category sql.NullString
		if err := rows.Scan(&value.entry.ItemUUID, &value.entry.ContentRevision, &value.entry.Variant,
			&value.entry.ProfileHash, &value.entry.MIMEType, &value.entry.MediaKind, &category, &value.position); err != nil {
			return nil, err
		}
		value.entry.ImageCategory = gallery.ImageCategory(category.String)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(values, func(i, j int) bool {
		left, right := scrubberGroup(values[i].entry), scrubberGroup(values[j].entry)
		if left != right {
			return left < right
		}
		if values[i].position != values[j].position {
			return values[i].position < values[j].position
		}
		return values[i].entry.ItemUUID < values[j].entry.ItemUUID
	})
	result := make([]ScrubberEntry, len(values))
	for index := range values {
		result[index] = values[index].entry
		result[index].Ordinal = index
	}
	return result, nil
}

func scrubberGroup(entry ScrubberEntry) int {
	if entry.MediaKind == gallery.MediaKindStaticImage {
		if entry.ImageCategory == gallery.ImageCategorySelfie {
			return 1
		}
		return 0
	}
	if entry.MediaKind == gallery.MediaKindAnimatedImage {
		return 2
	}
	return 3
}

func findDerivative(ctx context.Context, queryer galleryQueryer, itemUUID, variant string, current bool) (mediaprocessing.Derivative, error) {
	return scanDerivative(queryer.QueryRowContext(ctx, `SELECT id,item_uuid,variant,cache_tier,content_revision,profile_hash,state,is_current,
		cache_relative_path,mime_type,byte_size,width,height,created_at_utc,last_accessed_at_utc
		FROM media_derivatives WHERE item_uuid=? AND variant=? AND is_current=?`, itemUUID, variant, current))
}

func scanDerivative(row rowScanner) (mediaprocessing.Derivative, error) {
	var result mediaprocessing.Derivative
	var current int
	var created, accessed string
	if err := row.Scan(&result.ID, &result.ItemUUID, &result.Variant, &result.CacheTier, &result.ContentRevision,
		&result.ProfileHash, &result.State, &current, &result.CacheRelativePath, &result.MIMEType, &result.ByteSize,
		&result.Width, &result.Height, &created, &accessed); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	result.Current = current == 1
	var err error
	if result.CreatedAtUTC, err = parseTime(created); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	if result.LastAccessedUTC, err = parseTime(accessed); err != nil {
		return mediaprocessing.Derivative{}, err
	}
	return result, nil
}
