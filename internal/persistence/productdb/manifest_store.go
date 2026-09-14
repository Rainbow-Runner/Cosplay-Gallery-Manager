package productdb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portableid"
)

var (
	ErrManifestFileDirty = errors.New("Manifest changed outside the application; Pull or resolve it before Push")
	ErrUntrackedManifest = errors.New("existing Manifest has no synchronization baseline and cannot be overwritten")
)

type ManifestStatus string

const (
	ManifestNone      ManifestStatus = "NONE"
	ManifestClean     ManifestStatus = "CLEAN"
	ManifestDBDirty   ManifestStatus = "DB_DIRTY"
	ManifestFileDirty ManifestStatus = "FILE_DIRTY"
	ManifestConflict  ManifestStatus = "CONFLICT"
	ManifestMissing   ManifestStatus = "MISSING"
	ManifestError     ManifestStatus = "ERROR"
)

type GalleryManifestState struct {
	GalleryID        int64
	Path             string
	Status           ManifestStatus
	ManifestRevision int64
	FileHash         string
	Conflicts        []manifest.Conflict
}

type GalleryManifestPushPreview struct {
	Added, Removed, Retained, Updated int
}

type ManifestStore struct {
	db *sql.DB
}

func (db *Database) Manifests() *ManifestStore {
	return &ManifestStore{db: db.DB}
}

// PreviewGalleryPush compares only portable member identities and their
// manifest business fields. It neither reads nor writes the source file; Push
// still performs its independent baseline/hash safety checks.
func (s *ManifestStore) PreviewGalleryPush(ctx context.Context, galleryID int64) (GalleryManifestPushPreview, error) {
	_, baseline, _, found, err := loadGalleryManifestState(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestPushPreview{}, err
	}
	current, err := buildGalleryBusinessSnapshot(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestPushPreview{}, err
	}
	baselineItems := map[string]any{}
	if found {
		baselineItems = objectValue(baseline["items"])
	}
	currentItems := objectValue(current["items"])
	preview := GalleryManifestPushPreview{}
	for uuid, currentItem := range currentItems {
		baselineItem, exists := baselineItems[uuid]
		if !exists {
			preview.Added++
			continue
		}
		preview.Retained++
		if !reflect.DeepEqual(baselineItem, currentItem) {
			preview.Updated++
		}
	}
	for uuid := range baselineItems {
		if _, exists := currentItems[uuid]; !exists {
			preview.Removed++
		}
	}
	return preview, nil
}

