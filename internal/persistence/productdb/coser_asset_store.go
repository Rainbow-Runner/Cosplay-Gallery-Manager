package productdb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/coreentity"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portableid"
)

type CoserAssetKind string

const (
	CoserAssetAvatar CoserAssetKind = "AVATAR"
	CoserAssetBanner CoserAssetKind = "BANNER"
)

type CoserManagedAssetInput struct {
	Kind             CoserAssetKind
	RelativePath     string
	AvatarCrop       *coreentity.AvatarCrop
	BannerFocalPoint *coreentity.FocalPoint
}

type CoserMetadataImportInput struct {
	Avatar   *CoserManagedAssetInput
	Banner   *CoserManagedAssetInput
	Accounts []SocialAccountInput
}

type CoserManagedAssetOwnerState string

const (
	CoserManagedAssetOwnerActive  CoserManagedAssetOwnerState = "ACTIVE"
	CoserManagedAssetOwnerMerged  CoserManagedAssetOwnerState = "MERGED"
	CoserManagedAssetOwnerDeleted CoserManagedAssetOwnerState = "DELETED"
)

type CoserManagedAssetOwner struct {
	UUID       string
	State      CoserManagedAssetOwnerState
	AvatarPath string
	BannerPath string
}

// CoserManagedAssetOwners returns every permanently registered Coser UUID and
// the only database paths that may currently retain managed asset groups.
func (s *CoreEntityStore) CoserManagedAssetOwners(ctx context.Context) ([]CoserManagedAssetOwner, error) {
	return coserManagedAssetOwners(ctx, s.db)
}

// WithCoserManagedAssetOwners holds the database's immediate write
// transaction while action runs. Cleanup uses this bounded critical section so
// a Manifest pull or upload cannot publish a path between the final reference
// check and removal of an application-generated file.
func (s *CoreEntityStore) WithCoserManagedAssetOwners(ctx context.Context, action func([]CoserManagedAssetOwner) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	owners, err := coserManagedAssetOwners(ctx, tx)
	if err != nil {
		return err
	}
	if err := action(owners); err != nil {
		return err
	}
	return tx.Commit()
}

type coserAssetOwnerQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func coserManagedAssetOwners(ctx context.Context, queryer coserAssetOwnerQueryer) ([]CoserManagedAssetOwner, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT registry.uuid,
			CASE
				WHEN alias.alias_uuid IS NOT NULL THEN 'MERGED'
				WHEN tombstone.uuid IS NOT NULL THEN 'DELETED'
				ELSE 'ACTIVE'
			END,
			COALESCE(coser.avatar_path, ''),
			COALESCE(coser.banner_path, '')
		FROM portable_uuid_registry registry
		LEFT JOIN portable_uuid_aliases alias ON alias.alias_uuid = registry.uuid
		LEFT JOIN portable_uuid_tombstones tombstone ON tombstone.uuid = registry.uuid
		LEFT JOIN cosers coser ON coser.uuid = registry.uuid
		WHERE registry.entity_kind = 'COSER'
		ORDER BY registry.uuid
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CoserManagedAssetOwner
	for rows.Next() {
		var owner CoserManagedAssetOwner
		if err := rows.Scan(&owner.UUID, &owner.State, &owner.AvatarPath, &owner.BannerPath); err != nil {
			return nil, err
		}
		result = append(result, owner)
	}
	return result, rows.Err()
}

