package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portablecatalog"
)

type PortableAssetSource struct {
	CoserUUID    string
	RelativePath string
	PackagePath  string
	Kind         string
}

type PortableCatalogSnapshot struct {
	Bundle portablecatalog.Bundle
	Assets []PortableAssetSource
}

type PortableCatalogReader struct {
	tx            *sql.Tx
	snapshot      PortableCatalogSnapshot
	identityCount int
}

// BeginPortableCatalogRead captures the non-identity documents and keeps the
// same read transaction open so the identity ledger can be consumed without
// materialising it in memory.
func (db *Database) BeginPortableCatalogRead(ctx context.Context, manifest portablecatalog.PackageManifest) (*PortableCatalogReader, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	result := PortableCatalogSnapshot{Bundle: portablecatalog.Bundle{
		Manifest: manifest,
		Identity: portablecatalog.IdentityLedger{SchemaVersion: 1, Identities: []portablecatalog.IdentityRecord{}},
		Catalog:  portablecatalog.CoreCatalog{SchemaVersion: 1, Cosers: []portablecatalog.Coser{}, Works: []portablecatalog.Work{}, Characters: []portablecatalog.Character{}, Tags: []portablecatalog.Tag{}, TagEdges: []portablecatalog.TagEdge{}, Accounts: []portablecatalog.SocialAccount{}, SlugRedirects: []portablecatalog.SlugRedirect{}},
		Gallery:  portablecatalog.GalleryIndex{SchemaVersion: 1, Libraries: []portablecatalog.LibraryLocator{}, Galleries: []portablecatalog.GalleryLocator{}},
	}, Assets: []PortableAssetSource{}}
	fail := func(readErr error) (*PortableCatalogReader, error) {
		_ = tx.Rollback()
		return nil, readErr
	}
	aliases, err := loadPortableAliases(ctx, tx)
	if err != nil {
		return fail(err)
	}
	if err := loadPortableCosers(ctx, tx, aliases, &result); err != nil {
		return fail(err)
	}
	if err := loadPortableWorks(ctx, tx, aliases, &result); err != nil {
		return fail(err)
	}
	if err := loadPortableCharacters(ctx, tx, aliases, &result); err != nil {
		return fail(err)
	}
	if err := loadPortableTags(ctx, tx, aliases, &result); err != nil {
		return fail(err)
	}
	if err := loadPortableRelations(ctx, tx, &result); err != nil {
		return fail(err)
	}
	if err := loadPortableGalleryIndex(ctx, tx, &result); err != nil {
		return fail(err)
	}
	var identityCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_uuid_registry`).Scan(&identityCount); err != nil {
		return fail(err)
	}
	result.Bundle.Normalize()
	return &PortableCatalogReader{tx: tx, snapshot: result, identityCount: identityCount}, nil
}

func (r *PortableCatalogReader) Snapshot() PortableCatalogSnapshot { return r.snapshot }
func (r *PortableCatalogReader) IdentityCount() int                { return r.identityCount }

func (r *PortableCatalogReader) StreamIdentities(ctx context.Context, yield func(portablecatalog.IdentityRecord) error) error {
	if r == nil || r.tx == nil || yield == nil {
		return errors.New("portable catalog reader is closed")
	}
	rows, err := r.tx.QueryContext(ctx, `SELECT registry.uuid,registry.entity_kind,registry.created_at_utc,
		COALESCE(alias.target_uuid,''),COALESCE(alias.merged_at_utc,''),COALESCE(tombstone.deleted_at_utc,''),COALESCE(tombstone.reason,'')
		FROM portable_uuid_registry registry LEFT JOIN portable_uuid_aliases alias ON alias.alias_uuid=registry.uuid
		LEFT JOIN portable_uuid_tombstones tombstone ON tombstone.uuid=registry.uuid ORDER BY registry.uuid`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value portablecatalog.IdentityRecord
		var mergedAt, deletedAt string
		if err := rows.Scan(&value.UUID, &value.Kind, &value.CreatedAt, &value.TargetUUID, &mergedAt, &deletedAt, &value.Reason); err != nil {
			return err
		}
		switch {
		case value.TargetUUID != "":
			value.State, value.RetiredAt, value.Reason = "ALIAS", mergedAt, ""
		case deletedAt != "":
			value.State, value.RetiredAt = "TOMBSTONE", deletedAt
		default:
			value.State, value.Reason = "ACTIVE", ""
		}
		if err := yield(value); err != nil {
			return err
		}
	}
	return rows.Err()
}

// StreamMergeOccupiedIdentities includes unresolved portable-workflow reservations
// in addition to the active Registry. Reserved records use a synthetic local
// state so a different workflow can never mistake them for reusable identity.
func (r *PortableCatalogReader) StreamMergeOccupiedIdentities(ctx context.Context, yield func(portablecatalog.IdentityRecord) error) error {
	if r == nil || r.tx == nil || yield == nil {
		return errors.New("portable catalog reader is closed")
	}
	rows, err := r.tx.QueryContext(ctx, `SELECT uuid,entity_kind,state,target_uuid,created_at_utc,retired_at_utc,reason FROM (
		SELECT registry.uuid AS uuid,registry.entity_kind AS entity_kind,
			CASE WHEN alias.alias_uuid IS NOT NULL THEN 'ALIAS' WHEN tombstone.uuid IS NOT NULL THEN 'TOMBSTONE' ELSE 'ACTIVE' END AS state,
			COALESCE(alias.target_uuid,'') AS target_uuid,registry.created_at_utc AS created_at_utc,
			COALESCE(alias.merged_at_utc,tombstone.deleted_at_utc,'') AS retired_at_utc,COALESCE(tombstone.reason,'') AS reason
		FROM portable_uuid_registry registry LEFT JOIN portable_uuid_aliases alias ON alias.alias_uuid=registry.uuid
		LEFT JOIN portable_uuid_tombstones tombstone ON tombstone.uuid=registry.uuid
		UNION ALL
		SELECT uuid,entity_kind,'RESERVED_'||identity_state,target_uuid,created_at_utc,retired_at_utc,reason
		FROM portable_identity_claims WHERE claim_state<>'CLAIMED'
	) ORDER BY uuid`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value portablecatalog.IdentityRecord
		if err := rows.Scan(&value.UUID, &value.Kind, &value.State, &value.TargetUUID, &value.CreatedAt, &value.RetiredAt, &value.Reason); err != nil {
			return err
		}
		if err := yield(value); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *PortableCatalogReader) Commit() error {
	if r == nil || r.tx == nil {
		return errors.New("portable catalog reader is closed")
	}
	err := r.tx.Commit()
	r.tx = nil
	return err
}

func (r *PortableCatalogReader) Close() error {
	if r == nil || r.tx == nil {
		return nil
	}
	err := r.tx.Rollback()
	r.tx = nil
	return err
}

// PortableCatalogSnapshot reads every portable identity and core catalog row
// in one read transaction. Returned asset source paths are consumed only by
// the authenticated server and are never serialized into the package.
func (db *Database) PortableCatalogSnapshot(ctx context.Context, manifest portablecatalog.PackageManifest) (PortableCatalogSnapshot, error) {
	reader, err := db.BeginPortableCatalogRead(ctx, manifest)
	if err != nil {
		return PortableCatalogSnapshot{}, err
	}
	defer reader.Close()
	result := reader.Snapshot()
	if err := reader.StreamIdentities(ctx, func(value portablecatalog.IdentityRecord) error {
		result.Bundle.Identity.Identities = append(result.Bundle.Identity.Identities, value)
		return nil
	}); err != nil {
		return PortableCatalogSnapshot{}, err
	}
	if err := reader.Commit(); err != nil {
		return PortableCatalogSnapshot{}, err
	}
	result.Bundle.Normalize()
	return result, nil
}

func loadPortableAliases(ctx context.Context, tx *sql.Tx) (map[string][]string, error) {
	result := map[string][]string{}
	for _, item := range []struct{ table, column string }{{"coser_aliases", "coser_uuid"}, {"work_aliases", "work_uuid"}, {"character_aliases", "character_uuid"}, {"tag_aliases", "tag_uuid"}} {
		rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT %s,alias FROM %s ORDER BY %s,position`, item.column, item.table, item.column))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var uuid, alias string
			if err := rows.Scan(&uuid, &alias); err != nil {
				rows.Close()
				return nil, err
			}
			result[uuid] = append(result[uuid], alias)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func loadPortableCosers(ctx context.Context, tx *sql.Tx, aliases map[string][]string, result *PortableCatalogSnapshot) error {
	rows, err := tx.QueryContext(ctx, `SELECT uuid,name,sort_name,slug,profile_summary,biography,country_or_region,avatar_path,banner_path,
		avatar_crop_x,avatar_crop_y,avatar_crop_size,banner_focal_x,banner_focal_y,metadata_revision,created_at_utc,updated_at_utc FROM cosers ORDER BY uuid`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value portablecatalog.Coser
		var avatarPath, bannerPath string
		var cropX, cropY, cropSize, focalX, focalY sql.NullFloat64
		if err := rows.Scan(&value.UUID, &value.Name, &value.SortName, &value.Slug, &value.ProfileSummary, &value.Biography, &value.CountryOrRegion, &avatarPath, &bannerPath, &cropX, &cropY, &cropSize, &focalX, &focalY, &value.MetadataRevision, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return err
		}
		value.Aliases = append([]string{}, aliases[value.UUID]...)
		if cropX.Valid {
			value.AvatarCrop = &portablecatalog.AvatarCrop{X: cropX.Float64, Y: cropY.Float64, Size: cropSize.Float64}
		}
		if focalX.Valid {
			value.BannerFocal = &portablecatalog.FocalPoint{X: focalX.Float64, Y: focalY.Float64}
		}
		if err := addPortableAsset(value.UUID, avatarPath, "AVATAR", &value.Avatar, result); err != nil {
			return err
		}
		if err := addPortableAsset(value.UUID, bannerPath, "BANNER", &value.Banner, result); err != nil {
			return err
		}
		result.Bundle.Catalog.Cosers = append(result.Bundle.Catalog.Cosers, value)
	}
	return rows.Err()
}

