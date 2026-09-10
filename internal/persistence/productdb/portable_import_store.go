package productdb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/textsafe"
	"golang.org/x/text/unicode/norm"
)

var ErrPortableImportTargetNotEmpty = errors.New("portable import requires an empty business database")

var portableImportRelativePathPattern = regexp.MustCompile(`^portable-imports/[0-9a-f-]{36}/package\.zip$`)

type PortableImportSessionInput struct {
	ImportID            string
	ExportID            string
	PackageSHA256       string
	PackageRelativePath string
	FormatVersion       int
	IdentityCount       int
	CoreEntityCount     int
	GalleryClaimCount   int
	ItemClaimCount      int
	LinkClaimCount      int
	AssetCount          int
	GalleryIndex        portablecatalog.GalleryIndex
}

type PortableCoreImportResult struct {
	ImportID          string
	CoreIdentityCount int
	CoreEntityCount   int
	ClaimCount        int
}

type PortableImportSession struct {
	ImportID            string
	ExportID            string
	PackageSHA256       string
	PackageRelativePath string
	State               string
	FormatVersion       int
	IdentityCount       int
	CoreEntityCount     int
	GalleryClaimCount   int
	ItemClaimCount      int
	LinkClaimCount      int
	AssetCount          int
	ErrorCode           string
	CreatedAt           string
	UpdatedAt           string
}

func (db *Database) FindPortableImportSession(ctx context.Context, importID string) (PortableImportSession, error) {
	var result PortableImportSession
	err := db.QueryRowContext(ctx, `SELECT import_id,export_id,package_sha256,package_relative_path,state,format_version,identity_count,core_entity_count,gallery_claim_count,item_claim_count,link_claim_count,asset_count,error_code,created_at_utc,updated_at_utc
		FROM portable_import_sessions WHERE import_id=?`, importID).Scan(
		&result.ImportID, &result.ExportID, &result.PackageSHA256, &result.PackageRelativePath, &result.State, &result.FormatVersion, &result.IdentityCount, &result.CoreEntityCount, &result.GalleryClaimCount, &result.ItemClaimCount, &result.LinkClaimCount, &result.AssetCount, &result.ErrorCode, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return PortableImportSession{}, err
	}
	return result, nil
}

func (db *Database) ListPortableImportSessions(ctx context.Context) ([]PortableImportSession, error) {
	rows, err := db.QueryContext(ctx, `SELECT import_id,export_id,package_sha256,package_relative_path,state,format_version,identity_count,core_entity_count,gallery_claim_count,item_claim_count,link_claim_count,asset_count,error_code,created_at_utc,updated_at_utc FROM portable_import_sessions ORDER BY created_at_utc DESC,import_id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PortableImportSession{}
	for rows.Next() {
		var value PortableImportSession
		if err := rows.Scan(&value.ImportID, &value.ExportID, &value.PackageSHA256, &value.PackageRelativePath, &value.State, &value.FormatVersion, &value.IdentityCount, &value.CoreEntityCount, &value.GalleryClaimCount, &value.ItemClaimCount, &value.LinkClaimCount, &value.AssetCount, &value.ErrorCode, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (db *Database) PortableImportTargetEmpty(ctx context.Context) (bool, error) {
	var occupied int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM portable_uuid_registry)+
		(SELECT COUNT(*) FROM portable_identity_claims)+
		(SELECT COUNT(*) FROM portable_merge_sessions)+
		(SELECT COUNT(*) FROM galleries)+
		(SELECT COUNT(*) FROM cosers)+
		(SELECT COUNT(*) FROM works)+
		(SELECT COUNT(*) FROM characters)+
		(SELECT COUNT(*) FROM tags)+
		(SELECT COUNT(*) FROM coser_social_accounts)`).Scan(&occupied); err != nil {
		return false, err
	}
	return occupied == 0, nil
}

