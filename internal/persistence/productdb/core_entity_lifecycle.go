package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/stashapp/stash/internal/portableid"
	"golang.org/x/text/unicode/norm"
)

type coreEntityTable struct {
	name string
	kind portableid.Kind
}

func tableForCoreKind(kind portableid.Kind) (coreEntityTable, error) {
	switch kind {
	case portableid.KindCoser:
		return coreEntityTable{name: "cosers", kind: kind}, nil
	case portableid.KindWork:
		return coreEntityTable{name: "works", kind: kind}, nil
	case portableid.KindCharacter:
		return coreEntityTable{name: "characters", kind: kind}, nil
	case portableid.KindTag:
		return coreEntityTable{name: "tags", kind: kind}, nil
	default:
		return coreEntityTable{}, fmt.Errorf("%s is not a core entity kind", kind)
	}
}

// ChangeSlug is the only operation that changes a stable route slug. Ordinary
// name edits intentionally leave it untouched. The old route is retained as a
// lightweight redirect to the entity UUID.
func (s *CoreEntityStore) ChangeSlug(ctx context.Context, kind portableid.Kind, uuid string, expectedRevision int64, requested string, now time.Time) (string, error) {
	table, err := tableForCoreKind(kind)
	if err != nil {
		return "", err
	}
	newSlug, err := validateCustomSlug(requested)
	if err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var oldSlug string
	var revision int64
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT slug, metadata_revision FROM %s WHERE uuid = ?`, table.name), uuid).Scan(&oldSlug, &revision); err != nil {
		return "", err
	}
	if revision != expectedRevision {
		return "", ErrCoreMetadataRevisionConflict
	}
	if oldSlug == newSlug {
		return oldSlug, tx.Commit()
	}
	result, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET slug = ?, metadata_revision = metadata_revision + 1, updated_at_utc = ? WHERE uuid = ? AND metadata_revision = ?`, table.name), newSlug, formatTime(normalisedTime(now)), uuid, expectedRevision)
	if err != nil {
		return "", err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return "", ErrCoreMetadataRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO slug_redirects (entity_kind, old_slug, target_uuid, created_at_utc)
		VALUES (?, ?, ?, ?) ON CONFLICT(entity_kind, old_slug) DO UPDATE SET target_uuid = excluded.target_uuid,
		created_at_utc = excluded.created_at_utc`, kind, oldSlug, uuid, formatTime(normalisedTime(now))); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM slug_redirects WHERE entity_kind = ? AND old_slug = ?`, kind, newSlug); err != nil {
		return "", err
	}
	if err := markEntityGalleriesManifestDirty(ctx, tx, kind, uuid); err != nil {
		return "", err
	}
	if kind == portableid.KindCoser {
		if _, err := tx.ExecContext(ctx, `UPDATE coser_manifest_sync SET status = CASE WHEN status = 'CLEAN' THEN 'DB_DIRTY' ELSE status END WHERE coser_uuid = ?`, uuid); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return newSlug, nil
}

func validateCustomSlug(value string) (string, error) {
	value = norm.NFC.String(strings.TrimSpace(value))
	if value == "" || value == "." || value == ".." || len([]rune(value)) > 400 {
		return "", errors.New("slug must contain 1 to 400 characters")
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) || strings.ContainsRune("/?#\\%", character) {
			return "", errors.New("slug must be one safe URL path segment")
		}
	}
	return value, nil
}

// ResolveSlug returns the active UUID and whether a historical redirect was
// used. UUID remains the identity; the redirect is intentionally lightweight.
func (s *CoreEntityStore) ResolveSlug(ctx context.Context, kind portableid.Kind, value string) (uuid string, redirected bool, err error) {
	table, err := tableForCoreKind(kind)
	if err != nil {
		return "", false, err
	}
	err = s.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT uuid FROM %s WHERE slug = ?`, table.name), value).Scan(&uuid)
	if err == nil {
		return uuid, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT target_uuid FROM slug_redirects WHERE entity_kind = ? AND old_slug = ?`, kind, value).Scan(&uuid)
	if err != nil {
		return "", false, err
	}
	record, err := lookupPortableUUID(ctx, s.db, uuid)
	if err != nil || record.State != PortableUUIDActive || record.Kind != kind {
		if err == nil {
			err = ErrPortableUUIDNotActive
		}
		return "", false, err
	}
	return uuid, true, nil
}