func addPortableAsset(coserUUID, relative, kind string, target **portablecatalog.AssetRef, result *PortableCatalogSnapshot) error {
	if relative == "" {
		return nil
	}
	if err := manifest.ValidateManagedRelativeAsset(relative); err != nil {
		return err
	}
	if !strings.HasPrefix(relative, "assets/") {
		return errors.New("portable Coser asset is outside assets directory")
	}
	packagePath := path.Join("coser-assets", coserUUID, path.Base(relative))
	*target = &portablecatalog.AssetRef{PackagePath: packagePath, Kind: kind}
	result.Assets = append(result.Assets, PortableAssetSource{CoserUUID: coserUUID, RelativePath: relative, PackagePath: packagePath, Kind: "COSER_" + kind + "_ORIGINAL"})
	return nil
}

func loadPortableWorks(ctx context.Context, tx *sql.Tx, aliases map[string][]string, result *PortableCatalogSnapshot) error {
	rows, err := tx.QueryContext(ctx, `SELECT uuid,name,sort_name,slug,metadata_revision,created_at_utc,updated_at_utc FROM works ORDER BY uuid`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value portablecatalog.Work
		if err := rows.Scan(&value.UUID, &value.Name, &value.SortName, &value.Slug, &value.MetadataRevision, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return err
		}
		value.Aliases = append([]string{}, aliases[value.UUID]...)
		result.Bundle.Catalog.Works = append(result.Bundle.Catalog.Works, value)
	}
	return rows.Err()
}