func (db *Database) CreatePortableImportSession(ctx context.Context, input PortableImportSessionInput, now time.Time) error {
	if _, err := portableid.Parse(input.ImportID); err != nil {
		return err
	}
	if _, err := portableid.Parse(input.ExportID); err != nil {
		return err
	}
	if len(input.PackageSHA256) != 64 {
		return errors.New("portable import package digest is invalid")
	}
	if _, err := hex.DecodeString(input.PackageSHA256); err != nil {
		return errors.New("portable import package digest is invalid")
	}
	if !portableImportRelativePathPattern.MatchString(input.PackageRelativePath) || input.FormatVersion < 1 || input.IdentityCount < 0 || input.CoreEntityCount < 0 || input.GalleryClaimCount < 0 || input.ItemClaimCount < 0 || input.LinkClaimCount < 0 || input.AssetCount < 0 {
		return errors.New("portable import session input is invalid")
	}
	empty, err := db.PortableImportTargetEmpty(ctx)
	if err != nil {
		return err
	}
	if !empty {
		return ErrPortableImportTargetNotEmpty
	}
	timestamp := formatTime(normalisedTime(now))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO portable_import_sessions(
		import_id,export_id,package_sha256,package_relative_path,format_version,state,
		identity_count,core_entity_count,gallery_claim_count,item_claim_count,link_claim_count,asset_count,
		created_at_utc,updated_at_utc) VALUES(?,?,?,?,?,'INSPECTED',?,?,?,?,?,?,?,?)`,
		input.ImportID, input.ExportID, strings.ToLower(input.PackageSHA256), input.PackageRelativePath, input.FormatVersion,
		input.IdentityCount, input.CoreEntityCount, input.GalleryClaimCount, input.ItemClaimCount, input.LinkClaimCount, input.AssetCount,
		timestamp, timestamp); err != nil {
		return err
	}
	for _, value := range input.GalleryIndex.Libraries {
		if _, err := tx.ExecContext(ctx, `INSERT INTO portable_library_mappings(import_id,library_key,library_name,updated_at_utc) VALUES(?,?,?,?)`, input.ImportID, value.Key, value.Name, timestamp); err != nil {
			return err
		}
	}
	for _, value := range input.GalleryIndex.Galleries {
		if _, err := tx.ExecContext(ctx, `INSERT INTO portable_gallery_rebuilds(import_id,set_id,library_key,source_type,relative_source,locator_status,manifest_status,manifest_schema,manifest_revision,manifest_hash,updated_at_utc)
			VALUES(?,?,NULLIF(?,''),?,?,?,?,?,?,?,?)`, input.ImportID, value.SetID, value.LibraryKey, value.SourceType, value.RelativeSource, value.LocatorStatus, value.ManifestStatus, value.ManifestSchema, value.ManifestRevision, value.ManifestHash, timestamp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *Database) MarkPortableImportFailed(ctx context.Context, importID, code string, now time.Time) error {
	if len(code) == 0 || len(code) > 100 {
		return errors.New("portable import failure code is invalid")
	}
	result, err := db.ExecContext(ctx, `UPDATE portable_import_sessions SET state='FAILED',error_code=?,updated_at_utc=?
		WHERE import_id=? AND state IN ('INSPECTED','IMPORTING')`, code, formatTime(normalisedTime(now)), importID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable import session cannot be marked failed")
	}
	return nil
}

func (db *Database) BeginPortableImport(ctx context.Context, importID string, now time.Time) error {
	result, err := db.ExecContext(ctx, `UPDATE portable_import_sessions SET state='IMPORTING',error_code='',updated_at_utc=?
		WHERE import_id=? AND state='INSPECTED'`, formatTime(normalisedTime(now)), importID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable import session cannot start")
	}
	return nil
}

func (db *Database) ImportPortableCore(ctx context.Context, importID string, bundle portablecatalog.Bundle, stream func(func(portablecatalog.IdentityRecord) error) (int, error), beforeCommit func() error, now time.Time) (PortableCoreImportResult, error) {
	if stream == nil {
		return PortableCoreImportResult{}, errors.New("portable identity stream is required")
	}
	if err := validatePortableCatalogForImport(bundle.Catalog); err != nil {
		return PortableCoreImportResult{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return PortableCoreImportResult{}, err
	}
	defer tx.Rollback()
	var expectedIdentities, expectedCore, expectedGallery, expectedItems, expectedLinks int
	if err := tx.QueryRowContext(ctx, `SELECT identity_count,core_entity_count,gallery_claim_count,item_claim_count,link_claim_count
		FROM portable_import_sessions WHERE import_id=? AND state='IMPORTING'`, importID).Scan(&expectedIdentities, &expectedCore, &expectedGallery, &expectedItems, &expectedLinks); err != nil {
		return PortableCoreImportResult{}, errors.New("portable import session is unavailable")
	}
	var occupied int
	if err := tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM portable_uuid_registry)+(SELECT COUNT(*) FROM portable_identity_claims)`).Scan(&occupied); err != nil {
		return PortableCoreImportResult{}, err
	}
	if occupied != 0 {
		return PortableCoreImportResult{}, ErrPortableImportTargetNotEmpty
	}
	coreLifecycle := []portablecatalog.IdentityRecord{}
	claimCounts := map[string]int{}
	coreIdentities := 0
	streamed, err := stream(func(identity portablecatalog.IdentityRecord) error {
		kind := portableid.Kind(identity.Kind)
		switch kind {
		case portableid.KindCoser, portableid.KindWork, portableid.KindCharacter, portableid.KindTag, portableid.KindSocialAccount:
			if _, err := tx.ExecContext(ctx, `INSERT INTO portable_uuid_registry(uuid,entity_kind,created_at_utc) VALUES(?,?,?)`, identity.UUID, identity.Kind, identity.CreatedAt); err != nil {
				return err
			}
			coreIdentities++
			if identity.State != "ACTIVE" {
				coreLifecycle = append(coreLifecycle, identity)
			}
		case portableid.KindGallery, portableid.KindGalleryItem, portableid.KindExternalLink:
			if _, err := tx.ExecContext(ctx, `INSERT INTO portable_identity_claims(uuid,import_id,entity_kind,identity_state,target_uuid,created_at_utc,retired_at_utc,reason)
				VALUES(?,?,?,?,?,?,?,?)`, identity.UUID, importID, identity.Kind, identity.State, identity.TargetUUID, identity.CreatedAt, identity.RetiredAt, identity.Reason); err != nil {
				return err
			}
			claimCounts[identity.Kind]++
		default:
			return fmt.Errorf("unsupported portable import identity kind %q", identity.Kind)
		}
		return nil
	})
	if err != nil {
		return PortableCoreImportResult{}, err
	}
	if streamed != expectedIdentities || claimCounts["GALLERY"] != expectedGallery || claimCounts["GALLERY_ITEM"] != expectedItems || claimCounts["EXTERNAL_LINK"] != expectedLinks {
		return PortableCoreImportResult{}, errors.New("portable import identity counts changed after inspection")
	}
	if err := insertPortableCatalog(ctx, tx, bundle.Catalog); err != nil {
		return PortableCoreImportResult{}, err
	}
	if err := insertPortableCoreLifecycle(ctx, tx, coreLifecycle); err != nil {
		return PortableCoreImportResult{}, err
	}
	coreEntities := len(bundle.Catalog.Cosers) + len(bundle.Catalog.Works) + len(bundle.Catalog.Characters) + len(bundle.Catalog.Tags) + len(bundle.Catalog.Accounts)
	if coreEntities != expectedCore {
		return PortableCoreImportResult{}, errors.New("portable import core count changed after inspection")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE portable_import_sessions SET state='CORE_IMPORTED',updated_at_utc=? WHERE import_id=? AND state='IMPORTING'`, formatTime(normalisedTime(now)), importID); err != nil {
		return PortableCoreImportResult{}, err
	}
	if beforeCommit != nil {
		if err := beforeCommit(); err != nil {
			return PortableCoreImportResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return PortableCoreImportResult{}, err
	}
	return PortableCoreImportResult{ImportID: importID, CoreIdentityCount: coreIdentities, CoreEntityCount: coreEntities, ClaimCount: expectedGallery + expectedItems + expectedLinks}, nil
}

func validatePortableCatalogForImport(catalog portablecatalog.CoreCatalog) error {
	for _, entity := range appendNamedPortableEntities(catalog) {
		if err := validateNamedInput(CreateNamedEntityInput{Name: entity.Name, SortName: entity.SortName, Aliases: entity.Aliases}); err != nil {
			return err
		}
		if entity.Name != normalizedDisplay(entity.Name) || entity.SortName != normalizedDisplay(entity.SortName) || runeLength(entity.Slug) > 400 {
			return errors.New("portable core entity text is not canonical")
		}
		for _, alias := range entity.Aliases {
			if alias != normalizedDisplay(alias) {
				return errors.New("portable core Alias is not canonical")
			}
		}
	}
	for _, coser := range catalog.Cosers {
		if runeLength(coser.ProfileSummary) > 20000 || runeLength(coser.Biography) > 20000 || runeLength(coser.CountryOrRegion) > 100 || !norm.NFC.IsNormalString(coser.ProfileSummary) || !norm.NFC.IsNormalString(coser.Biography) || !norm.NFC.IsNormalString(coser.CountryOrRegion) {
			return errors.New("portable Coser profile field is invalid")
		}
		if err := textsafe.ValidateMarkdown(coser.Biography); err != nil {
			return fmt.Errorf("portable Coser biography is invalid: %w", err)
		}
		for _, asset := range []struct {
			ref  *portablecatalog.AssetRef
			kind string
		}{{coser.Avatar, "AVATAR"}, {coser.Banner, "BANNER"}} {
			if _, err := portableImportedAssetRelative(coser.UUID, asset.ref, asset.kind); err != nil {
				return err
			}
		}
	}
	accountCounts := map[string]int{}
	for _, account := range catalog.Accounts {
		if !platformKeyPattern.MatchString(account.PlatformKey) || runeLength(account.Label) > 100 || runeLength(account.Handle) > 200 || account.Position <= 0 || (account.Status != "ACTIVE" && account.Status != "INACTIVE") || account.Label != normalizedDisplay(account.Label) || account.Handle != normalizedDisplay(account.Handle) {
			return errors.New("portable SocialAccount field is invalid")
		}
		normalizedURL, err := validateExternalHTTPURL(account.URL)
		if err != nil || normalizedURL != account.URL {
			return errors.New("portable SocialAccount URL is invalid")
		}
		accountCounts[account.CoserUUID]++
		if accountCounts[account.CoserUUID] > 50 {
			return errors.New("portable Coser SocialAccount limit exceeds 50")
		}
	}
	return nil
}

func appendNamedPortableEntities(catalog portablecatalog.CoreCatalog) []portablecatalog.NamedEntity {
	result := make([]portablecatalog.NamedEntity, 0, len(catalog.Cosers)+len(catalog.Works)+len(catalog.Characters)+len(catalog.Tags))
	for _, value := range catalog.Cosers {
		result = append(result, value.NamedEntity)
	}
	for _, value := range catalog.Works {
		result = append(result, value.NamedEntity)
	}
	for _, value := range catalog.Characters {
		result = append(result, value.NamedEntity)
	}
	for _, value := range catalog.Tags {
		result = append(result, value.NamedEntity)
	}
	return result
}

func portableImportedAssetRelative(coserUUID string, ref *portablecatalog.AssetRef, kind string) (string, error) {
	if ref == nil {
		return "", nil
	}
	prefix := "coser-assets/" + coserUUID + "/"
	name := strings.TrimPrefix(ref.PackagePath, prefix)
	if ref.Kind != kind || name == ref.PackagePath || name == "" || path.Base(name) != name {
		return "", errors.New("portable Coser asset path is invalid")
	}
	return path.Join("assets", name), nil
}

func insertPortableCatalog(ctx context.Context, tx *sql.Tx, catalog portablecatalog.CoreCatalog) error {
	for _, value := range catalog.Works {
		if _, err := tx.ExecContext(ctx, `INSERT INTO works(uuid,name,sort_name,slug,metadata_revision,created_at_utc,updated_at_utc) VALUES(?,?,?,?,?,?,?)`, value.UUID, value.Name, value.SortName, value.Slug, value.MetadataRevision, value.CreatedAt, value.UpdatedAt); err != nil {
			return err
		}
		if err := insertAliases(ctx, tx, "work_aliases", "work_uuid", value.UUID, value.Aliases); err != nil {
			return err
		}
	}
	for _, value := range catalog.Cosers {
		avatar, err := portableImportedAssetRelative(value.UUID, value.Avatar, "AVATAR")
		if err != nil {
			return err
		}
		banner, err := portableImportedAssetRelative(value.UUID, value.Banner, "BANNER")
		if err != nil {
			return err
		}
		var cropX, cropY, cropSize, focalX, focalY any
		if value.AvatarCrop != nil {
			cropX, cropY, cropSize = value.AvatarCrop.X, value.AvatarCrop.Y, value.AvatarCrop.Size
		}
		if value.BannerFocal != nil {
			focalX, focalY = value.BannerFocal.X, value.BannerFocal.Y
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO cosers(uuid,name,sort_name,slug,profile_summary,biography,country_or_region,avatar_path,banner_path,avatar_crop_x,avatar_crop_y,avatar_crop_size,banner_focal_x,banner_focal_y,metadata_revision,created_at_utc,updated_at_utc) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.UUID, value.Name, value.SortName, value.Slug, value.ProfileSummary, value.Biography, value.CountryOrRegion, avatar, banner, cropX, cropY, cropSize, focalX, focalY, value.MetadataRevision, value.CreatedAt, value.UpdatedAt); err != nil {
			return err
		}
		if err := insertAliases(ctx, tx, "coser_aliases", "coser_uuid", value.UUID, value.Aliases); err != nil {
			return err
		}
	}
	for _, value := range catalog.Tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tags(uuid,name,normalized_name,sort_name,slug,use_in_recommendation,metadata_revision,created_at_utc,updated_at_utc) VALUES(?,?,?,?,?,?,?,?,?)`, value.UUID, value.Name, normalizedKey(value.Name), value.SortName, value.Slug, value.UseInRecommendation, value.MetadataRevision, value.CreatedAt, value.UpdatedAt); err != nil {
			return err
		}
		if err := insertAliases(ctx, tx, "tag_aliases", "tag_uuid", value.UUID, value.Aliases); err != nil {
			return err
		}
	}
	for _, value := range catalog.Characters {
		if _, err := tx.ExecContext(ctx, `INSERT INTO characters(uuid,work_uuid,name,normalized_name,sort_name,slug,metadata_revision,created_at_utc,updated_at_utc) VALUES(?,?,?,?,?,?,?,?,?)`, value.UUID, value.WorkUUID, value.Name, normalizedKey(value.Name), value.SortName, value.Slug, value.MetadataRevision, value.CreatedAt, value.UpdatedAt); err != nil {
			return err
		}
		if err := insertAliases(ctx, tx, "character_aliases", "character_uuid", value.UUID, value.Aliases); err != nil {
			return err
		}
	}
	for _, value := range catalog.Accounts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO coser_social_accounts(account_uuid,coser_uuid,platform_key,label,handle,url,status,visible,position) VALUES(?,?,?,?,?,?,?,?,?)`, value.UUID, value.CoserUUID, value.PlatformKey, value.Label, value.Handle, value.URL, value.Status, value.Visible, value.Position); err != nil {
			return err
		}
	}
	for _, value := range catalog.TagEdges {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tag_edges(parent_uuid,child_uuid,position) VALUES(?,?,?)`, value.ParentUUID, value.ChildUUID, value.Position); err != nil {
			return err
		}
	}
	for _, value := range catalog.SlugRedirects {
		if _, err := tx.ExecContext(ctx, `INSERT INTO slug_redirects(entity_kind,old_slug,target_uuid,created_at_utc) VALUES(?,?,?,?)`, value.Kind, value.OldSlug, value.TargetUUID, value.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func insertPortableCoreLifecycle(ctx context.Context, tx *sql.Tx, records []portablecatalog.IdentityRecord) error {
	aliases := map[string]portablecatalog.IdentityRecord{}
	tombstones := []portablecatalog.IdentityRecord{}
	incoming := map[string]int{}
	for _, record := range records {
		if record.State == "ALIAS" {
			aliases[record.UUID] = record
			incoming[record.TargetUUID]++
		} else if record.State == "TOMBSTONE" {
			tombstones = append(tombstones, record)
		}
	}
	for len(aliases) > 0 {
		progress := false
		for uuid, record := range aliases {
			if incoming[uuid] != 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO portable_uuid_aliases(alias_uuid,target_uuid,entity_kind,merged_at_utc) VALUES(?,?,?,?)`, record.UUID, record.TargetUUID, record.Kind, record.RetiredAt); err != nil {
				return err
			}
			delete(aliases, uuid)
			incoming[record.TargetUUID]--
			progress = true
		}
		if !progress {
			return errors.New("portable core identity Alias graph cannot be restored")
		}
	}
	for _, record := range tombstones {
		if _, err := tx.ExecContext(ctx, `INSERT INTO portable_uuid_tombstones(uuid,entity_kind,deleted_at_utc,reason) VALUES(?,?,?,?)`, record.UUID, record.Kind, record.RetiredAt, record.Reason); err != nil {
			return err
		}
	}
	return nil
}
