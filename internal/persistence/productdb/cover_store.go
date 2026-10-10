package productdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

type CoverStore struct {
	db     *sql.DB
	random io.Reader
}

func (db *Database) Covers() *CoverStore { return &CoverStore{db: db.DB, random: rand.Reader} }

func (s *CoverStore) Initialize(ctx context.Context, galleryID int64, now time.Time) (gallery.CoverState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.CoverState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	state, found, err := findCover(ctx, tx, galleryID)
	if err != nil {
		return gallery.CoverState{}, err
	}
	if found && state.PreferredKind != gallery.CoverNone {
		if err := recomputeEffectiveCover(ctx, tx, &state, false, s.random, now); err != nil {
			return gallery.CoverState{}, err
		}
		if err := persistCover(ctx, tx, state); err != nil {
			return gallery.CoverState{}, err
		}
		if err := tx.Commit(); err != nil {
			return gallery.CoverState{}, err
		}
		return state, nil
	}
	selected, err := chooseStaticMember(ctx, tx, galleryID, "", s.random)
	if err != nil {
		return gallery.CoverState{}, err
	}
	state = gallery.CoverState{GalleryID: galleryID, PreferredKind: gallery.CoverNone, EffectiveKind: gallery.CoverNone, Revision: 1, UpdatedAtUTC: normalisedTime(now)}
	if selected != "" {
		state.PreferredKind, state.PreferredItemUUID = gallery.CoverAutoRandom, selected
	}
	if err := recomputeEffectiveCover(ctx, tx, &state, false, s.random, now); err != nil {
		return gallery.CoverState{}, err
	}
	if err := persistCover(ctx, tx, state); err != nil {
		return gallery.CoverState{}, err
	}
	if selected != "" {
		if err := touchGalleryCoverMetadata(ctx, tx, galleryID, nil, now); err != nil {
			return gallery.CoverState{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return gallery.CoverState{}, err
	}
	return state, nil
}

func (s *CoverStore) SetItem(ctx context.Context, galleryID int64, itemUUID string, expectedMetadataRevision int64, now time.Time) (gallery.CoverState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.CoverState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var kind gallery.MediaKind
	if err := tx.QueryRowContext(ctx, `SELECT media_kind FROM gallery_items WHERE gallery_id=? AND item_uuid=?`, galleryID, itemUUID).Scan(&kind); err != nil {
		return gallery.CoverState{}, err
	}
	if kind != gallery.MediaKindStaticImage {
		return gallery.CoverState{}, errors.New("only a static GalleryItem can become the preferred Item cover")
	}
	state, _, err := findCover(ctx, tx, galleryID)
	if err != nil {
		return gallery.CoverState{}, err
	}
	state = normalizedCoverState(state, galleryID, now)
	previous, err := json.Marshal(state)
	if err != nil {
		return gallery.CoverState{}, err
	}
	state.GalleryID, state.PreferredKind, state.PreferredItemUUID, state.PreferredPath = galleryID, gallery.CoverItem, itemUUID, ""
	state.Revision++
	if state.Revision == 1 {
		state.Revision = 1
	}
	state.UpdatedAtUTC, state.CanUndo = normalisedTime(now), true
	if err := recomputeEffectiveCover(ctx, tx, &state, false, s.random, now); err != nil {
		return gallery.CoverState{}, err
	}
	if err := persistCoverWithPrevious(ctx, tx, state, previous); err != nil {
		return gallery.CoverState{}, err
	}
	if err := touchGalleryCoverMetadata(ctx, tx, galleryID, &expectedMetadataRevision, now); err != nil {
		return gallery.CoverState{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.CoverState{}, err
	}
	return state, nil
}

func (s *CoverStore) SetManaged(ctx context.Context, galleryID int64, relativePath string, available bool, expectedMetadataRevision int64, now time.Time) (gallery.CoverState, error) {
	if err := manifest.ValidateManagedRelativeAsset(relativePath); err != nil {
		return gallery.CoverState{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.CoverState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	state, _, err := findCover(ctx, tx, galleryID)
	if err != nil {
		return gallery.CoverState{}, err
	}
	state = normalizedCoverState(state, galleryID, now)
	previous, _ := json.Marshal(state)
	state.GalleryID, state.PreferredKind, state.PreferredItemUUID, state.PreferredPath = galleryID, gallery.CoverManaged, "", relativePath
	state.Revision++
	if state.Revision == 1 {
		state.Revision = 1
	}
	state.UpdatedAtUTC, state.CanUndo = normalisedTime(now), true
	if err := recomputeEffectiveCover(ctx, tx, &state, available, s.random, now); err != nil {
		return gallery.CoverState{}, err
	}
	if err := persistCoverWithPrevious(ctx, tx, state, previous); err != nil {
		return gallery.CoverState{}, err
	}
	if err := touchGalleryCoverMetadata(ctx, tx, galleryID, &expectedMetadataRevision, now); err != nil {
		return gallery.CoverState{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.CoverState{}, err
	}
	return state, nil
}

func (s *CoverStore) ResetToAuto(ctx context.Context, galleryID int64, expectedMetadataRevision int64, now time.Time) (gallery.CoverState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.CoverState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	state, _, err := findCover(ctx, tx, galleryID)
	if err != nil {
		return gallery.CoverState{}, err
	}
	state = normalizedCoverState(state, galleryID, now)
	previous, _ := json.Marshal(state)
	selected, err := chooseStaticMember(ctx, tx, galleryID, "", s.random)
	if err != nil {
		return gallery.CoverState{}, err
	}
	state.GalleryID, state.PreferredPath, state.PreferredItemUUID = galleryID, "", selected
	state.PreferredKind = gallery.CoverNone
	if selected != "" {
		state.PreferredKind = gallery.CoverAutoRandom
	}
	state.Revision++
	if state.Revision == 1 {
		state.Revision = 1
	}
	state.UpdatedAtUTC, state.CanUndo = normalisedTime(now), true
	if err := recomputeEffectiveCover(ctx, tx, &state, false, s.random, now); err != nil {
		return gallery.CoverState{}, err
	}
	if err := persistCoverWithPrevious(ctx, tx, state, previous); err != nil {
		return gallery.CoverState{}, err
	}
	if err := touchGalleryCoverMetadata(ctx, tx, galleryID, &expectedMetadataRevision, now); err != nil {
		return gallery.CoverState{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.CoverState{}, err
	}
	return state, nil
}

func (s *CoverStore) Undo(ctx context.Context, galleryID int64, managedAvailable bool, expectedMetadataRevision int64, now time.Time) (gallery.CoverState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.CoverState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, found, err := findCover(ctx, tx, galleryID)
	if err != nil {
		return gallery.CoverState{}, err
	}
	if !found || !current.CanUndo {
		return gallery.CoverState{}, errors.New("cover has no undo snapshot")
	}
	var previousJSON []byte
	if err := tx.QueryRowContext(ctx, `SELECT previous_json FROM gallery_covers WHERE gallery_id=?`, galleryID).Scan(&previousJSON); err != nil {
		return gallery.CoverState{}, err
	}
	var restored gallery.CoverState
	if err := json.Unmarshal(previousJSON, &restored); err != nil {
		return gallery.CoverState{}, err
	}
	restored = normalizedCoverState(restored, galleryID, now)
	restored.Revision = current.Revision + 1
	restored.CanUndo = false
	if err := recomputeEffectiveCover(ctx, tx, &restored, managedAvailable, s.random, now); err != nil {
		return gallery.CoverState{}, err
	}
	if err := persistCoverClearingPrevious(ctx, tx, restored); err != nil {
		return gallery.CoverState{}, err
	}
	if err := touchGalleryCoverMetadata(ctx, tx, galleryID, &expectedMetadataRevision, now); err != nil {
		return gallery.CoverState{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.CoverState{}, err
	}
	return restored, nil
}

func (s *CoverStore) Reconcile(ctx context.Context, galleryID int64, managedAvailable bool, now time.Time) (gallery.CoverState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.CoverState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	state, found, err := findCover(ctx, tx, galleryID)
	if err != nil {
		return gallery.CoverState{}, err
	}
	if !found {
		return gallery.CoverState{}, sql.ErrNoRows
	}
	state.Revision++
	if err := recomputeEffectiveCover(ctx, tx, &state, managedAvailable, s.random, now); err != nil {
		return gallery.CoverState{}, err
	}
	if err := persistCover(ctx, tx, state); err != nil {
		return gallery.CoverState{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.CoverState{}, err
	}
	return state, nil
}

func (s *CoverStore) Find(ctx context.Context, galleryID int64) (gallery.CoverState, error) {
	state, found, err := findCover(ctx, s.db, galleryID)
	if err != nil {
		return gallery.CoverState{}, err
	}
	if !found {
		return gallery.CoverState{}, sql.ErrNoRows
	}
	return state, nil
}

func recomputeEffectiveCover(ctx context.Context, tx *sql.Tx, state *gallery.CoverState, managedAvailable bool, random io.Reader, now time.Time) error {
	state.EffectiveKind, state.EffectiveItemUUID, state.EffectivePath, state.WarningCode = gallery.CoverNone, "", "", ""
	state.UpdatedAtUTC = normalisedTime(now)
	if state.PreferredKind == gallery.CoverManaged && managedAvailable {
		state.EffectiveKind, state.EffectivePath = gallery.CoverManaged, state.PreferredPath
		return nil
	}
	if state.PreferredKind == gallery.CoverItem || state.PreferredKind == gallery.CoverAutoRandom {
		memberAvailable, err := isStaticMemberAvailable(ctx, tx, state.GalleryID, state.PreferredItemUUID)
		if err != nil {
			return err
		}
		available, err := isStaticCoverAvailable(ctx, tx, state.GalleryID, state.PreferredItemUUID)
		if err != nil {
			return err
		}
		if available {
			state.EffectiveKind, state.EffectiveItemUUID = gallery.CoverItem, state.PreferredItemUUID
			state.FallbackItemUUID = ""
			return nil
		}
		if state.PreferredKind == gallery.CoverAutoRandom && !memberAvailable {
			replacement, err := chooseStaticMember(ctx, tx, state.GalleryID, state.PreferredItemUUID, random)
			if err != nil {
				return err
			}
			state.PreferredItemUUID = replacement
			if replacement != "" {
				state.WarningCode = "AUTO_RANDOM_REINITIALIZED"
				displayable, err := isStaticCoverAvailable(ctx, tx, state.GalleryID, replacement)
				if err != nil {
					return err
				}
				if displayable {
					state.EffectiveKind, state.EffectiveItemUUID = gallery.CoverItem, replacement
					return nil
				}
			} else {
				state.PreferredKind = gallery.CoverNone
			}
		}
	}
	if state.PreferredKind == gallery.CoverManaged {
		state.WarningCode = "PREFERRED_COVER_MISSING"
	}
	if state.PreferredKind == gallery.CoverItem {
		memberAvailable, err := isStaticMemberAvailable(ctx, tx, state.GalleryID, state.PreferredItemUUID)
		if err != nil {
			return err
		}
		if !memberAvailable {
			state.WarningCode = "PREFERRED_COVER_MISSING"
		}
	}
	fallback, err := chooseStaticCover(ctx, tx, state.GalleryID, state.PreferredItemUUID, random)
	if err != nil {
		return err
	}
	state.FallbackItemUUID = fallback
	if fallback != "" {
		state.EffectiveKind, state.EffectiveItemUUID = gallery.CoverItem, fallback
		return nil
	}
	var videoUUID string
	err = tx.QueryRowContext(ctx, `SELECT item.item_uuid FROM gallery_items item JOIN media_derivatives derivative ON derivative.item_uuid=item.item_uuid
		WHERE item.gallery_id=? AND item.media_kind='VIDEO' AND item.excluded=0 AND item.availability_state='AVAILABLE'
		AND item.processing_state='READY' AND derivative.variant=? AND derivative.is_current=1 AND derivative.state IN ('READY','STALE')
		ORDER BY item.position,item.item_uuid LIMIT 1`, state.GalleryID, mediaprocessing.VariantStaticPoster).Scan(&videoUUID)
	if err == nil {
		state.EffectiveKind, state.EffectiveItemUUID = gallery.CoverVideoPoster, videoUUID
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

func chooseStaticCover(ctx context.Context, tx *sql.Tx, galleryID int64, excludedUUID string, random io.Reader) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT item.item_uuid FROM gallery_items item JOIN media_derivatives derivative ON derivative.item_uuid=item.item_uuid
		WHERE item.gallery_id=? AND item.media_kind='STATIC_IMAGE' AND item.excluded=0 AND item.availability_state='AVAILABLE'
		AND item.processing_state='READY' AND item.item_uuid<>? AND derivative.variant=? AND derivative.is_current=1
		AND derivative.state IN ('READY','STALE') ORDER BY item.position,item.item_uuid`, galleryID, excludedUUID, mediaprocessing.VariantCard480)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return "", err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil || len(values) == 0 {
		return "", err
	}
	var bytes [8]byte
	if _, err := io.ReadFull(random, bytes[:]); err != nil {
		return "", err
	}
	return values[binary.LittleEndian.Uint64(bytes[:])%uint64(len(values))], nil
}

func chooseStaticMember(ctx context.Context, tx *sql.Tx, galleryID int64, excludedUUID string, random io.Reader) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT item_uuid FROM gallery_items
		WHERE gallery_id=? AND media_kind='STATIC_IMAGE' AND excluded=0 AND availability_state='AVAILABLE' AND item_uuid<>?
		ORDER BY position,item_uuid`, galleryID, excludedUUID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return "", err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil || len(values) == 0 {
		return "", err
	}
	var bytes [8]byte
	if _, err := io.ReadFull(random, bytes[:]); err != nil {
		return "", err
	}
	return values[binary.LittleEndian.Uint64(bytes[:])%uint64(len(values))], nil
}

func isStaticMemberAvailable(ctx context.Context, tx *sql.Tx, galleryID int64, itemUUID string) (bool, error) {
	if itemUUID == "" {
		return false, nil
	}
	var available int
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gallery_items WHERE gallery_id=? AND item_uuid=?
		AND media_kind='STATIC_IMAGE' AND excluded=0 AND availability_state='AVAILABLE')`, galleryID, itemUUID).Scan(&available)
	return available == 1, err
}

func isStaticCoverAvailable(ctx context.Context, tx *sql.Tx, galleryID int64, itemUUID string) (bool, error) {
	if itemUUID == "" {
		return false, nil
	}
	var available int
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gallery_items item JOIN media_derivatives derivative ON derivative.item_uuid=item.item_uuid
		WHERE item.gallery_id=? AND item.item_uuid=? AND item.media_kind='STATIC_IMAGE' AND item.excluded=0
		AND item.availability_state='AVAILABLE' AND item.processing_state='READY' AND derivative.variant=?
		AND derivative.is_current=1 AND derivative.state IN ('READY','STALE'))`, galleryID, itemUUID, mediaprocessing.VariantCard480).Scan(&available)
	return available == 1, err
}

func touchGalleryCoverMetadata(ctx context.Context, tx *sql.Tx, galleryID int64, expected *int64, now time.Time) error {
	query := `UPDATE galleries SET metadata_revision=metadata_revision+1,updated_at_utc=? WHERE id=?`
	args := []any{formatTime(normalisedTime(now)), galleryID}
	if expected != nil {
		query += ` AND metadata_revision=?`
		args = append(args, *expected)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return ErrMetadataRevisionConflict
	}
	_, err = tx.ExecContext(ctx, `UPDATE gallery_manifest_sync SET status=CASE WHEN status='CLEAN' THEN 'DB_DIRTY' ELSE status END WHERE gallery_id=?`, galleryID)
	return err
}

func findCover(ctx context.Context, queryer galleryQueryer, galleryID int64) (gallery.CoverState, bool, error) {
	var result gallery.CoverState
	var preferredItem, fallbackItem, effectiveItem sql.NullString
	var previous []byte
	var updated string
	err := queryer.QueryRowContext(ctx, `SELECT gallery_id,preferred_kind,preferred_item_uuid,preferred_path,
		fallback_item_uuid,effective_kind,effective_item_uuid,effective_path,warning_code,cover_revision,previous_json,updated_at_utc
		FROM gallery_covers WHERE gallery_id=?`, galleryID).Scan(&result.GalleryID, &result.PreferredKind, &preferredItem,
		&result.PreferredPath, &fallbackItem, &result.EffectiveKind, &effectiveItem, &result.EffectivePath, &result.WarningCode,
		&result.Revision, &previous, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return gallery.CoverState{GalleryID: galleryID}, false, nil
	}
	if err != nil {
		return gallery.CoverState{}, false, err
	}
	result.PreferredItemUUID, result.FallbackItemUUID, result.EffectiveItemUUID = preferredItem.String, fallbackItem.String, effectiveItem.String
	result.CanUndo = len(previous) > 0
	result.UpdatedAtUTC, err = parseTime(updated)
	return result, true, err
}

func persistCover(ctx context.Context, tx *sql.Tx, state gallery.CoverState) error {
	return persistCoverWithPrevious(ctx, tx, state, nil)
}

func persistCoverClearingPrevious(ctx context.Context, tx *sql.Tx, state gallery.CoverState) error {
	if err := persistCoverWithPrevious(ctx, tx, state, nil); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE gallery_covers SET previous_json=NULL WHERE gallery_id=?`, state.GalleryID)
	return err
}

func normalizedCoverState(state gallery.CoverState, galleryID int64, now time.Time) gallery.CoverState {
	state.GalleryID = galleryID
	if state.PreferredKind == "" {
		state.PreferredKind = gallery.CoverNone
	}
	if state.EffectiveKind == "" {
		state.EffectiveKind = gallery.CoverNone
	}
	if state.UpdatedAtUTC.IsZero() {
		state.UpdatedAtUTC = normalisedTime(now)
	}
	return state
}
func persistCoverWithPrevious(ctx context.Context, tx *sql.Tx, state gallery.CoverState, previous []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO gallery_covers (gallery_id,preferred_kind,preferred_item_uuid,preferred_path,
		fallback_item_uuid,effective_kind,effective_item_uuid,effective_path,warning_code,cover_revision,previous_json,updated_at_utc)
		VALUES (?, ?,NULLIF(?,''),?,NULLIF(?,''),?,NULLIF(?,''),?,?,?,NULLIF(?,''),?)
		ON CONFLICT(gallery_id) DO UPDATE SET preferred_kind=excluded.preferred_kind,preferred_item_uuid=excluded.preferred_item_uuid,
		preferred_path=excluded.preferred_path,fallback_item_uuid=excluded.fallback_item_uuid,effective_kind=excluded.effective_kind,
		effective_item_uuid=excluded.effective_item_uuid,effective_path=excluded.effective_path,warning_code=excluded.warning_code,
		cover_revision=excluded.cover_revision,previous_json=COALESCE(excluded.previous_json,gallery_covers.previous_json),updated_at_utc=excluded.updated_at_utc`,
		state.GalleryID, state.PreferredKind, state.PreferredItemUUID, state.PreferredPath, state.FallbackItemUUID, state.EffectiveKind,
		state.EffectiveItemUUID, state.EffectivePath, state.WarningCode, state.Revision, previous, formatTime(state.UpdatedAtUTC))
	return err
}
