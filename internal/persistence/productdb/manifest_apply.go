package productdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/slug"
)

var ErrManifestNameHintConflict = errors.New("Manifest UUID name_hint conflicts with the database name")

func (s *ManifestStore) applyGalleryBusinessSnapshot(
	ctx context.Context,
	galleryID int64,
	expectedRevision int64,
	snapshot map[string]any,
	firstImport bool,
	portableImportID string,
	now time.Time,
) (int64, error) {
	title := stringValue(snapshot["title"])
	description := stringValue(snapshot["description"])
	contentRating := gallery.ContentRating(stringValue(snapshot["content_rating"]))
	photographer := stringValue(snapshot["photographer_name"])
	studio := stringValue(snapshot["studio_name"])
	shootDate, shootPrecision, err := snapshotShootDate(snapshot["shoot_date"])
	if err != nil {
		return 0, err
	}
	if err := validateGalleryMetadata(title, description, shootDate, shootPrecision, contentRating, photographer, studio); err != nil {
		return 0, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE galleries SET title = ?, description = ?, shoot_date = NULLIF(?, ''),
			shoot_date_precision = NULLIF(?, ''), content_rating = NULLIF(?, ''),
			shoot_date_origin = 'MANUAL',
			photographer_name = ?, studio_name = ?, metadata_revision = metadata_revision + 1,
			updated_at_utc = ?
		WHERE id = ? AND metadata_revision = ?
	`, title, description, shootDate, shootPrecision, contentRating, photographer, studio,
		formatTime(normalisedTime(now)), galleryID, expectedRevision)
	if err != nil {
		return 0, err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return 0, err
	}
	if _,err:=tx.ExecContext(ctx,`DELETE FROM gallery_capture_date_reviews WHERE gallery_id=? AND manual_date<>COALESCE((SELECT shoot_date FROM galleries WHERE id=?),'')`,galleryID,galleryID);err!=nil{return 0,err}

	if err := applyManifestRating(ctx, tx, galleryID, 0, snapshot["rating"], now); err != nil {
		return 0, err
	}
	creditIDs, err := applyManifestCredits(ctx, tx, galleryID, objectValue(snapshot["credits"]), now)
	if err != nil {
		return 0, err
	}
	if err := applyManifestCast(ctx, tx, galleryID, objectValue(snapshot["cast"]), creditIDs, now); err != nil {
		return 0, err
	}
	if err := applyManifestTags(ctx, tx, galleryID, objectValue(snapshot["tags"]), now); err != nil {
		return 0, err
	}
	if err := applyManifestLinks(ctx, tx, galleryID, objectValue(snapshot["external_links"]), portableImportID, now); err != nil {
		return 0, err
	}
	if err := applyManifestItems(ctx, tx, galleryID, objectValue(snapshot["items"]), firstImport, now); err != nil {
		return 0, err
	}
	if err := applyManifestExclusions(ctx, tx, galleryID, objectValue(snapshot["excluded_items"]), now); err != nil {
		return 0, err
	}
	if err := applyManifestCover(ctx, tx, galleryID, snapshot["cover"], now); err != nil {
		return 0, err
	}
	if err := reconcileGalleryCaptureDate(ctx, tx, galleryID, now); err != nil {
		return 0, err
	}
	if err := demoteInvalidActiveGallery(ctx, tx, galleryID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return expectedRevision + 1, nil
}

func applyManifestCover(ctx context.Context, tx *sql.Tx, galleryID int64, raw any, now time.Time) error {
	current, _, err := findCover(ctx, tx, galleryID)
	if err != nil {
		return err
	}
	current = normalizedCoverState(current, galleryID, now)
	previous, _ := json.Marshal(current)
	cover := objectValue(raw)
	current.PreferredKind, current.PreferredItemUUID, current.PreferredPath = gallery.CoverNone, "", ""
	if len(cover) > 0 {
		kind := gallery.CoverKind(stringValue(cover["kind"]))
		switch kind {
		case gallery.CoverAutoRandom, gallery.CoverItem:
			itemUUID := stringValue(cover["item_uuid"])
			var mediaKind gallery.MediaKind
			if err := tx.QueryRowContext(ctx, `SELECT media_kind FROM gallery_items WHERE gallery_id=? AND item_uuid=?`, galleryID, itemUUID).Scan(&mediaKind); err != nil {
				return errors.New("Manifest cover Item does not belong to Gallery")
			}
			if mediaKind != gallery.MediaKindStaticImage {
				return errors.New("Manifest Item cover must reference a static image")
			}
			current.PreferredKind, current.PreferredItemUUID = kind, itemUUID
		case gallery.CoverManaged:
			path := stringValue(cover["path"])
			if err := manifest.ValidateManagedRelativeAsset(path); err != nil {
				return err
			}
			current.PreferredKind, current.PreferredPath = kind, path
		default:
			return errors.New("Manifest cover has invalid kind")
		}
	} else {
		selected, err := chooseStaticCover(ctx, tx, galleryID, "", rand.Reader)
		if err != nil {
			return err
		}
		if selected != "" {
			current.PreferredKind, current.PreferredItemUUID = gallery.CoverAutoRandom, selected
		}
	}
	current.Revision++
	if current.Revision == 1 {
		current.Revision = 1
	}
	current.UpdatedAtUTC, current.CanUndo = normalisedTime(now), true
	// Managed asset existence is checked by the caller's source inspection;
	// absent/unknown here deliberately preserves intent and selects a fallback.
	if err := recomputeEffectiveCover(ctx, tx, &current, false, rand.Reader, now); err != nil {
		return err
	}
	return persistCoverWithPrevious(ctx, tx, current, previous)
}

func applyManifestCredits(
	ctx context.Context,
	tx *sql.Tx,
	galleryID int64,
	credits map[string]any,
	now time.Time,
) (map[string]int64, error) {
	if len(credits) > 100 {
		return nil, errors.New("Gallery Manifest credits exceeds 100")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_cast WHERE gallery_id = ?; DELETE FROM gallery_credits WHERE gallery_id = ?`, galleryID, galleryID); err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(credits))
	for _, key := range sortedObjectKeys(credits) {
		credit := objectValue(credits[key])
		coser := objectValue(credit["coser"])
		coserUUID := stringValue(coser["uuid"])
		if err := ensureCoserReference(ctx, tx, coserUUID, stringValue(coser["name_hint"]), now); err != nil {
			return nil, err
		}
		position, err := positivePosition(credit["position"], "credit")
		if err != nil {
			return nil, err
		}
		inserted, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_credits (gallery_id, coser_uuid, position) VALUES (?, ?, ?)
		`, galleryID, coserUUID, position)
		if err != nil {
			return nil, err
		}
		id, err := inserted.LastInsertId()
		if err != nil {
			return nil, err
		}
		result[coserUUID] = id
	}
	return result, nil
}

func applyManifestCast(
	ctx context.Context,
	tx *sql.Tx,
	galleryID int64,
	castItems map[string]any,
	creditIDs map[string]int64,
	now time.Time,
) error {
	if len(castItems) > 500 {
		return errors.New("Gallery Manifest cast exceeds 500")
	}
	for _, key := range sortedObjectKeys(castItems) {
		castItem := objectValue(castItems[key])
		coser := objectValue(castItem["coser"])
		character := objectValue(castItem["character"])
		work := objectValue(castItem["work"])
		coserUUID := stringValue(coser["uuid"])
		creditID, exists := creditIDs[coserUUID]
		if !exists {
			return errors.New("Manifest Cast references a Coser absent from Credits")
		}
		workUUID := stringValue(work["uuid"])
		if err := ensureWorkReference(ctx, tx, workUUID, stringValue(work["name_hint"]), now); err != nil {
			return err
		}
		characterUUID := stringValue(character["uuid"])
		if err := ensureCharacterReference(ctx, tx, characterUUID, workUUID, stringValue(character["name_hint"]), now); err != nil {
			return err
		}
		position, err := positivePosition(castItem["position"], "cast")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_cast (gallery_id, gallery_credit_id, character_uuid, position)
			VALUES (?, ?, ?, ?)
		`, galleryID, creditID, characterUUID, position); err != nil {
			return err
		}
	}
	return nil
}