// SetCoserManagedAsset publishes a previously validated application-managed
// file reference. The file is stored before this transaction; a revision
// conflict therefore leaves only an unreferenced managed asset, never a
// partially updated Coser or a deleted previous asset.
func (s *CoreEntityStore) SetCoserManagedAsset(ctx context.Context, uuid string, expectedRevision int64, input CoserManagedAssetInput, now time.Time) (coreentity.Coser, error) {
	if input.Kind != CoserAssetAvatar && input.Kind != CoserAssetBanner {
		return coreentity.Coser{}, errors.New("unsupported Coser managed asset kind")
	}
	if err := manifest.ValidateManagedRelativeAsset(input.RelativePath); err != nil {
		return coreentity.Coser{}, err
	}
	if !strings.HasPrefix(filepath.ToSlash(input.RelativePath), "assets/") {
		return coreentity.Coser{}, errors.New("Coser managed asset must be stored below assets")
	}
	if err := validateCoserCrop(input.AvatarCrop, input.BannerFocalPoint); err != nil {
		return coreentity.Coser{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.Coser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := findCoser(ctx, tx, uuid); err != nil {
		return coreentity.Coser{}, err
	}
	timestamp := formatTime(normalisedTime(now))
	var result sqlResult
	if input.Kind == CoserAssetAvatar {
		var cropX, cropY, cropSize any
		if input.AvatarCrop != nil {
			cropX, cropY, cropSize = input.AvatarCrop.X, input.AvatarCrop.Y, input.AvatarCrop.Size
		}
		result, err = tx.ExecContext(ctx, `UPDATE cosers SET avatar_path=?,avatar_crop_x=?,avatar_crop_y=?,avatar_crop_size=?,
			metadata_revision=metadata_revision+1,updated_at_utc=? WHERE uuid=? AND metadata_revision=?`,
			filepath.ToSlash(input.RelativePath), cropX, cropY, cropSize, timestamp, uuid, expectedRevision)
	} else {
		var focalX, focalY any
		if input.BannerFocalPoint != nil {
			focalX, focalY = input.BannerFocalPoint.X, input.BannerFocalPoint.Y
		}
		result, err = tx.ExecContext(ctx, `UPDATE cosers SET banner_path=?,banner_focal_x=?,banner_focal_y=?,
			metadata_revision=metadata_revision+1,updated_at_utc=? WHERE uuid=? AND metadata_revision=?`,
			filepath.ToSlash(input.RelativePath), focalX, focalY, timestamp, uuid, expectedRevision)
	}
	if err != nil {
		return coreentity.Coser{}, err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return coreentity.Coser{}, ErrCoreMetadataRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, `UPDATE coser_manifest_sync SET status=CASE WHEN status='CLEAN' THEN 'DB_DIRTY' ELSE status END WHERE coser_uuid=?`, uuid); err != nil {
		return coreentity.Coser{}, err
	}
	updated, err := findCoser(ctx, tx, uuid)
	if err != nil {
		return coreentity.Coser{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.Coser{}, err
	}
	return updated, nil
}

// ApplyCoserMetadataImport publishes already validated managed assets and
// reviewed social accounts in one database transaction and advances the Coser
// revision exactly once. Provider-specific data never reaches this layer.
func (s *CoreEntityStore) ApplyCoserMetadataImport(ctx context.Context, uuid string, expectedRevision int64, input CoserMetadataImportInput, now time.Time) (coreentity.Coser, error) {
	for kind, asset := range map[CoserAssetKind]*CoserManagedAssetInput{CoserAssetAvatar: input.Avatar, CoserAssetBanner: input.Banner} {
		if asset == nil {
			continue
		}
		if asset.Kind != kind || manifest.ValidateManagedRelativeAsset(asset.RelativePath) != nil ||
			!strings.HasPrefix(filepath.ToSlash(asset.RelativePath), "assets/") || validateCoserCrop(asset.AvatarCrop, asset.BannerFocalPoint) != nil {
			return coreentity.Coser{}, errors.New("invalid prepared Coser managed asset")
		}
	}
	type accountInput struct {
		SocialAccountInput
		URL string
	}
	accounts := make([]accountInput, 0, len(input.Accounts))
	seen := make(map[string]bool)
	for _, account := range input.Accounts {
		if !platformKeyPattern.MatchString(account.PlatformKey) || runeLength(account.Label) > 100 || runeLength(account.Handle) > 200 ||
			(account.Status != "ACTIVE" && account.Status != "INACTIVE") {
			return coreentity.Coser{}, errors.New("invalid SocialAccount field")
		}
		value, err := validateExternalHTTPURL(account.URL)
		if err != nil {
			return coreentity.Coser{}, err
		}
		if !seen[value] {
			seen[value] = true
			accounts = append(accounts, accountInput{SocialAccountInput: account, URL: value})
		}
	}
	if input.Avatar == nil && input.Banner == nil && len(accounts) == 0 {
		return coreentity.Coser{}, errors.New("empty Coser metadata import")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.Coser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	coser, err := findCoser(ctx, tx, uuid)
	if err != nil {
		return coreentity.Coser{}, err
	}
	if coser.MetadataRevision != expectedRevision {
		return coreentity.Coser{}, ErrCoreMetadataRevisionConflict
	}
	existing := make(map[string]bool)
	rows, err := tx.QueryContext(ctx, `SELECT url FROM coser_social_accounts WHERE coser_uuid=?`, uuid)
	if err != nil {
		return coreentity.Coser{}, err
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			_ = rows.Close()
			return coreentity.Coser{}, err
		}
		existing[value] = true
	}
	if err := rows.Close(); err != nil {
		return coreentity.Coser{}, err
	}
	var count int
	var highwater int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(position),0) FROM coser_social_accounts WHERE coser_uuid=?`, uuid).Scan(&count, &highwater); err != nil {
		return coreentity.Coser{}, err
	}
	added := 0
	for _, account := range accounts {
		if existing[account.URL] {
			continue
		}
		if count >= 50 {
			return coreentity.Coser{}, errors.New("Coser SocialAccount limit exceeds 50")
		}
		accountUUID, err := allocateEntityUUID(ctx, tx, "", portableid.KindSocialAccount, now)
		if err != nil {
			return coreentity.Coser{}, err
		}
		highwater += 1024
		if _, err := tx.ExecContext(ctx, `INSERT INTO coser_social_accounts
			(account_uuid,coser_uuid,platform_key,label,handle,url,status,visible,position) VALUES(?,?,?,?,?,?,?,?,?)`,
			accountUUID, uuid, account.PlatformKey, normalizedDisplay(account.Label), normalizedDisplay(account.Handle), account.URL,
			account.Status, account.Visible, highwater); err != nil {
			return coreentity.Coser{}, err
		}
		count++
		added++
	}
	if input.Avatar == nil && input.Banner == nil && added == 0 {
		return coser, tx.Commit()
	}
	avatarPath, avatarCrop := coser.AvatarPath, coser.AvatarCrop
	bannerPath, bannerFocal := coser.BannerPath, coser.BannerFocalPoint
	if input.Avatar != nil {
		avatarPath, avatarCrop = filepath.ToSlash(input.Avatar.RelativePath), input.Avatar.AvatarCrop
	}
	if input.Banner != nil {
		bannerPath, bannerFocal = filepath.ToSlash(input.Banner.RelativePath), input.Banner.BannerFocalPoint
	}
	var cropX, cropY, cropSize, focalX, focalY any
	if avatarCrop != nil {
		cropX, cropY, cropSize = avatarCrop.X, avatarCrop.Y, avatarCrop.Size
	}
	if bannerFocal != nil {
		focalX, focalY = bannerFocal.X, bannerFocal.Y
	}
	result, err := tx.ExecContext(ctx, `UPDATE cosers SET avatar_path=?,avatar_crop_x=?,avatar_crop_y=?,avatar_crop_size=?,
		banner_path=?,banner_focal_x=?,banner_focal_y=?,metadata_revision=metadata_revision+1,updated_at_utc=?
		WHERE uuid=? AND metadata_revision=?`, avatarPath, cropX, cropY, cropSize, bannerPath, focalX, focalY,
		formatTime(normalisedTime(now)), uuid, expectedRevision)
	if err != nil {
		return coreentity.Coser{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return coreentity.Coser{}, ErrCoreMetadataRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, `UPDATE coser_manifest_sync SET status=CASE WHEN status='CLEAN' THEN 'DB_DIRTY' ELSE status END WHERE coser_uuid=?`, uuid); err != nil {
		return coreentity.Coser{}, err
	}
	updated, err := findCoser(ctx, tx, uuid)
	if err != nil {
		return coreentity.Coser{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.Coser{}, err
	}
	return updated, nil
}

type sqlResult interface {
	RowsAffected() (int64, error)
}
