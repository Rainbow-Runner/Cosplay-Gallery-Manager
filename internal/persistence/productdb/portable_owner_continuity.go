package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portablecatalog"
)

type PortableOwnerContinuityResult struct {
	GalleryCount  int
	ItemCount     int
	ActiveCount   int
	ArchivedCount int
}

// ApplyPortableOwnerContinuity applies only the intentionally small, optional
// owner state partition. Ratings are manifest-owned and browsing history is
// deliberately untouched. The whole partition is atomic and retryable.
func (db *Database) ApplyPortableOwnerContinuity(ctx context.Context, importID string, owner portablecatalog.OwnerContinuity, now time.Time) (PortableOwnerContinuityResult, error) {
	var sessionState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM portable_import_sessions WHERE import_id=?`, importID).Scan(&sessionState); err != nil {
		return PortableOwnerContinuityResult{}, err
	}
	if sessionState != "GALLERIES_REBUILT" {
		return PortableOwnerContinuityResult{}, errors.New("portable owner continuity requires completed Gallery rebuild")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return PortableOwnerContinuityResult{}, err
	}
	defer tx.Rollback()
	result := PortableOwnerContinuityResult{}
	timestamp := formatTime(normalisedTime(now))
	for _, incoming := range owner.Galleries {
		current, err := findGalleryBySetID(ctx, tx, incoming.SetID)
		if err != nil {
			return result, fmt.Errorf("owner continuity Gallery identity is unavailable: %w", err)
		}
		if incoming.State == string(gallery.StateActive) {
			facts, err := activationFacts(ctx, tx, current.ID)
			if err != nil {
				return result, err
			}
			if blockers := current.ActivationBlockers(facts); len(blockers) > 0 {
				return result, &ActivationError{Blockers: blockers}
			}
			result.ActiveCount++
		} else if incoming.State == string(gallery.StateArchived) {
			result.ArchivedCount++
		}
		if owner.IncludesGalleryLifecycle {
			if _, err := tx.ExecContext(ctx, `UPDATE galleries SET state=?,added_at_utc=NULLIF(?,''),updated_at_utc=?,metadata_revision=metadata_revision+1 WHERE id=?`, incoming.State, incoming.AddedAt, timestamp, current.ID); err != nil {
				return result, err
			}
		}
		if owner.IncludesPersonalFlags {
			favoriteAt := any(nil)
			if incoming.Favorite {
				favoriteAt = incoming.FavoritedAt
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO gallery_personal_states(gallery_id,favorite,favorited_at_utc,hidden) VALUES(?,?,?,?)
			ON CONFLICT(gallery_id) DO UPDATE SET favorite=excluded.favorite,favorited_at_utc=excluded.favorited_at_utc,hidden=excluded.hidden`, current.ID, incoming.Favorite, favoriteAt, incoming.Hidden); err != nil {
				return result, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE gallery_item_personal_states SET favorite=0,favorited_at_utc=NULL WHERE gallery_item_id IN (SELECT id FROM gallery_items WHERE gallery_id=?)`, current.ID); err != nil {
				return result, err
			}
			for _, item := range incoming.Items {
				var itemID int64
				if err := tx.QueryRowContext(ctx, `SELECT id FROM gallery_items WHERE gallery_id=? AND item_uuid=?`, current.ID, item.ItemUUID).Scan(&itemID); err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return result, errors.New("owner continuity Item identity is unavailable in its Gallery")
					}
					return result, err
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO gallery_item_personal_states(gallery_item_id,favorite,favorited_at_utc) VALUES(?,1,?)
				ON CONFLICT(gallery_item_id) DO UPDATE SET favorite=1,favorited_at_utc=excluded.favorited_at_utc`, itemID, item.FavoritedAt); err != nil {
					return result, err
				}
				result.ItemCount++
			}
		}
		result.GalleryCount++
	}
	if err := tx.Commit(); err != nil {
		return PortableOwnerContinuityResult{}, err
	}
	return result, nil
}

func findGalleryBySetID(ctx context.Context, queryer galleryQueryer, setID string) (gallery.Gallery, error) {
	var id int64
	if err := queryer.QueryRowContext(ctx, `SELECT id FROM galleries WHERE set_id=?`, setID).Scan(&id); err != nil {
		return gallery.Gallery{}, err
	}
	return findGallery(ctx, queryer, id)
}