func loadPortableCharacters(ctx context.Context, tx *sql.Tx, aliases map[string][]string, result *PortableCatalogSnapshot) error {
	rows, err := tx.QueryContext(ctx, `SELECT uuid,work_uuid,name,sort_name,slug,metadata_revision,created_at_utc,updated_at_utc FROM characters ORDER BY uuid`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value portablecatalog.Character
		if err := rows.Scan(&value.UUID, &value.WorkUUID, &value.Name, &value.SortName, &value.Slug, &value.MetadataRevision, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return err
		}
		value.Aliases = append([]string{}, aliases[value.UUID]...)
		result.Bundle.Catalog.Characters = append(result.Bundle.Catalog.Characters, value)
	}
	return rows.Err()
}

func loadPortableTags(ctx context.Context, tx *sql.Tx, aliases map[string][]string, result *PortableCatalogSnapshot) error {
	rows, err := tx.QueryContext(ctx, `SELECT uuid,name,sort_name,slug,use_in_recommendation,metadata_revision,created_at_utc,updated_at_utc FROM tags ORDER BY uuid`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value portablecatalog.Tag
		var recommendation int
		if err := rows.Scan(&value.UUID, &value.Name, &value.SortName, &value.Slug, &recommendation, &value.MetadataRevision, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return err
		}
		value.UseInRecommendation = recommendation == 1
		value.Aliases = append([]string{}, aliases[value.UUID]...)
		result.Bundle.Catalog.Tags = append(result.Bundle.Catalog.Tags, value)
	}
	return rows.Err()
}