func applyManifestTags(ctx context.Context, tx *sql.Tx, galleryID int64, tags map[string]any, now time.Time) error {
	if len(tags) > 200 {
		return errors.New("Gallery Manifest direct tags exceeds 200")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_tags WHERE gallery_id = ?`, galleryID); err != nil {
		return err
	}
	for _, key := range sortedObjectKeys(tags) {
		tag := objectValue(tags[key])
		uuid := stringValue(tag["uuid"])
		if err := ensureTagReference(ctx, tx, uuid, stringValue(tag["name_hint"]), now); err != nil {
			return err
		}
		position, err := positivePosition(tag["position"], "tag")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO gallery_tags (gallery_id, tag_uuid, position) VALUES (?, ?, ?)`, galleryID, uuid, position); err != nil {
			return err
		}
	}
	return nil
}

func applyManifestLinks(ctx context.Context, tx *sql.Tx, galleryID int64, links map[string]any, portableImportID string, now time.Time) error {
	if len(links) > 50 {
		return errors.New("Gallery Manifest ExternalLinks exceeds 50")
	}
	rows, err := tx.QueryContext(ctx, `SELECT link_uuid FROM gallery_external_links WHERE gallery_id = ?`, galleryID)
	if err != nil {
		return err
	}
	var previous []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			rows.Close()
			return err
		}
		previous = append(previous, uuid)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_external_links WHERE gallery_id = ?`, galleryID); err != nil {
		return err
	}
	desired := make(map[string]struct{}, len(links))
	for _, key := range sortedObjectKeys(links) {
		link := objectValue(links[key])
		uuid := stringValue(link["link_uuid"])
		desired[uuid] = struct{}{}
		record, err := lookupPortableUUID(ctx, tx, uuid)
		if errors.Is(err, ErrPortableUUIDNotFound) {
			if portableImportID != "" {
				if err := claimPortableUUID(ctx, tx, portableImportID, uuid, portableid.KindExternalLink, now); err != nil {
					return err
				}
			} else {
				if _, err := registerPortableUUID(ctx, tx, uuid, portableid.KindExternalLink, normalisedTime(now)); err != nil {
					return err
				}
			}
		} else if err != nil {
			return err
		} else if record.Kind != portableid.KindExternalLink || record.State != PortableUUIDActive {
			return errors.New("Manifest ExternalLink UUID is occupied by another identity")
		}
		linkType := gallery.ExternalLinkType(stringValue(link["type"]))
		if linkType != gallery.ExternalLinkSource && linkType != gallery.ExternalLinkProfile && linkType != gallery.ExternalLinkReference {
			return errors.New("Manifest ExternalLink has invalid type")
		}
		normalizedURL, err := validateExternalHTTPURL(stringValue(link["url"]))
		if err != nil {
			return err
		}
		position, err := positivePosition(link["position"], "ExternalLink")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_external_links (link_uuid, gallery_id, link_type, label, url, position)
			VALUES (?, ?, ?, ?, ?, ?)
		`, uuid, galleryID, linkType, normalizedDisplay(stringValue(link["label"])), normalizedURL, position); err != nil {
			return err
		}
	}
	for _, uuid := range previous {
		if _, retained := desired[uuid]; retained {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO portable_uuid_tombstones (uuid, entity_kind, deleted_at_utc, reason)
			VALUES (?, 'EXTERNAL_LINK', ?, 'GalleryExternalLink removed by Manifest Pull')
		`, uuid, formatTime(normalisedTime(now))); err != nil {
			return err
		}
	}
	return nil
}

func applyManifestItems(ctx context.Context, tx *sql.Tx, galleryID int64, items map[string]any, firstImport bool, now time.Time) error {
	type update struct {
		id       int64
		position int64
	}
	var positions []update
	for _, key := range sortedObjectKeys(items) {
		itemData := objectValue(items[key])
		itemUUID := stringValue(itemData["item_uuid"])
		path := stringValue(itemData["path"])
		var itemID int64
		if itemUUID != "" {
			if err := tx.QueryRowContext(ctx, `SELECT id FROM gallery_items WHERE gallery_id = ? AND item_uuid = ?`, galleryID, itemUUID).Scan(&itemID); err != nil {
				return fmt.Errorf("Manifest Item UUID does not belong to Gallery: %w", err)
			}
		} else {
			if !firstImport {
				return errors.New("Manifest Item requires item_uuid after first import")
			}
			if err := tx.QueryRowContext(ctx, `SELECT id FROM gallery_items WHERE gallery_id = ? AND relative_path = ?`, galleryID, path).Scan(&itemID); err != nil {
				return fmt.Errorf("first-import Manifest Item path did not match one Item: %w", err)
			}
		}
		item, err := findItem(ctx, tx, itemID)
		if err != nil {
			return err
		}
		category := gallery.ImageCategory(stringValue(itemData["category"]))
		if category == "" {
			category = item.ImageCategory
		}
		caption := stringValue(itemData["caption"])
		if runeLength(caption) > 1000 {
			return errors.New("Manifest Item Caption exceeds 1000 characters")
		}
		if item.MediaKind == gallery.MediaKindStaticImage && category != gallery.ImageCategoryPhoto && category != gallery.ImageCategorySelfie {
			return errors.New("Manifest static Item requires PHOTO or SELFIE")
		}
		if item.MediaKind != gallery.MediaKindStaticImage {
			category = ""
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE gallery_items SET image_category = NULLIF(?, ''), caption = ?, updated_at_utc = ? WHERE id = ?
		`, category, caption, formatTime(normalisedTime(now)), itemID); err != nil {
			return err
		}
		if err := applyManifestRating(ctx, tx, galleryID, itemID, itemData["rating"], now); err != nil {
			return err
		}
		if position := int64Value(itemData["position"]); position > 0 {
			positions = append(positions, update{id: itemID, position: position})
		}
	}
	if len(positions) > 0 {
		var highWater int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) FROM gallery_items WHERE gallery_id = ?`, galleryID).Scan(&highWater); err != nil {
			return err
		}
		for index, item := range positions {
			if _, err := tx.ExecContext(ctx, `UPDATE gallery_items SET position = ? WHERE id = ?`, highWater+int64(index+1)*1024, item.id); err != nil {
				return err
			}
		}
		for _, item := range positions {
			if _, err := tx.ExecContext(ctx, `UPDATE gallery_items SET position = ? WHERE id = ?`, item.position, item.id); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyManifestExclusions(ctx context.Context, tx *sql.Tx, galleryID int64, exclusions map[string]any, now time.Time) error {
	desired := make(map[int64]struct{}, len(exclusions))
	for _, key := range sortedObjectKeys(exclusions) {
		exclusion := objectValue(exclusions[key])
		var itemID int64
		switch {
		case stringValue(exclusion["item_uuid"]) != "":
			if err := tx.QueryRowContext(ctx, `SELECT id FROM gallery_items WHERE gallery_id = ? AND item_uuid = ?`, galleryID, stringValue(exclusion["item_uuid"])).Scan(&itemID); err != nil {
				return err
			}
		case stringValue(exclusion["fingerprint"]) != "":
			rows, err := tx.QueryContext(ctx, `SELECT id FROM gallery_items WHERE gallery_id = ? AND full_fingerprint = ?`, galleryID, stringValue(exclusion["fingerprint"]))
			if err != nil {
				return err
			}
			var ids []int64
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, id)
			}
			if err := rows.Close(); err != nil {
				return err
			}
			if len(ids) != 1 {
				return errors.New("Manifest exclusion fingerprint must uniquely match one GalleryItem")
			}
			itemID = ids[0]
		default:
			if err := tx.QueryRowContext(ctx, `SELECT id FROM gallery_items WHERE gallery_id = ? AND relative_path = ?`, galleryID, stringValue(exclusion["path"])).Scan(&itemID); err != nil {
				return err
			}
		}
		desired[itemID] = struct{}{}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, item_uuid, content_revision, excluded FROM gallery_items WHERE gallery_id = ?`, galleryID)
	if err != nil {
		return err
	}
	type state struct {
		id              int64
		uuid            string
		contentRevision int64
		excluded        bool
	}
	var states []state
	for rows.Next() {
		var value state
		if err := rows.Scan(&value.id, &value.uuid, &value.contentRevision, &value.excluded); err != nil {
			rows.Close()
			return err
		}
		states = append(states, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	restored := false
	for _, current := range states {
		_, shouldExclude := desired[current.id]
		if current.excluded == shouldExclude {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_items SET excluded = ?, updated_at_utc = ? WHERE id = ?`, shouldExclude, formatTime(normalisedTime(now)), current.id); err != nil {
			return err
		}
		decisionStatus, currentStatus := "SUPERSEDED", "PENDING"
		if !shouldExclude {
			decisionStatus, currentStatus = "REVERSED", "APPLIED"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE media_exclusion_decisions SET status=?,resolved_at_utc=?
			WHERE item_uuid=? AND status=?`, decisionStatus, formatTime(normalisedTime(now)), current.uuid, currentStatus); err != nil {
			return err
		}
		if shouldExclude {
			if err := cancelItemProcessingJobs(ctx, tx, current.uuid, now); err != nil {
				return err
			}
		} else {
			restored = true
			if err := resumeCancelledItemProcessingJobs(ctx, tx, current.uuid, current.contentRevision, now); err != nil {
				return err
			}
		}
	}
	if restored {
		if err := enqueueScanProcessingJobs(ctx, tx, galleryID, now); err != nil {
			return err
		}
	}
	return nil
}

func applyManifestRating(ctx context.Context, tx *sql.Tx, galleryID int64, itemID int64, raw any, now time.Time) error {
	var halfSteps *int
	if raw != nil {
		value, ok := raw.(float64)
		if !ok || value < 0.5 || value > 5 || value*2 != float64(int(value*2)) {
			return errors.New("Manifest rating must use half-star steps")
		}
		converted := int(value * 2)
		halfSteps = &converted
	}
	var ratedAt any
	if halfSteps != nil {
		ratedAt = formatTime(normalisedTime(now))
	}
	if itemID == 0 {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_personal_states (gallery_id, rating_half_steps, rated_at_utc)
			VALUES (?, ?, ?) ON CONFLICT(gallery_id) DO UPDATE SET
				rating_half_steps = excluded.rating_half_steps, rated_at_utc = excluded.rated_at_utc
		`, galleryID, halfSteps, ratedAt)
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_item_personal_states (gallery_item_id, rating_half_steps, rated_at_utc)
		VALUES (?, ?, ?) ON CONFLICT(gallery_item_id) DO UPDATE SET
			rating_half_steps = excluded.rating_half_steps, rated_at_utc = excluded.rated_at_utc
	`, itemID, halfSteps, ratedAt)
	return err
}

func ensureCoserReference(ctx context.Context, tx *sql.Tx, uuid string, hint string, now time.Time) error {
	return ensureNamedReference(ctx, tx, uuid, hint, portableid.KindCoser, "cosers", "", now)
}

func ensureWorkReference(ctx context.Context, tx *sql.Tx, uuid string, hint string, now time.Time) error {
	return ensureNamedReference(ctx, tx, uuid, hint, portableid.KindWork, "works", "", now)
}

func ensureCharacterReference(ctx context.Context, tx *sql.Tx, uuid string, workUUID string, hint string, now time.Time) error {
	if err := ensureNamedReference(ctx, tx, uuid, hint, portableid.KindCharacter, "characters", workUUID, now); err != nil {
		return err
	}
	var actualWork string
	if err := tx.QueryRowContext(ctx, `SELECT work_uuid FROM characters WHERE uuid = ?`, uuid).Scan(&actualWork); err != nil {
		return err
	}
	if actualWork != workUUID {
		return errors.New("Manifest Character Work conflicts with database relationship")
	}
	return nil
}

func ensureTagReference(ctx context.Context, tx *sql.Tx, uuid string, hint string, now time.Time) error {
	return ensureNamedReference(ctx, tx, uuid, hint, portableid.KindTag, "tags", "", now)
}

func ensureNamedReference(ctx context.Context, tx *sql.Tx, uuid string, hint string, kind portableid.Kind, table string, workUUID string, now time.Time) error {
	if _, err := portableid.Parse(uuid); err != nil {
		return err
	}
	record, lookupErr := lookupPortableUUID(ctx, tx, uuid)
	if lookupErr != nil && !errors.Is(lookupErr, ErrPortableUUIDNotFound) {
		return lookupErr
	}
	if lookupErr == nil && (record.Kind != kind || record.State != PortableUUIDActive) {
		return errors.New("Manifest UUID is occupied by another or retired identity")
	}
	var existingName string
	err := tx.QueryRowContext(ctx, `SELECT name FROM `+table+` WHERE uuid = ?`, uuid).Scan(&existingName)
	if err == nil {
		if hint != "" && normalizedDisplay(hint) != existingName {
			return fmt.Errorf("%w: %s", ErrManifestNameHintConflict, uuid)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if normalizedDisplay(hint) == "" {
		return errors.New("Manifest cannot create missing entity without name_hint")
	}
	if lookupErr != nil {
		if _, err := registerPortableUUID(ctx, tx, uuid, kind, normalisedTime(now)); err != nil {
			return err
		}
	}
	timestamp := formatTime(normalisedTime(now))
	switch kind {
	case portableid.KindCoser:
		_, err = tx.ExecContext(ctx, `INSERT INTO cosers (uuid, name, slug, created_at_utc, updated_at_utc) VALUES (?, ?, ?, ?, ?)`, uuid, normalizedDisplay(hint), slug.FromName(hint, uuid), timestamp, timestamp)
	case portableid.KindWork:
		_, err = tx.ExecContext(ctx, `INSERT INTO works (uuid, name, slug, created_at_utc, updated_at_utc) VALUES (?, ?, ?, ?, ?)`, uuid, normalizedDisplay(hint), slug.FromName(hint, uuid), timestamp, timestamp)
	case portableid.KindCharacter:
		_, err = tx.ExecContext(ctx, `INSERT INTO characters (uuid, work_uuid, name, normalized_name, slug, created_at_utc, updated_at_utc) VALUES (?, ?, ?, ?, ?, ?, ?)`, uuid, workUUID, normalizedDisplay(hint), normalizedKey(hint), slug.FromName(hint, uuid), timestamp, timestamp)
	case portableid.KindTag:
		if occupied, checkErr := tagNameOccupied(ctx, tx, normalizedKey(hint)); checkErr != nil {
			return checkErr
		} else if occupied {
			return ErrTagNameAmbiguous
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO tags (uuid, name, normalized_name, slug, created_at_utc, updated_at_utc) VALUES (?, ?, ?, ?, ?, ?)`, uuid, normalizedDisplay(hint), normalizedKey(hint), slug.FromName(hint, uuid), timestamp, timestamp)
	}
	return err
}

func snapshotShootDate(value any) (string, gallery.ShootDatePrecision, error) {
	if value == nil {
		return "", "", nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return "", "", errors.New("Manifest shoot_date must be an object or null")
	}
	return stringValue(object["value"]), gallery.ShootDatePrecision(stringValue(object["precision"])), nil
}

func objectValue(value any) map[string]any {
	if result, ok := value.(map[string]any); ok {
		return result
	}
	return map[string]any{}
}

func stringValue(value any) string {
	if result, ok := value.(string); ok {
		return result
	}
	return ""
}

func int64Value(value any) int64 {
	switch converted := value.(type) {
	case float64:
		return int64(converted)
	case int64:
		return converted
	case int:
		return int64(converted)
	default:
		return 0
	}
}

func positivePosition(value any, field string) (int64, error) {
	position := int64Value(value)
	if position <= 0 {
		return 0, fmt.Errorf("Manifest %s position must be positive", field)
	}
	return position, nil
}

func sortedObjectKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
