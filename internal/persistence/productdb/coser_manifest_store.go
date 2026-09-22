package productdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/coreentity"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/textsafe"
)

type CoserManifestState struct {
	CoserUUID        string
	Path             string
	Status           ManifestStatus
	ManifestRevision int64
	FileHash         string
	Conflicts        []manifest.Conflict
}

func (s *ManifestStore) CheckCoser(ctx context.Context, metadataRoot string, coserUUID string, now time.Time) (CoserManifestState, error) {
	path, err := manifest.CoserPath(metadataRoot, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	state, baseline, baselineRevision, found, err := loadCoserManifestState(ctx, s.db, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	storedHash := state.FileHash
	state.Path = path
	data, hash, readErr := manifest.ReadFile(path, manifest.MaxCoserBytes)
	if errors.Is(readErr, os.ErrNotExist) {
		if !found {
			state.Status = ManifestNone
			return state, nil
		}
		state.Status = ManifestMissing
		_ = persistCoserStatus(ctx, s.db, coserUUID, state.Status, now)
		return state, nil
	}
	if readErr != nil {
		if found {
			_ = persistCoserStatus(ctx, s.db, coserUUID, ManifestError, now)
		}
		return state, readErr
	}
	document, err := manifest.ParseCoser(bytesReader(data))
	if err != nil || document.CoserUUID != coserUUID {
		if found {
			_ = persistCoserStatus(ctx, s.db, coserUUID, ManifestError, now)
		}
		if err != nil {
			return state, err
		}
		return state, errors.New("Coser Manifest UUID does not match target")
	}
	state.FileHash = hash
	if !found {
		state.Status = ManifestFileDirty
		return state, nil
	}
	coser, err := findCoser(ctx, s.db, coserUUID)
	if err != nil {
		return state, err
	}
	databaseChanged := coser.MetadataRevision != baselineRevision
	fileChanged := hash != storedHash
	switch {
	case !databaseChanged && !fileChanged:
		state.Status = ManifestClean
	case databaseChanged && !fileChanged:
		state.Status = ManifestDBDirty
	case !databaseChanged && fileChanged:
		state.Status = ManifestFileDirty
	default:
		databaseSnapshot, err := buildCoserBusinessSnapshot(ctx, s.db, coserUUID)
		if err != nil {
			return state, err
		}
		fileSnapshot := overlayCoserDocument(baseline, document)
		_, state.Conflicts = manifest.ThreeWayMerge(baseline, databaseSnapshot, fileSnapshot)
		if len(state.Conflicts) > 0 {
			state.Status = ManifestConflict
			_ = persistCoserConflicts(ctx, s.db, coserUUID, state.Conflicts, now)
			return state, nil
		}
		state.Status = ManifestDBDirty
	}
	if err := persistCoserStatus(ctx, s.db, coserUUID, state.Status, now); err != nil {
		return state, err
	}
	return state, nil
}

func (s *ManifestStore) PushCoser(
	ctx context.Context,
	metadataRoot string,
	coserUUID string,
	expectedMetadataRevision int64,
	now time.Time,
) (CoserManifestState, error) {
	coser, err := findCoser(ctx, s.db, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	if coser.MetadataRevision != expectedMetadataRevision {
		return CoserManifestState{}, ErrCoreMetadataRevisionConflict
	}
	path, err := manifest.EnsureCoserDirectory(metadataRoot, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	state, _, _, found, err := loadCoserManifestState(ctx, s.db, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	if _, hash, readErr := manifest.ReadFile(path, manifest.MaxCoserBytes); readErr == nil {
		if !found {
			return CoserManifestState{}, ErrUntrackedManifest
		}
		if hash != state.FileHash {
			return CoserManifestState{}, ErrManifestFileDirty
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return CoserManifestState{}, readErr
	}
	business, err := buildCoserBusinessSnapshot(ctx, s.db, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	revision := state.ManifestRevision + 1
	document := coserBusinessDocument(business)
	document["schema_version"] = manifest.GallerySchemaVersion
	document["revision"] = revision
	document["coser_uuid"] = coserUUID
	document["updated_at"] = normalisedTime(now).Format(time.RFC3339)
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return CoserManifestState{}, err
	}
	data = append(data, '\n')
	fileHash, err := manifest.WriteAtomic(path, data, manifest.MaxCoserBytes)
	if err != nil {
		return CoserManifestState{}, err
	}
	baseline, _ := json.Marshal(business)
	if err := upsertCoserManifestState(ctx, s.db, coserUUID, path, ManifestClean,
		revision, fileHash, baseline, coser.MetadataRevision, now); err != nil {
		return CoserManifestState{}, err
	}
	return CoserManifestState{
		CoserUUID: coserUUID, Path: path, Status: ManifestClean,
		ManifestRevision: revision, FileHash: fileHash,
	}, nil
}

func (s *ManifestStore) PullCoser(
	ctx context.Context,
	metadataRoot string,
	coserUUID string,
	expectedMetadataRevision int64,
	now time.Time,
) (CoserManifestState, error) {
	return s.pullCoser(ctx, metadataRoot, coserUUID, expectedMetadataRevision, nil, false, now)
}

func (s *ManifestStore) ResolveCoserConflicts(
	ctx context.Context,
	metadataRoot string,
	coserUUID string,
	expectedMetadataRevision int64,
	choices map[string]manifest.ConflictChoice,
	now time.Time,
) (CoserManifestState, error) {
	return s.pullCoser(ctx, metadataRoot, coserUUID, expectedMetadataRevision, choices, true, now)
}

func (s *ManifestStore) pullCoser(
	ctx context.Context,
	metadataRoot string,
	coserUUID string,
	expectedMetadataRevision int64,
	choices map[string]manifest.ConflictChoice,
	resolve bool,
	now time.Time,
) (CoserManifestState, error) {
	coser, err := findCoser(ctx, s.db, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	if coser.MetadataRevision != expectedMetadataRevision {
		return CoserManifestState{}, ErrCoreMetadataRevisionConflict
	}
	path, err := manifest.CoserPath(metadataRoot, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	data, fileHash, err := manifest.ReadFile(path, manifest.MaxCoserBytes)
	if err != nil {
		return CoserManifestState{}, err
	}
	document, err := manifest.ParseCoser(bytesReader(data))
	if err != nil {
		return CoserManifestState{}, err
	}
	if document.CoserUUID != coserUUID {
		return CoserManifestState{}, errors.New("Coser Manifest UUID does not match target directory")
	}
	state, baseline, _, found, err := loadCoserManifestState(ctx, s.db, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	if err := assignCoserAccountUUIDs(&document, baseline, found); err != nil {
		return CoserManifestState{}, err
	}
	databaseSnapshot, err := buildCoserBusinessSnapshot(ctx, s.db, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	if !found {
		baseline = databaseSnapshot
	}
	fileSnapshot := overlayCoserDocument(baseline, document)
	mergedValue, conflicts := manifest.ThreeWayMerge(baseline, databaseSnapshot, fileSnapshot)
	if len(conflicts) > 0 {
		if resolve {
			mergedValue, conflicts, err = manifest.ResolveThreeWay(baseline, databaseSnapshot, fileSnapshot, choices)
			if err != nil {
				return CoserManifestState{CoserUUID: coserUUID, Path: path, Status: ManifestConflict, Conflicts: conflicts}, err
			}
		} else {
			state = CoserManifestState{
				CoserUUID: coserUUID, Path: path, Status: ManifestConflict,
				ManifestRevision: document.Revision, FileHash: fileHash, Conflicts: conflicts,
			}
			if found {
				_ = persistCoserConflicts(ctx, s.db, coserUUID, conflicts, now)
			}
			return state, nil
		}
	} else if resolve && len(choices) > 0 {
		return CoserManifestState{}, errors.New("conflict choices were supplied but the Coser no longer has conflicts")
	}
	merged := mergedValue.(map[string]any)
	newRevision, err := applyCoserSnapshot(ctx, s.db, coserUUID, expectedMetadataRevision, merged, now)
	if err != nil {
		return CoserManifestState{}, err
	}
	applied, err := buildCoserBusinessSnapshot(ctx, s.db, coserUUID)
	if err != nil {
		return CoserManifestState{}, err
	}
	baselineJSON, _ := json.Marshal(applied)
	if err := upsertCoserManifestState(ctx, s.db, coserUUID, path, ManifestClean,
		document.Revision, fileHash, baselineJSON, newRevision, now); err != nil {
		return CoserManifestState{}, err
	}
	return CoserManifestState{
		CoserUUID: coserUUID, Path: path, Status: ManifestClean,
		ManifestRevision: document.Revision, FileHash: fileHash,
	}, nil
}

// ImportStandaloneCoser creates a Manage-only Coser from a valid independent
// Manifest, then establishes its synchronization baseline.
func (s *ManifestStore) ImportStandaloneCoser(
	ctx context.Context,
	metadataRoot string,
	coserUUID string,
	now time.Time,
) (coreentity.Coser, error) {
	path, err := manifest.CoserPath(metadataRoot, coserUUID)
	if err != nil {
		return coreentity.Coser{}, err
	}
	data, _, err := manifest.ReadFile(path, manifest.MaxCoserBytes)
	if err != nil {
		return coreentity.Coser{}, err
	}
	document, err := manifest.ParseCoser(bytesReader(data))
	if err != nil {
		return coreentity.Coser{}, err
	}
	if document.CoserUUID != coserUUID || !document.Name.Present || document.Name.Null || normalizedDisplay(document.Name.Value) == "" {
		return coreentity.Coser{}, errors.New("standalone Coser Manifest requires matching UUID and name")
	}
	created, err := (&CoreEntityStore{db: s.db}).CreateCoser(ctx, CreateCoserInput{
		CreateNamedEntityInput: CreateNamedEntityInput{
			UUID: coserUUID, Name: document.Name.Value,
			SortName: optionalString(document.SortName), Aliases: optionalStrings(document.Aliases),
		},
		ProfileSummary: optionalString(document.ProfileSummary), Biography: optionalString(document.Biography),
		CountryOrRegion: optionalString(document.CountryOrRegion),
	}, now)
	if err != nil {
		return coreentity.Coser{}, err
	}
	if _, err := s.PullCoser(ctx, metadataRoot, coserUUID, created.MetadataRevision, now); err != nil {
		return coreentity.Coser{}, err
	}
	return findCoser(ctx, s.db, coserUUID)
}

// WriteCoserMergeRedirect is retry-safe filesystem completion for a committed
// Coser merge. It writes only after the database proves the permanent UUID
// Alias points at the requested active target.
func (s *ManifestStore) WriteCoserMergeRedirect(ctx context.Context, metadataRoot, sourceUUID, targetUUID string) (string, error) {
	source, err := lookupPortableUUID(ctx, s.db, sourceUUID)
	if err != nil {
		return "", err
	}
	target, err := lookupPortableUUID(ctx, s.db, targetUUID)
	if err != nil {
		return "", err
	}
	if source.Kind != portableid.KindCoser || source.State != PortableUUIDAlias || source.TargetUUID != targetUUID ||
		target.Kind != portableid.KindCoser || target.State != PortableUUIDActive {
		return "", errors.New("database does not contain the requested committed Coser merge")
	}
	return manifest.WriteCoserRedirect(metadataRoot, sourceUUID, targetUUID)
}

func buildCoserBusinessSnapshot(ctx context.Context, queryer *sql.DB, uuid string) (map[string]any, error) {
	coser, err := findCoser(ctx, queryer, uuid)
	if err != nil {
		return nil, err
	}
	aliases := make([]any, len(coser.Aliases))
	for index := range coser.Aliases {
		aliases[index] = coser.Aliases[index]
	}
	result := map[string]any{
		"name": coser.Name, "sort_name": coser.SortName, "aliases": aliases,
		"profile_summary": coser.ProfileSummary, "biography": coser.Biography,
		"country_or_region": coser.CountryOrRegion,
		"avatar":            nullableString(coser.AvatarPath), "banner": nullableString(coser.BannerPath),
		"avatar_crop": nil, "banner_focal_point": nil, "extensions": map[string]any{},
	}
	if coser.AvatarCrop != nil {
		result["avatar_crop"] = map[string]any{"x": coser.AvatarCrop.X, "y": coser.AvatarCrop.Y, "size": coser.AvatarCrop.Size}
	}
	if coser.BannerFocalPoint != nil {
		result["banner_focal_point"] = map[string]any{"x": coser.BannerFocalPoint.X, "y": coser.BannerFocalPoint.Y}
	}
	rows, err := queryer.QueryContext(ctx, `
		SELECT account_uuid, platform_key, label, handle, url, status, visible, position
		FROM coser_social_accounts WHERE coser_uuid = ? ORDER BY position
	`, uuid)
	if err != nil {
		return nil, err
	}
	accounts := map[string]any{}
	for rows.Next() {
		var accountUUID, platform, label, handle, url, status string
		var visible int
		var position int64
		if err := rows.Scan(&accountUUID, &platform, &label, &handle, &url, &status, &visible, &position); err != nil {
			rows.Close()
			return nil, err
		}
		accounts[accountUUID] = map[string]any{
			"account_uuid": accountUUID, "platform_key": platform, "label": label,
			"handle": handle, "url": url, "status": status,
			"visible": visible == 1, "position": position,
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	result["social_accounts"] = accounts
	return cloneMap(result), nil
}

func coserBusinessDocument(business map[string]any) map[string]any {
	result := cloneMap(business)
	collection := objectValue(result["social_accounts"])
	keys := sortedObjectKeys(collection)
	sort.SliceStable(keys, func(i, j int) bool {
		left, right := collectionPosition(collection[keys[i]]), collectionPosition(collection[keys[j]])
		if left != right {
			return left < right
		}
		return keys[i] < keys[j]
	})
	items := make([]any, 0, len(keys))
	for _, key := range keys {
		items = append(items, collection[key])
	}
	result["social_accounts"] = items
	return result
}

func overlayCoserDocument(baseline map[string]any, document manifest.CoserDocument) map[string]any {
	result := cloneMap(baseline)
	set := func(key string, present bool, null bool, value any) {
		if present {
			if null {
				result[key] = nil
			} else {
				result[key] = value
			}
		}
	}
	set("name", document.Name.Present, document.Name.Null, document.Name.Value)
	set("sort_name", document.SortName.Present, document.SortName.Null, document.SortName.Value)
	set("aliases", document.Aliases.Present, document.Aliases.Null, document.Aliases.Value)
	set("profile_summary", document.ProfileSummary.Present, document.ProfileSummary.Null, document.ProfileSummary.Value)
	set("biography", document.Biography.Present, document.Biography.Null, document.Biography.Value)
	set("country_or_region", document.CountryOrRegion.Present, document.CountryOrRegion.Null, document.CountryOrRegion.Value)
	oldAvatar := result["avatar"]
	set("avatar", document.Avatar.Present, document.Avatar.Null, document.Avatar.Value)
	set("banner", document.Banner.Present, document.Banner.Null, document.Banner.Value)
	set("avatar_crop", document.AvatarCrop.Present, document.AvatarCrop.Null,
		map[string]any{"x": document.AvatarCrop.Value.X, "y": document.AvatarCrop.Value.Y, "size": document.AvatarCrop.Value.Size})
	if document.Avatar.Present && !document.AvatarCrop.Present && result["avatar"] != oldAvatar {
		result["avatar_crop"] = nil
	}
	set("banner_focal_point", document.BannerFocalPoint.Present, document.BannerFocalPoint.Null,
		map[string]any{"x": document.BannerFocalPoint.Value.X, "y": document.BannerFocalPoint.Value.Y})
	if document.SocialAccounts.Present {
		accounts := map[string]any{}
		if !document.SocialAccounts.Null {
			for _, account := range document.SocialAccounts.Value {
				accounts[account.AccountUUID] = map[string]any{
					"account_uuid": account.AccountUUID, "platform_key": account.PlatformKey,
					"label": account.Label, "handle": account.Handle, "url": account.URL,
					"status": account.Status, "visible": account.Visible, "position": account.Position,
				}
			}
		}
		result["social_accounts"] = accounts
	}
	return cloneMap(result)
}

func assignCoserAccountUUIDs(document *manifest.CoserDocument, baseline map[string]any, baselineExists bool) error {
	if !document.SocialAccounts.Present || document.SocialAccounts.Null {
		return nil
	}
	existing := objectValue(baseline["social_accounts"])
	for index := range document.SocialAccounts.Value {
		account := &document.SocialAccounts.Value[index]
		if account.AccountUUID != "" {
			continue
		}
		if baselineExists {
			for _, raw := range existing {
				if stringValue(objectValue(raw)["url"]) == account.URL {
					return fmt.Errorf("social_accounts[%d] requires account_uuid when updating an existing URL", index)
				}
			}
		}
		account.AccountUUID = portableid.New()
	}
	return nil
}

func applyCoserSnapshot(ctx context.Context, db *sql.DB, uuid string, expectedRevision int64, snapshot map[string]any, now time.Time) (int64, error) {
	aliases := stringSlice(snapshot["aliases"])
	input := UpdateCoserInput{
		Name: stringValue(snapshot["name"]), SortName: stringValue(snapshot["sort_name"]), Aliases: aliases,
		ProfileSummary: stringValue(snapshot["profile_summary"]), Biography: stringValue(snapshot["biography"]),
		CountryOrRegion: stringValue(snapshot["country_or_region"]),
		AvatarPath:      stringValue(snapshot["avatar"]), BannerPath: stringValue(snapshot["banner"]),
	}
	if crop := objectValue(snapshot["avatar_crop"]); len(crop) > 0 {
		input.AvatarCrop = &coreentity.AvatarCrop{X: floatValue(crop["x"]), Y: floatValue(crop["y"]), Size: floatValue(crop["size"])}
	}
	if focal := objectValue(snapshot["banner_focal_point"]); len(focal) > 0 {
		input.BannerFocalPoint = &coreentity.FocalPoint{X: floatValue(focal["x"]), Y: floatValue(focal["y"])}
	}
	if err := validateNamedInput(CreateNamedEntityInput{Name: input.Name, SortName: input.SortName, Aliases: aliases}); err != nil {
		return 0, err
	}
	if runeLength(input.ProfileSummary) > 20000 || runeLength(input.Biography) > 20000 || runeLength(input.CountryOrRegion) > 100 {
		return 0, errors.New("Coser Manifest profile field exceeds its length limit")
	}
	if err := textsafe.ValidateMarkdown(input.Biography); err != nil {
		return 0, err
	}
	for _, asset := range []string{input.AvatarPath, input.BannerPath} {
		if asset != "" {
			if err := manifest.ValidateManagedRelativeAsset(asset); err != nil {
				return 0, err
			}
		}
	}
	if err := validateCoserCrop(input.AvatarCrop, input.BannerFocalPoint); err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var cropX, cropY, cropSize, focalX, focalY any
	if input.AvatarCrop != nil {
		cropX, cropY, cropSize = input.AvatarCrop.X, input.AvatarCrop.Y, input.AvatarCrop.Size
	}
	if input.BannerFocalPoint != nil {
		focalX, focalY = input.BannerFocalPoint.X, input.BannerFocalPoint.Y
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE cosers SET name = ?, sort_name = ?, profile_summary = ?, biography = ?,
			country_or_region = ?, avatar_path = ?, banner_path = ?, avatar_crop_x = ?,
			avatar_crop_y = ?, avatar_crop_size = ?, banner_focal_x = ?, banner_focal_y = ?,
			metadata_revision = metadata_revision + 1, updated_at_utc = ?
		WHERE uuid = ? AND metadata_revision = ?
	`, normalizedDisplay(input.Name), normalizedDisplay(input.SortName), input.ProfileSummary,
		input.Biography, normalizedDisplay(input.CountryOrRegion), input.AvatarPath, input.BannerPath,
		cropX, cropY, cropSize, focalX, focalY, formatTime(normalisedTime(now)), uuid, expectedRevision)
	if err != nil {
		return 0, err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return 0, ErrCoreMetadataRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM coser_aliases WHERE coser_uuid = ?`, uuid); err != nil {
		return 0, err
	}
	if err := insertAliases(ctx, tx, "coser_aliases", "coser_uuid", uuid, aliases); err != nil {
		return 0, err
	}
	if err := applyCoserAccounts(ctx, tx, uuid, objectValue(snapshot["social_accounts"]), now); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return expectedRevision + 1, nil
}

func applyCoserAccounts(ctx context.Context, tx *sql.Tx, coserUUID string, accounts map[string]any, now time.Time) error {
	if len(accounts) > 50 {
		return errors.New("Coser Manifest SocialAccounts exceeds 50")
	}
	rows, err := tx.QueryContext(ctx, `SELECT account_uuid FROM coser_social_accounts WHERE coser_uuid = ?`, coserUUID)
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM coser_social_accounts WHERE coser_uuid = ?`, coserUUID); err != nil {
		return err
	}
	desired := map[string]struct{}{}
	for _, key := range sortedObjectKeys(accounts) {
		account := objectValue(accounts[key])
		uuid := stringValue(account["account_uuid"])
		desired[uuid] = struct{}{}
		record, err := lookupPortableUUID(ctx, tx, uuid)
		if errors.Is(err, ErrPortableUUIDNotFound) {
			if _, err := registerPortableUUID(ctx, tx, uuid, portableid.KindSocialAccount, normalisedTime(now)); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if record.Kind != portableid.KindSocialAccount || record.State != PortableUUIDActive {
			return errors.New("SocialAccount UUID is occupied")
		}
		platform := stringValue(account["platform_key"])
		if !platformKeyPattern.MatchString(platform) {
			return errors.New("invalid SocialAccount platform_key")
		}
		url, err := validateExternalHTTPURL(stringValue(account["url"]))
		if err != nil {
			return err
		}
		status := stringValue(account["status"])
		if status != "ACTIVE" && status != "INACTIVE" {
			return errors.New("invalid SocialAccount status")
		}
		position, err := positivePosition(account["position"], "SocialAccount")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO coser_social_accounts (account_uuid, coser_uuid, platform_key, label, handle, url, status, visible, position)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, uuid, coserUUID, platform, normalizedDisplay(stringValue(account["label"])), normalizedDisplay(stringValue(account["handle"])), url, status, boolValue(account["visible"]), position); err != nil {
			return err
		}
	}
	for _, uuid := range previous {
		if _, ok := desired[uuid]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO portable_uuid_tombstones (uuid, entity_kind, deleted_at_utc, reason)
			VALUES (?, 'SOCIAL_ACCOUNT', ?, 'SocialAccount removed by Coser Manifest Pull')`, uuid, formatTime(normalisedTime(now))); err != nil {
			return err
		}
	}
	return nil
}

func loadCoserManifestState(ctx context.Context, queryer galleryQueryer, uuid string) (CoserManifestState, map[string]any, int64, bool, error) {
	var state CoserManifestState
	var status string
	var baseline []byte
	var baselineRevision int64
	err := queryer.QueryRowContext(ctx, `SELECT coser_uuid, manifest_path, status, manifest_revision, file_hash,
		baseline_json, baseline_metadata_revision FROM coser_manifest_sync WHERE coser_uuid = ?`, uuid).Scan(
		&state.CoserUUID, &state.Path, &status, &state.ManifestRevision, &state.FileHash, &baseline, &baselineRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return CoserManifestState{CoserUUID: uuid, Status: ManifestNone}, map[string]any{}, 0, false, nil
	}
	if err != nil {
		return CoserManifestState{}, nil, 0, false, err
	}
	state.Status = ManifestStatus(status)
	var decoded map[string]any
	if err := json.Unmarshal(baseline, &decoded); err != nil {
		return CoserManifestState{}, nil, 0, false, err
	}
	return state, decoded, baselineRevision, true, nil
}

func upsertCoserManifestState(ctx context.Context, db *sql.DB, uuid string, path string, status ManifestStatus, revision int64, hash string, baseline []byte, metadataRevision int64, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO coser_manifest_sync (coser_uuid, manifest_path, status, schema_version,
		manifest_revision, file_hash, baseline_json, baseline_metadata_revision, checked_at_utc)
		VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?)
		ON CONFLICT(coser_uuid) DO UPDATE SET manifest_path=excluded.manifest_path, status=excluded.status,
		manifest_revision=excluded.manifest_revision, file_hash=excluded.file_hash, baseline_json=excluded.baseline_json,
		baseline_metadata_revision=excluded.baseline_metadata_revision, checked_at_utc=excluded.checked_at_utc, last_error_code=''`,
		uuid, path, status, revision, hash, baseline, metadataRevision, formatTime(normalisedTime(now)))
	return err
}

func persistCoserConflicts(ctx context.Context, db *sql.DB, uuid string, conflicts []manifest.Conflict, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE coser_manifest_sync SET status='CONFLICT', checked_at_utc=? WHERE coser_uuid=?`, formatTime(normalisedTime(now)), uuid); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM coser_manifest_conflicts WHERE coser_uuid=?`, uuid); err != nil {
		return err
	}
	for _, conflict := range conflicts {
		base, _ := json.Marshal(conflict.Baseline)
		database, _ := json.Marshal(conflict.Database)
		file, _ := json.Marshal(conflict.File)
		if _, err := tx.ExecContext(ctx, `INSERT INTO coser_manifest_conflicts (coser_uuid,field_path,baseline_json,database_json,file_json,created_at_utc) VALUES(?,?,?,?,?,?)`, uuid, conflict.Path, base, database, file, formatTime(normalisedTime(now))); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func persistCoserStatus(ctx context.Context, db *sql.DB, uuid string, status ManifestStatus, now time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE coser_manifest_sync SET status=?, checked_at_utc=? WHERE coser_uuid=?`,
		status, formatTime(normalisedTime(now)), uuid)
	return err
}

func optionalString(value manifest.Optional[string]) string {
	if !value.Present || value.Null {
		return ""
	}
	return value.Value
}
func optionalStrings(value manifest.Optional[[]string]) []string {
	if !value.Present || value.Null {
		return nil
	}
	return value.Value
}
func stringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			return strings
		}
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		result = append(result, stringValue(item))
	}
	return result
}
func floatValue(value any) float64 {
	if result, ok := value.(float64); ok {
		return result
	}
	return 0
}
func boolValue(value any) bool { result, _ := value.(bool); return result }