func loadPortableRelations(ctx context.Context, tx *sql.Tx, result *PortableCatalogSnapshot) error {
	rows, err := tx.QueryContext(ctx, `SELECT account_uuid,coser_uuid,platform_key,label,handle,url,status,visible,position FROM coser_social_accounts ORDER BY account_uuid`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value portablecatalog.SocialAccount
		var visible int
		if err := rows.Scan(&value.UUID, &value.CoserUUID, &value.PlatformKey, &value.Label, &value.Handle, &value.URL, &value.Status, &visible, &value.Position); err != nil {
			rows.Close()
			return err
		}
		value.Visible = visible == 1
		result.Bundle.Catalog.Accounts = append(result.Bundle.Catalog.Accounts, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, `SELECT parent_uuid,child_uuid,position FROM tag_edges ORDER BY child_uuid,position,parent_uuid`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value portablecatalog.TagEdge
		if err := rows.Scan(&value.ParentUUID, &value.ChildUUID, &value.Position); err != nil {
			rows.Close()
			return err
		}
		result.Bundle.Catalog.TagEdges = append(result.Bundle.Catalog.TagEdges, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, `SELECT entity_kind,old_slug,target_uuid,created_at_utc FROM slug_redirects WHERE entity_kind IN ('COSER','WORK','CHARACTER','TAG') ORDER BY entity_kind,old_slug`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value portablecatalog.SlugRedirect
		if err := rows.Scan(&value.Kind, &value.OldSlug, &value.TargetUUID, &value.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		result.Bundle.Catalog.SlugRedirects = append(result.Bundle.Catalog.SlugRedirects, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return rows.Err()
}

func loadPortableGalleryIndex(ctx context.Context, tx *sql.Tx, result *PortableCatalogSnapshot) error {
	libraryKeys := map[int64]string{}
	libraryRoots := map[int64]string{}
	rows, err := tx.QueryContext(ctx, `SELECT id,name,root_path FROM media_libraries ORDER BY id`)
	if err != nil {
		return err
	}
	ordinal := 0
	for rows.Next() {
		var id int64
		var name, root string
		if err := rows.Scan(&id, &name, &root); err != nil {
			rows.Close()
			return err
		}
		ordinal++
		key := fmt.Sprintf("library-%06d", ordinal)
		libraryKeys[id] = key
		libraryRoots[id] = root
		result.Bundle.Gallery.Libraries = append(result.Bundle.Gallery.Libraries, portablecatalog.LibraryLocator{Key: key, Name: name})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, `SELECT gallery.set_id,source.library_id,COALESCE(source.source_type,''),COALESCE(source.source_path,''),
		COALESCE(sync.status,'MISSING'),COALESCE(sync.schema_version,0),COALESCE(sync.manifest_revision,0),COALESCE(sync.file_hash,'')
		FROM galleries gallery LEFT JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_manifest_sync sync ON sync.gallery_id=gallery.id ORDER BY gallery.set_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value portablecatalog.GalleryLocator
		var libraryID sql.NullInt64
		var sourcePath string
		if err := rows.Scan(&value.SetID, &libraryID, &value.SourceType, &sourcePath, &value.ManifestStatus, &value.ManifestSchema, &value.ManifestRevision, &value.ManifestHash); err != nil {
			return err
		}
		value.LocatorStatus = "UNBOUND"
		if libraryID.Valid {
			value.LibraryKey = libraryKeys[libraryID.Int64]
			if relative, ok := portableRelativeSource(libraryRoots[libraryID.Int64], sourcePath); ok {
				if relative == "." {
					value.LocatorStatus = "LIBRARY_ROOT"
				} else {
					value.RelativeSource = relative
					value.LocatorStatus = "MAPPED"
				}
			} else {
				value.LocatorStatus = "OUTSIDE_LIBRARY"
			}
		}
		result.Bundle.Gallery.Galleries = append(result.Bundle.Gallery.Galleries, value)
	}
	return rows.Err()
}

func portableRelativeSource(root, source string) (string, bool) {
	if root == "" || source == "" {
		return "", false
	}
	relative, err := filepath.Rel(root, source)
	if err != nil {
		return "", false
	}
	relative = filepath.ToSlash(relative)
	if relative == "." {
		return ".", true
	}
	if relative == ".." || strings.HasPrefix(relative, "../") || strings.HasPrefix(relative, "/") {
		return "", false
	}
	return relative, true
}