// DeleteCoreEntity permanently removes an unreferenced entity from business
// tables and tombstones its UUID. It never deletes media or Coser assets.
func (s *CoreEntityStore) DeleteCoreEntity(ctx context.Context, kind portableid.Kind, uuid string, expectedRevision int64, reason string, now time.Time) error {
	table, err := tableForCoreKind(kind)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var revision int64
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT metadata_revision FROM %s WHERE uuid = ?`, table.name), uuid).Scan(&revision); err != nil {
		return err
	}
	if revision != expectedRevision {
		return ErrCoreMetadataRevisionConflict
	}
	referenced, err := coreEntityReferenceCount(ctx, tx, kind, uuid)
	if err != nil {
		return err
	}
	if referenced > 0 {
		return fmt.Errorf("%w: %s has %d reference(s)", ErrCoreEntityReferenced, kind, referenced)
	}
	if kind == portableid.KindCoser {
		rows, err := tx.QueryContext(ctx, `SELECT account_uuid FROM coser_social_accounts WHERE coser_uuid = ?`, uuid)
		if err != nil {
			return err
		}
		var accountUUIDs []string
		for rows.Next() {
			var accountUUID string
			if err := rows.Scan(&accountUUID); err != nil {
				rows.Close()
				return err
			}
			accountUUIDs = append(accountUUIDs, accountUUID)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, accountUUID := range accountUUIDs {
			if err := tombstonePortableUUID(ctx, tx, accountUUID, portableid.KindSocialAccount, "owning Coser deleted", now); err != nil {
				return err
			}
		}
	}
	if err := tombstonePortableUUID(ctx, tx, uuid, kind, reason, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE uuid = ?`, table.name), uuid); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM slug_redirects WHERE entity_kind = ? AND target_uuid = ?`, kind, uuid); err != nil {
		return err
	}
	return tx.Commit()
}

func tombstonePortableUUID(ctx context.Context, tx *sql.Tx, uuid string, kind portableid.Kind, reason string, now time.Time) error {
	record, err := lookupPortableUUID(ctx, tx, uuid)
	if err != nil {
		return err
	}
	if record.Kind != kind || record.State != PortableUUIDActive {
		return ErrPortableUUIDNotActive
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO portable_uuid_tombstones (uuid, entity_kind, deleted_at_utc, reason) VALUES (?, ?, ?, ?)`, uuid, kind, formatTime(normalisedTime(now)), reason)
	return err
}

func coreEntityReferenceCount(ctx context.Context, tx *sql.Tx, kind portableid.Kind, uuid string) (int64, error) {
	var query string
	switch kind {
	case portableid.KindCoser:
		query = `SELECT COUNT(*) FROM gallery_credits WHERE coser_uuid = ?`
	case portableid.KindWork:
		query = `SELECT COUNT(*) FROM characters WHERE work_uuid = ?`
	case portableid.KindCharacter:
		query = `SELECT COUNT(*) FROM gallery_cast WHERE character_uuid = ?`
	case portableid.KindTag:
		query = `SELECT (SELECT COUNT(*) FROM gallery_tags WHERE tag_uuid = ?) +
			(SELECT COUNT(*) FROM tag_edges WHERE parent_uuid = ? OR child_uuid = ?)`
	default:
		return 0, fmt.Errorf("unsupported core entity kind %s", kind)
	}
	var count int64
	args := []any{uuid}
	if kind == portableid.KindTag {
		args = []any{uuid, uuid, uuid}
	}
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func markEntityGalleriesManifestDirty(ctx context.Context, tx *sql.Tx, kind portableid.Kind, uuid string) error {
	var galleryQuery string
	switch kind {
	case portableid.KindCoser:
		galleryQuery = `SELECT gallery_id FROM gallery_credits WHERE coser_uuid = ?`
	case portableid.KindCharacter:
		galleryQuery = `SELECT gallery_id FROM gallery_cast WHERE character_uuid = ?`
	case portableid.KindWork:
		galleryQuery = `SELECT DISTINCT gc.gallery_id FROM gallery_cast gc JOIN characters c ON c.uuid = gc.character_uuid WHERE c.work_uuid = ?`
	case portableid.KindTag:
		galleryQuery = `SELECT gallery_id FROM gallery_tags WHERE tag_uuid = ?`
	default:
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE gallery_manifest_sync SET status = CASE WHEN status = 'CLEAN' THEN 'DB_DIRTY' ELSE status END
		WHERE gallery_id IN (`+galleryQuery+`)`, uuid)
	return err
}