func (s *ManifestStore) CheckGallery(ctx context.Context, galleryID int64, now time.Time) (GalleryManifestState, error) {
	source, err := findGallerySourceByGallery(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{GalleryID: galleryID, Status: ManifestNone}, nil
	}
	manifestPath, err := manifest.GalleryPath(source.Type, source.Path)
	if err != nil {
		return GalleryManifestState{}, err
	}
	state, baseline, baselineRevision, found, err := loadGalleryManifestState(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	state.Path = manifestPath
	data, currentHash, readErr := manifest.ReadFile(manifestPath, manifest.MaxGalleryBytes)
	if errors.Is(readErr, os.ErrNotExist) {
		if !found {
			state.Status = ManifestNone
			return state, nil
		}
		state.Status = ManifestMissing
		if err := s.persistManifestInspection(ctx, galleryID, state.Status, now, nil); err != nil {
			return GalleryManifestState{}, err
		}
		return state, nil
	}
	if readErr != nil {
		state.Status = ManifestError
		if found {
			_ = s.persistManifestInspection(ctx, galleryID, state.Status, now, nil)
		}
		return state, readErr
	}
	if !found {
		state.Status = ManifestFileDirty
		state.FileHash = currentHash
		return state, nil
	}
	document, err := manifest.ParseGallery(bytesReader(data))
	if err != nil || document.SetID != mustGallerySetID(ctx, s.db, galleryID) {
		state.Status = ManifestError
		_ = s.persistManifestInspection(ctx, galleryID, state.Status, now, nil)
		if err != nil {
			return state, err
		}
		return state, errors.New("Manifest set_id does not match Gallery")
	}
	currentGallery, err := findGallery(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	databaseChanged := currentGallery.MetadataRevision != baselineRevision || state.Status == ManifestDBDirty
	fileChanged := currentHash != state.FileHash
	state.FileHash = currentHash
	switch {
	case !databaseChanged && !fileChanged:
		state.Status = ManifestClean
	case databaseChanged && !fileChanged:
		state.Status = ManifestDBDirty
	case !databaseChanged && fileChanged:
		state.Status = ManifestFileDirty
	default:
		databaseSnapshot, err := buildGalleryBusinessSnapshot(ctx, s.db, galleryID)
		if err != nil {
			return GalleryManifestState{}, err
		}
		fileSnapshot, err := overlayGalleryDocument(baseline, document)
		if err != nil {
			return GalleryManifestState{}, err
		}
		_, state.Conflicts = manifest.ThreeWayMerge(baseline, databaseSnapshot, fileSnapshot)
		if len(state.Conflicts) > 0 {
			state.Status = ManifestConflict
		} else {
			state.Status = ManifestDBDirty
		}
	}
	if err := s.persistManifestInspection(ctx, galleryID, state.Status, now, state.Conflicts); err != nil {
		return GalleryManifestState{}, err
	}
	return state, nil
}

func (s *ManifestStore) PushGallery(
	ctx context.Context,
	galleryID int64,
	expectedMetadataRevision int64,
	now time.Time,
) (GalleryManifestState, error) {
	current, err := findGallery(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	if current.MetadataRevision != expectedMetadataRevision {
		return GalleryManifestState{}, ErrMetadataRevisionConflict
	}
	source, err := findGallerySourceByGallery(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	if source.LibraryID != nil {
		var readOnly int
		if err := s.db.QueryRowContext(ctx, `SELECT read_only FROM media_libraries WHERE id = ?`, *source.LibraryID).Scan(&readOnly); err != nil {
			return GalleryManifestState{}, err
		}
		if readOnly == 1 {
			return GalleryManifestState{}, errors.New("media library is read-only; Manifest Push is unavailable")
		}
	}
	path, err := manifest.GalleryPath(source.Type, source.Path)
	if err != nil {
		return GalleryManifestState{}, err
	}
	state, _, _, found, err := loadGalleryManifestState(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	if data, hash, readErr := manifest.ReadFile(path, manifest.MaxGalleryBytes); readErr == nil {
		_ = data
		if !found {
			return GalleryManifestState{}, ErrUntrackedManifest
		}
		if hash != state.FileHash {
			return GalleryManifestState{}, ErrManifestFileDirty
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return GalleryManifestState{}, readErr
	} else if found && state.Status != ManifestMissing {
		// A previously synchronized file disappeared. Push remains explicit and
		// may recreate it; its previous baseline is retained below.
	}

	business, err := buildGalleryBusinessSnapshot(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	revision := state.ManifestRevision + 1
	document := businessDocument(business)
	document["schema_version"] = manifest.GallerySchemaVersion
	document["revision"] = revision
	document["set_id"] = current.SetID
	document["updated_at"] = normalisedTime(now).Format(time.RFC3339)
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return GalleryManifestState{}, err
	}
	data = append(data, '\n')
	fileHash, err := manifest.WriteAtomic(path, data, manifest.MaxGalleryBytes)
	if err != nil {
		return GalleryManifestState{}, err
	}
	baselineJSON, err := json.Marshal(business)
	if err != nil {
		return GalleryManifestState{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_manifest_sync (
			gallery_id, manifest_path, status, schema_version, manifest_revision,
			file_hash, baseline_json, baseline_metadata_revision, checked_at_utc
		) VALUES (?, ?, 'CLEAN', ?, ?, ?, ?, ?, ?)
		ON CONFLICT(gallery_id) DO UPDATE SET manifest_path = excluded.manifest_path,
			status = 'CLEAN', schema_version = excluded.schema_version,
			manifest_revision = excluded.manifest_revision, file_hash = excluded.file_hash,
			baseline_json = excluded.baseline_json,
			baseline_metadata_revision = excluded.baseline_metadata_revision,
			checked_at_utc = excluded.checked_at_utc, last_error_code = ''
	`, galleryID, path, manifest.GallerySchemaVersion, revision, fileHash,
		baselineJSON, current.MetadataRevision, formatTime(normalisedTime(now))); err != nil {
		return GalleryManifestState{}, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM gallery_manifest_conflicts WHERE gallery_id = ?`, galleryID); err != nil {
		return GalleryManifestState{}, err
	}
	return GalleryManifestState{
		GalleryID: galleryID, Path: path, Status: ManifestClean,
		ManifestRevision: revision, FileHash: fileHash,
	}, nil
}

func (s *ManifestStore) PullGallery(
	ctx context.Context,
	galleryID int64,
	expectedMetadataRevision int64,
	now time.Time,
) (GalleryManifestState, error) {
	return s.pullGallery(ctx, galleryID, expectedMetadataRevision, nil, false, "", now)
}

func (s *ManifestStore) PullPortableGallery(ctx context.Context, galleryID int64, expectedMetadataRevision int64, importID string, now time.Time) (GalleryManifestState, error) {
	if _, err := portableid.Parse(importID); err != nil {
		return GalleryManifestState{}, err
	}
	return s.pullGallery(ctx, galleryID, expectedMetadataRevision, nil, false, importID, now)
}

func (s *ManifestStore) ResolveGalleryConflicts(
	ctx context.Context,
	galleryID int64,
	expectedMetadataRevision int64,
	choices map[string]manifest.ConflictChoice,
	now time.Time,
) (GalleryManifestState, error) {
	return s.pullGallery(ctx, galleryID, expectedMetadataRevision, choices, true, "", now)
}

func (s *ManifestStore) pullGallery(
	ctx context.Context,
	galleryID int64,
	expectedMetadataRevision int64,
	choices map[string]manifest.ConflictChoice,
	resolve bool,
	portableImportID string,
	now time.Time,
) (GalleryManifestState, error) {
	current, err := findGallery(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	if current.MetadataRevision != expectedMetadataRevision {
		return GalleryManifestState{}, ErrMetadataRevisionConflict
	}
	source, err := findGallerySourceByGallery(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	path, err := manifest.GalleryPath(source.Type, source.Path)
	if err != nil {
		return GalleryManifestState{}, err
	}
	data, fileHash, err := manifest.ReadFile(path, manifest.MaxGalleryBytes)
	if err != nil {
		return GalleryManifestState{}, err
	}
	document, err := manifest.ParseGallery(bytesReader(data))
	if err != nil {
		return GalleryManifestState{}, err
	}
	if document.SetID != current.SetID {
		return GalleryManifestState{}, errors.New("Manifest set_id does not match Gallery")
	}
	state, baseline, _, found, err := loadGalleryManifestState(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	if err := assignFirstImportSubUUIDs(&document, baseline, found); err != nil {
		return GalleryManifestState{}, err
	}
	databaseSnapshot, err := buildGalleryBusinessSnapshot(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	if !found {
		baseline = databaseSnapshot
	}
	fileSnapshot, err := overlayGalleryDocument(baseline, document)
	if err != nil {
		return GalleryManifestState{}, err
	}
	mergedValue, conflicts := manifest.ThreeWayMerge(baseline, databaseSnapshot, fileSnapshot)
	if len(conflicts) > 0 {
		if resolve {
			mergedValue, conflicts, err = manifest.ResolveThreeWay(baseline, databaseSnapshot, fileSnapshot, choices)
			if err != nil {
				return GalleryManifestState{GalleryID: galleryID, Path: path, Status: ManifestConflict, Conflicts: conflicts}, err
			}
		} else {
			state = GalleryManifestState{
				GalleryID: galleryID, Path: path, Status: ManifestConflict,
				ManifestRevision: document.Revision, FileHash: fileHash, Conflicts: conflicts,
			}
			if found {
				if err := s.persistManifestInspection(ctx, galleryID, state.Status, now, conflicts); err != nil {
					return GalleryManifestState{}, err
				}
			}
			return state, nil
		}
	} else if resolve && len(choices) > 0 {
		return GalleryManifestState{}, errors.New("conflict choices were supplied but the Gallery no longer has conflicts")
	}
	merged, ok := mergedValue.(map[string]any)
	if !ok {
		return GalleryManifestState{}, errors.New("merged Gallery Manifest snapshot is not an object")
	}
	newMetadataRevision, err := s.applyGalleryBusinessSnapshot(
		ctx, galleryID, expectedMetadataRevision, merged, !found, portableImportID, now,
	)
	if err != nil {
		return GalleryManifestState{}, err
	}
	appliedSnapshot, err := buildGalleryBusinessSnapshot(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestState{}, err
	}
	baselineJSON, err := json.Marshal(appliedSnapshot)
	if err != nil {
		return GalleryManifestState{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_manifest_sync (
			gallery_id, manifest_path, status, schema_version, manifest_revision,
			file_hash, baseline_json, baseline_metadata_revision, checked_at_utc
		) VALUES (?, ?, 'CLEAN', ?, ?, ?, ?, ?, ?)
		ON CONFLICT(gallery_id) DO UPDATE SET manifest_path = excluded.manifest_path,
			status = 'CLEAN', schema_version = excluded.schema_version,
			manifest_revision = excluded.manifest_revision, file_hash = excluded.file_hash,
			baseline_json = excluded.baseline_json,
			baseline_metadata_revision = excluded.baseline_metadata_revision,
			checked_at_utc = excluded.checked_at_utc, last_error_code = ''
	`, galleryID, path, manifest.GallerySchemaVersion, document.Revision, fileHash,
		baselineJSON, newMetadataRevision, formatTime(normalisedTime(now))); err != nil {
		return GalleryManifestState{}, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM gallery_manifest_conflicts WHERE gallery_id = ?`, galleryID); err != nil {
		return GalleryManifestState{}, err
	}
	return GalleryManifestState{
		GalleryID: galleryID, Path: path, Status: ManifestClean,
		ManifestRevision: document.Revision, FileHash: fileHash,
	}, nil
}

func assignFirstImportSubUUIDs(document *manifest.GalleryDocument, baseline map[string]any, baselineExists bool) error {
	if document.ExternalLinks.Present && !document.ExternalLinks.Null {
		existing := objectValue(baseline["external_links"])
		for index := range document.ExternalLinks.Value {
			if document.ExternalLinks.Value[index].LinkUUID == "" {
				if baselineExists {
					for _, raw := range existing {
						if stringValue(objectValue(raw)["url"]) == document.ExternalLinks.Value[index].URL {
							return fmt.Errorf("external_links[%d] requires link_uuid when updating an existing URL", index)
						}
					}
				}
				document.ExternalLinks.Value[index].LinkUUID = portableid.New()
			}
		}
	}
	if baselineExists && document.Items.Present && !document.Items.Null {
		for index, item := range document.Items.Value {
			if item.ItemUUID == "" {
				return fmt.Errorf("items[%d] requires item_uuid after synchronization baseline exists", index)
			}
		}
	}
	return nil
}

func loadGalleryManifestState(ctx context.Context, queryer galleryQueryer, galleryID int64) (GalleryManifestState, map[string]any, int64, bool, error) {
	var state GalleryManifestState
	var status string
	var baselineJSON []byte
	var baselineRevision int64
	err := queryer.QueryRowContext(ctx, `
		SELECT gallery_id, manifest_path, status, manifest_revision, file_hash,
			baseline_json, baseline_metadata_revision
		FROM gallery_manifest_sync WHERE gallery_id = ?
	`, galleryID).Scan(&state.GalleryID, &state.Path, &status, &state.ManifestRevision,
		&state.FileHash, &baselineJSON, &baselineRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return GalleryManifestState{GalleryID: galleryID, Status: ManifestNone}, map[string]any{}, 0, false, nil
	}
	if err != nil {
		return GalleryManifestState{}, nil, 0, false, err
	}
	state.Status = ManifestStatus(status)
	var baseline map[string]any
	if err := json.Unmarshal(baselineJSON, &baseline); err != nil {
		return GalleryManifestState{}, nil, 0, false, err
	}
	return state, baseline, baselineRevision, true, nil
}

func (s *ManifestStore) persistManifestInspection(
	ctx context.Context,
	galleryID int64,
	status ManifestStatus,
	now time.Time,
	conflicts []manifest.Conflict,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE gallery_manifest_sync SET status = ?, checked_at_utc = ? WHERE gallery_id = ?
	`, status, formatTime(normalisedTime(now)), galleryID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_manifest_conflicts WHERE gallery_id = ?`, galleryID); err != nil {
		return err
	}
	for _, conflict := range conflicts {
		baseline, _ := json.Marshal(conflict.Baseline)
		database, _ := json.Marshal(conflict.Database)
		file, _ := json.Marshal(conflict.File)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_manifest_conflicts (
				gallery_id, field_path, baseline_json, database_json, file_json, created_at_utc
			) VALUES (?, ?, ?, ?, ?, ?)
		`, galleryID, conflict.Path, baseline, database, file, formatTime(normalisedTime(now))); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func buildGalleryBusinessSnapshot(ctx context.Context, queryer rowsQueryer, galleryID int64) (map[string]any, error) {
	galleryRecord, err := findGallery(ctx, queryer.(galleryQueryer), galleryID)
	if err != nil {
		return nil, err
	}
	snapshot := map[string]any{
		"title": galleryRecord.Title, "description": galleryRecord.Description,
		"content_rating":    nullableString(string(galleryRecord.ContentRating)),
		"photographer_name": galleryRecord.PhotographerName, "studio_name": galleryRecord.StudioName,
		"cover": nil, "extensions": map[string]any{},
	}
	rowQueryer := queryer.(galleryQueryer)
	var coverKind, coverItem, coverPath string
	var coverItemValue sql.NullString
	if err := rowQueryer.QueryRowContext(ctx, `SELECT preferred_kind,preferred_item_uuid,preferred_path FROM gallery_covers WHERE gallery_id=?`, galleryID).Scan(&coverKind, &coverItemValue, &coverPath); err == nil {
		coverItem = coverItemValue.String
		if coverKind != string(gallery.CoverNone) {
			snapshot["cover"] = map[string]any{"kind": coverKind, "item_uuid": coverItem, "path": coverPath}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if galleryRecord.ShootDate == "" {
		snapshot["shoot_date"] = nil
	} else {
		snapshot["shoot_date"] = map[string]any{"value": galleryRecord.ShootDate, "precision": galleryRecord.ShootDatePrecision}
	}
	var galleryRating sql.NullInt64
	if err := rowQueryer.QueryRowContext(ctx, `SELECT rating_half_steps FROM gallery_personal_states WHERE gallery_id = ?`, galleryID).Scan(&galleryRating); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if galleryRating.Valid {
		snapshot["rating"] = float64(galleryRating.Int64) / 2
	} else {
		snapshot["rating"] = nil
	}

	credits, err := queryMaps(ctx, queryer, `
		SELECT coser.uuid, coser.name, credit.position
		FROM gallery_credits credit JOIN cosers coser ON coser.uuid = credit.coser_uuid
		WHERE credit.gallery_id = ? ORDER BY credit.position
	`, galleryID, func(rows *sql.Rows) (map[string]any, error) {
		var uuid, name string
		var position int64
		if err := rows.Scan(&uuid, &name, &position); err != nil {
			return nil, err
		}
		return map[string]any{"coser": map[string]any{"uuid": uuid, "name_hint": name}, "position": position}, nil
	})
	if err != nil {
		return nil, err
	}
	snapshot["credits"] = keyedCollection(credits, func(value map[string]any) string { return value["coser"].(map[string]any)["uuid"].(string) })

	cast, err := queryMaps(ctx, queryer, `
		SELECT coser.uuid, coser.name, character.uuid, character.name, work.uuid, work.name, cast_item.position
		FROM gallery_cast cast_item
		JOIN gallery_credits credit ON credit.id = cast_item.gallery_credit_id
		JOIN cosers coser ON coser.uuid = credit.coser_uuid
		JOIN characters character ON character.uuid = cast_item.character_uuid
		JOIN works work ON work.uuid = character.work_uuid
		WHERE cast_item.gallery_id = ? ORDER BY credit.position, cast_item.position
	`, galleryID, func(rows *sql.Rows) (map[string]any, error) {
		var cu, cn, hu, hn, wu, wn string
		var position int64
		if err := rows.Scan(&cu, &cn, &hu, &hn, &wu, &wn, &position); err != nil {
			return nil, err
		}
		return map[string]any{
			"coser":     map[string]any{"uuid": cu, "name_hint": cn},
			"character": map[string]any{"uuid": hu, "name_hint": hn},
			"work":      map[string]any{"uuid": wu, "name_hint": wn}, "position": position,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	snapshot["cast"] = keyedCollection(cast, func(value map[string]any) string {
		return value["coser"].(map[string]any)["uuid"].(string) + "|" + value["character"].(map[string]any)["uuid"].(string)
	})

	tags, err := queryMaps(ctx, queryer, `
		SELECT tag.uuid, tag.name, relation.position FROM gallery_tags relation
		JOIN tags tag ON tag.uuid = relation.tag_uuid WHERE relation.gallery_id = ? ORDER BY relation.position
	`, galleryID, func(rows *sql.Rows) (map[string]any, error) {
		var uuid, name string
		var position int64
		if err := rows.Scan(&uuid, &name, &position); err != nil {
			return nil, err
		}
		return map[string]any{"uuid": uuid, "name_hint": name, "position": position}, nil
	})
	if err != nil {
		return nil, err
	}
	snapshot["tags"] = keyedCollection(tags, func(value map[string]any) string { return value["uuid"].(string) })

	links, err := queryMaps(ctx, queryer, `
		SELECT link_uuid, link_type, label, url, position FROM gallery_external_links
		WHERE gallery_id = ? ORDER BY position
	`, galleryID, func(rows *sql.Rows) (map[string]any, error) {
		var uuid, kind, label, url string
		var position int64
		if err := rows.Scan(&uuid, &kind, &label, &url, &position); err != nil {
			return nil, err
		}
		return map[string]any{"link_uuid": uuid, "type": kind, "label": label, "url": url, "position": position}, nil
	})
	if err != nil {
		return nil, err
	}
	snapshot["external_links"] = keyedCollection(links, func(value map[string]any) string { return value["link_uuid"].(string) })

	items, err := queryMaps(ctx, queryer, `
		SELECT item.item_uuid, item.relative_path, item.image_category, item.caption, item.position,
			state.rating_half_steps, item.excluded
		FROM gallery_items item LEFT JOIN gallery_item_personal_states state ON state.gallery_item_id = item.id
		WHERE item.gallery_id = ? ORDER BY item.position
	`, galleryID, func(rows *sql.Rows) (map[string]any, error) {
		var uuid, path, caption string
		var category sql.NullString
		var rating sql.NullInt64
		var position int64
		var excluded int
		if err := rows.Scan(&uuid, &path, &category, &caption, &position, &rating, &excluded); err != nil {
			return nil, err
		}
		value := map[string]any{"item_uuid": uuid, "path": path, "category": nullableString(category.String), "caption": caption, "position": position, "rating": nil, "excluded": excluded == 1}
		if rating.Valid {
			value["rating"] = float64(rating.Int64) / 2
		}
		return value, nil
	})
	if err != nil {
		return nil, err
	}
	snapshot["items"] = keyedCollection(items, func(value map[string]any) string { return value["item_uuid"].(string) })
	excluded := make(map[string]any)
	for key, raw := range snapshot["items"].(map[string]any) {
		item := raw.(map[string]any)
		if item["excluded"].(bool) {
			excluded[key] = map[string]any{"item_uuid": key, "path": item["path"]}
		}
		delete(item, "excluded")
	}
	snapshot["excluded_items"] = excluded
	return cloneMap(snapshot), nil
}

func queryMaps(ctx context.Context, queryer rowsQueryer, query string, argument any, scan func(*sql.Rows) (map[string]any, error)) ([]map[string]any, error) {
	rows, err := queryer.QueryContext(ctx, query, argument)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		value, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func keyedCollection(values []map[string]any, key func(map[string]any) string) map[string]any {
	result := make(map[string]any, len(values))
	for _, value := range values {
		result[key(value)] = value
	}
	return result
}

func overlayGalleryDocument(baseline map[string]any, document manifest.GalleryDocument) (map[string]any, error) {
	result := cloneMap(baseline)
	setOptional := func(key string, present bool, null bool, value any) {
		if present {
			if null {
				result[key] = nil
			} else {
				result[key] = value
			}
		}
	}
	setOptional("title", document.Title.Present, document.Title.Null, document.Title.Value)
	setOptional("description", document.Description.Present, document.Description.Null, document.Description.Value)
	setOptional("shoot_date", document.ShootDate.Present, document.ShootDate.Null, map[string]any{"value": document.ShootDate.Value.Value, "precision": document.ShootDate.Value.Precision})
	setOptional("content_rating", document.ContentRating.Present, document.ContentRating.Null, document.ContentRating.Value)
	setOptional("rating", document.Rating.Present, document.Rating.Null, document.Rating.Value)
	setOptional("photographer_name", document.PhotographerName.Present, document.PhotographerName.Null, document.PhotographerName.Value)
	setOptional("studio_name", document.StudioName.Present, document.StudioName.Null, document.StudioName.Value)
	setOptional("cover", document.Cover.Present, document.Cover.Null, map[string]any{
		"kind": document.Cover.Value.Kind, "item_uuid": document.Cover.Value.ItemUUID, "path": document.Cover.Value.Path,
	})
	if document.Credits.Present {
		result["credits"] = keyedCredits(document.Credits)
	}
	if document.Cast.Present {
		result["cast"] = keyedCast(document.Cast)
	}
	if document.Tags.Present {
		result["tags"] = keyedReferences(document.Tags)
	}
	if document.ExternalLinks.Present {
		result["external_links"] = keyedLinks(document.ExternalLinks)
	}
	if document.Items.Present {
		result["items"] = keyedItems(document.Items)
	}
	if document.ExcludedItems.Present {
		result["excluded_items"] = keyedExclusions(document.ExcludedItems)
	}
	return cloneMap(result), nil
}

func keyedCredits(value manifest.Optional[[]manifest.Credit]) map[string]any {
	result := map[string]any{}
	if value.Null {
		return result
	}
	for _, item := range value.Value {
		result[item.Coser.UUID] = map[string]any{"coser": referenceMap(item.Coser), "position": item.Position}
	}
	return result
}
func keyedCast(value manifest.Optional[[]manifest.Cast]) map[string]any {
	result := map[string]any{}
	if value.Null {
		return result
	}
	for _, item := range value.Value {
		result[item.Coser.UUID+"|"+item.Character.UUID] = map[string]any{"coser": referenceMap(item.Coser), "character": referenceMap(item.Character), "work": referenceMap(item.Work), "position": item.Position}
	}
	return result
}
func keyedReferences(value manifest.Optional[[]manifest.EntityReference]) map[string]any {
	result := map[string]any{}
	if value.Null {
		return result
	}
	for index, item := range value.Value {
		result[item.UUID] = map[string]any{"uuid": item.UUID, "name_hint": item.NameHint, "position": int64(index+1) * 1024}
	}
	return result
}
func keyedLinks(value manifest.Optional[[]manifest.ExternalLink]) map[string]any {
	result := map[string]any{}
	if value.Null {
		return result
	}
	for _, item := range value.Value {
		result[item.LinkUUID] = map[string]any{"link_uuid": item.LinkUUID, "type": item.Type, "label": item.Label, "url": item.URL, "position": item.Position}
	}
	return result
}
func keyedItems(value manifest.Optional[[]manifest.Item]) map[string]any {
	result := map[string]any{}
	if value.Null {
		return result
	}
	for _, item := range value.Value {
		key := item.ItemUUID
		if key == "" {
			key = "path:" + item.Path
		}
		var rating any
		if item.Rating != nil {
			rating = *item.Rating
		}
		result[key] = map[string]any{"item_uuid": item.ItemUUID, "path": item.Path, "category": nullableString(item.Category), "caption": item.Caption, "position": item.Position, "rating": rating}
	}
	return result
}
func keyedExclusions(value manifest.Optional[[]manifest.Exclusion]) map[string]any {
	result := map[string]any{}
	if value.Null {
		return result
	}
	for _, item := range value.Value {
		key := item.ItemUUID
		if key == "" {
			key = item.Fingerprint
		}
		if key == "" {
			key = "path:" + item.Path
		}
		result[key] = map[string]any{"item_uuid": item.ItemUUID, "path": item.Path, "fingerprint": item.Fingerprint}
	}
	return result
}
func referenceMap(value manifest.EntityReference) map[string]any {
	return map[string]any{"uuid": value.UUID, "name_hint": value.NameHint}
}

func cloneMap(value map[string]any) map[string]any {
	data, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func bytesReader(data []byte) *bytes.Reader { return bytes.NewReader(data) }
func mustGallerySetID(ctx context.Context, queryer galleryQueryer, galleryID int64) string {
	record, _ := findGallery(ctx, queryer, galleryID)
	return record.SetID
}

// businessDocument converts keyed collections back to deterministic arrays for Push.
func businessDocument(value map[string]any) map[string]any {
	result := cloneMap(value)
	for _, key := range []string{"credits", "cast", "tags", "external_links", "items", "excluded_items"} {
		collection, ok := result[key].(map[string]any)
		if !ok {
			continue
		}
		keys := make([]string, 0, len(collection))
		for itemKey := range collection {
			keys = append(keys, itemKey)
		}
		sort.SliceStable(keys, func(left int, right int) bool {
			leftPosition := collectionPosition(collection[keys[left]])
			rightPosition := collectionPosition(collection[keys[right]])
			if leftPosition != rightPosition {
				return leftPosition < rightPosition
			}
			return keys[left] < keys[right]
		})
		items := make([]any, 0, len(keys))
		for _, itemKey := range keys {
			item := collection[itemKey]
			if key == "tags" {
				if tag, ok := item.(map[string]any); ok {
					tag = cloneMap(tag)
					delete(tag, "position")
					item = tag
				}
			}
			items = append(items, item)
		}
		result[key] = items
	}
	return result
}

func collectionPosition(value any) float64 {
	if object, ok := value.(map[string]any); ok {
		if position, ok := object["position"].(float64); ok {
			return position
		}
	}
	return 0
}
