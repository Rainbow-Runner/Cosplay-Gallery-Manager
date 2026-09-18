package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/portableid"
)

type PortableCoreMergeResult struct {
	MergeID              string
	NewCoreIdentities    int
	ReusedIdentities     int
	PendingGalleryClaims int
}

// MergePortableCoreConflictFree remains as the narrow compatibility entry
// point used by lower-level rollback tests.
func (db *Database) MergePortableCoreConflictFree(ctx context.Context, mergeID, targetFingerprint, safetyBackupID string, bundle portablecatalog.Bundle, stream func(func(portablecatalog.IdentityRecord) error) (int, error), now time.Time) (PortableCoreMergeResult, error) {
	return db.MergePortableCore(ctx, mergeID, targetFingerprint, safetyBackupID, bundle, nil, stream, nil, now)
}

// MergePortableCore atomically applies a reviewed core merge and creates a
// Stage-3-compatible rebuild workflow for still-missing Gallery identities.
// A MAP_TO_LOCAL decision turns the incoming UUID into a permanent Alias; it
// never silently discards or regenerates an identity.
func (db *Database) MergePortableCore(ctx context.Context, mergeID, targetFingerprint, safetyBackupID string, bundle portablecatalog.Bundle, conflicts []PortableMergeConflict, stream func(func(portablecatalog.IdentityRecord) error) (int, error), beforeCommit func() error, now time.Time) (PortableCoreMergeResult, error) {
	result := PortableCoreMergeResult{MergeID: mergeID}
	if stream == nil || safetyBackupID == "" {
		return result, errors.New("portable merge stream and safety backup are required")
	}
	if err := validatePortableCatalogForImport(bundle.Catalog); err != nil {
		return result, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var state, expectedFingerprint string
	var hard, review, expectedAdd, expectedReuse int
	if err := tx.QueryRowContext(ctx, `SELECT state,target_fingerprint,hard_blocking_count,review_count,identity_add_count,identity_reuse_count
		FROM portable_merge_sessions WHERE merge_id=?`, mergeID).Scan(&state, &expectedFingerprint, &hard, &review, &expectedAdd, &expectedReuse); err != nil {
		return result, err
	}
	if state != "READY" || hard != 0 || expectedFingerprint != targetFingerprint {
		return result, errors.New("portable merge is not a reviewed ready session")
	}
	resolution, err := buildPortableMergeResolution(conflicts)
	if err != nil || len(conflicts) != review {
		return result, errors.New("portable merge decisions are incomplete or invalid")
	}
	timestamp := formatTime(normalisedTime(now))
	coreCount := len(bundle.Catalog.Cosers) + len(bundle.Catalog.Works) + len(bundle.Catalog.Characters) + len(bundle.Catalog.Tags) + len(bundle.Catalog.Accounts)
	if _, err := tx.ExecContext(ctx, `INSERT INTO portable_import_sessions(import_id,export_id,package_sha256,package_relative_path,format_version,state,identity_count,core_entity_count,gallery_claim_count,item_claim_count,link_claim_count,asset_count,created_at_utc,updated_at_utc)
		SELECT merge_id,export_id,package_sha256,package_relative_path,format_version,'CORE_IMPORTED',identity_add_count+identity_reuse_count,?,0,0,0,0,?,? FROM portable_merge_sessions WHERE merge_id=?`, coreCount, timestamp, timestamp, mergeID); err != nil {
		return result, err
	}
	for _, value := range bundle.Gallery.Libraries {
		if _, err := tx.ExecContext(ctx, `INSERT INTO portable_library_mappings(import_id,library_key,library_name,updated_at_utc) VALUES(?,?,?,?)`, mergeID, value.Key, value.Name, timestamp); err != nil {
			return result, err
		}
	}
	coreLifecycle := []portablecatalog.IdentityRecord{}
	claimedGallery := map[string]bool{}
	streamed, err := stream(func(identity portablecatalog.IdentityRecord) error {
		if target := resolution.mapped[identity.UUID]; target != "" {
			createdAt, err := parseTime(identity.CreatedAt)
			if err != nil {
				return err
			}
			if _, err := registerPortableUUID(ctx, tx, identity.UUID, portableid.Kind(identity.Kind), createdAt); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO portable_uuid_aliases(alias_uuid,target_uuid,entity_kind,merged_at_utc) VALUES(?,?,?,?)`, identity.UUID, target, identity.Kind, timestamp); err != nil {
				return err
			}
			result.NewCoreIdentities++
			return nil
		}
		local, found, err := portableIdentityInTx(ctx, tx, identity.UUID)
		if err != nil {
			return err
		}
		if found {
			if local != identity {
				return errors.New("portable merge reused identity changed after preflight")
			}
			result.ReusedIdentities++
			return nil
		}
		kind := portableid.Kind(identity.Kind)
		switch kind {
		case portableid.KindCoser, portableid.KindWork, portableid.KindCharacter, portableid.KindTag, portableid.KindSocialAccount:
			createdAt, err := parseTime(identity.CreatedAt)
			if err != nil {
				return err
			}
			if _, err := registerPortableUUID(ctx, tx, identity.UUID, kind, createdAt); err != nil {
				return err
			}
			result.NewCoreIdentities++
			if identity.State != "ACTIVE" {
				coreLifecycle = append(coreLifecycle, identity)
			}
		case portableid.KindGallery, portableid.KindGalleryItem, portableid.KindExternalLink:
			if _, err := tx.ExecContext(ctx, `INSERT INTO portable_identity_claims(uuid,import_id,entity_kind,identity_state,target_uuid,created_at_utc,retired_at_utc,reason)
				VALUES(?,?,?,?,?,?,?,?)`, identity.UUID, mergeID, identity.Kind, identity.State, identity.TargetUUID, identity.CreatedAt, identity.RetiredAt, identity.Reason); err != nil {
				return err
			}
			if kind == portableid.KindGallery && identity.State == "ACTIVE" {
				claimedGallery[identity.UUID] = true
			}
			result.PendingGalleryClaims++
		default:
			return errors.New("portable merge identity kind is unsupported")
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if streamed != expectedAdd+expectedReuse || result.NewCoreIdentities+result.PendingGalleryClaims != expectedAdd || result.ReusedIdentities != expectedReuse {
		return result, errors.New("portable merge identity counts changed after preflight")
	}
	for _, value := range bundle.Gallery.Galleries {
		state := "SKIPPED"
		if claimedGallery[value.SetID] {
			state = "PENDING"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO portable_gallery_rebuilds(import_id,set_id,library_key,source_type,relative_source,locator_status,manifest_status,manifest_schema,manifest_revision,manifest_hash,state,updated_at_utc)
			VALUES(?,?,NULLIF(?,''),?,?,?,?,?,?,?,?,?)`, mergeID, value.SetID, value.LibraryKey, value.SourceType, value.RelativeSource, value.LocatorStatus, value.ManifestStatus, value.ManifestSchema, value.ManifestRevision, value.ManifestHash, state, timestamp); err != nil {
			return result, err
		}
	}
	if err := mergePortableCatalogReviewed(ctx, tx, bundle.Catalog, resolution); err != nil {
		return result, err
	}
	if err := insertPortableCoreLifecycle(ctx, tx, coreLifecycle); err != nil {
		return result, err
	}
	updated, err := tx.ExecContext(ctx, `UPDATE portable_merge_sessions SET state='APPLIED',safety_backup_id=?,updated_at_utc=? WHERE merge_id=? AND state='READY'`, safetyBackupID, timestamp, mergeID)
	if err != nil {
		return result, err
	}
	if changed, err := updated.RowsAffected(); err != nil || changed != 1 {
		return result, errors.New("portable merge session could not complete")
	}
	if beforeCommit != nil {
		if err := beforeCommit(); err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

type portableMergeResolution struct {
	mapped, content, accountPosition, tagEdge map[string]string
}

func buildPortableMergeResolution(conflicts []PortableMergeConflict) (portableMergeResolution, error) {
	r := portableMergeResolution{mapped: map[string]string{}, content: map[string]string{}, accountPosition: map[string]string{}, tagEdge: map[string]string{}}
	for _, value := range conflicts {
		if value.Severity != "REVIEW" || value.Decision == "UNRESOLVED" || !portableMergeDecisionAllowed(value.IssueCode, value.Decision) {
			return r, errors.New("unresolved portable merge decision")
		}
		switch value.IssueCode {
		case "PORTABLE_CORE_NAME_MATCH_REVIEW", "PORTABLE_SOCIAL_ACCOUNT_URL_REVIEW":
			if value.Decision == "MAP_TO_LOCAL" {
				r.mapped[value.IncomingUUID] = value.LocalUUID
			}
		case "PORTABLE_CORE_ENTITY_CONTENT_CONFLICT":
			r.content[value.IncomingUUID] = value.Decision
		case "PORTABLE_SOCIAL_ACCOUNT_POSITION_CONFLICT":
			r.accountPosition[value.IncomingUUID] = value.Decision
		case "PORTABLE_TAG_EDGE_POSITION_CONFLICT", "PORTABLE_TAG_EDGE_SLOT_CONFLICT":
			r.tagEdge[value.FieldKey] = value.Decision
		}
	}
	return r, nil
}

func portableIdentityInTx(ctx context.Context, tx *sql.Tx, uuid string) (portablecatalog.IdentityRecord, bool, error) {
	var value portablecatalog.IdentityRecord
	var mergedAt, deletedAt string
	err := tx.QueryRowContext(ctx, `SELECT registry.uuid,registry.entity_kind,registry.created_at_utc,COALESCE(alias.target_uuid,''),COALESCE(alias.merged_at_utc,''),COALESCE(tombstone.deleted_at_utc,''),COALESCE(tombstone.reason,'')
		FROM portable_uuid_registry registry LEFT JOIN portable_uuid_aliases alias ON alias.alias_uuid=registry.uuid
		LEFT JOIN portable_uuid_tombstones tombstone ON tombstone.uuid=registry.uuid WHERE registry.uuid=?`, uuid).Scan(&value.UUID, &value.Kind, &value.CreatedAt, &value.TargetUUID, &mergedAt, &deletedAt, &value.Reason)
	if errors.Is(err, sql.ErrNoRows) {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	switch {
	case value.TargetUUID != "":
		value.State, value.RetiredAt, value.Reason = "ALIAS", mergedAt, ""
	case deletedAt != "":
		value.State, value.RetiredAt = "TOMBSTONE", deletedAt
	default:
		value.State, value.Reason = "ACTIVE", ""
	}
	return value, true, nil
}

func mergePortableCatalogReviewed(ctx context.Context, tx *sql.Tx, catalog portablecatalog.CoreCatalog, resolution portableMergeResolution) error {
	exists := func(table, column, uuid string) (bool, error) {
		var count int
		err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+column+`=?`, uuid).Scan(&count)
		return count == 1, err
	}
	for _, value := range catalog.Works {
		if resolution.mapped[value.UUID] != "" {
			continue
		}
		found, err := exists("works", "uuid", value.UUID)
		if err != nil {
			return err
		}
		if found && resolution.content[value.UUID] != "USE_INCOMING" {
			continue
		}
		if found {
			if err := replacePortableNamedEntity(ctx, tx, "works", "work_aliases", "work_uuid", value.NamedEntity, ""); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO works(uuid,name,sort_name,slug,metadata_revision,created_at_utc,updated_at_utc) VALUES(?,?,?,?,?,?,?)`, value.UUID, value.Name, value.SortName, value.Slug, value.MetadataRevision, value.CreatedAt, value.UpdatedAt); err != nil {
			return err
		}
		if err := insertAliases(ctx, tx, "work_aliases", "work_uuid", value.UUID, value.Aliases); err != nil {
			return err
		}
	}
	for _, value := range catalog.Cosers {
		if resolution.mapped[value.UUID] != "" {
			continue
		}
		found, err := exists("cosers", "uuid", value.UUID)
		if err != nil {
			return err
		}
		if found && resolution.content[value.UUID] != "USE_INCOMING" {
			continue
		}
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
		if found {
			if _, err := tx.ExecContext(ctx, `UPDATE cosers SET name=?,sort_name=?,slug=?,profile_summary=?,biography=?,country_or_region=?,avatar_path=?,banner_path=?,avatar_crop_x=?,avatar_crop_y=?,avatar_crop_size=?,banner_focal_x=?,banner_focal_y=?,metadata_revision=?,updated_at_utc=? WHERE uuid=?`, value.Name, value.SortName, value.Slug, value.ProfileSummary, value.Biography, value.CountryOrRegion, avatar, banner, cropX, cropY, cropSize, focalX, focalY, value.MetadataRevision, value.UpdatedAt, value.UUID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM coser_aliases WHERE coser_uuid=?`, value.UUID); err != nil {
				return err
			}
			if err := insertAliases(ctx, tx, "coser_aliases", "coser_uuid", value.UUID, value.Aliases); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO cosers(uuid,name,sort_name,slug,profile_summary,biography,country_or_region,avatar_path,banner_path,avatar_crop_x,avatar_crop_y,avatar_crop_size,banner_focal_x,banner_focal_y,metadata_revision,created_at_utc,updated_at_utc) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.UUID, value.Name, value.SortName, value.Slug, value.ProfileSummary, value.Biography, value.CountryOrRegion, avatar, banner, cropX, cropY, cropSize, focalX, focalY, value.MetadataRevision, value.CreatedAt, value.UpdatedAt); err != nil {
			return err
		}
		if err := insertAliases(ctx, tx, "coser_aliases", "coser_uuid", value.UUID, value.Aliases); err != nil {
			return err
		}
	}
	for _, value := range catalog.Tags {
		if resolution.mapped[value.UUID] != "" {
			continue
		}
		found, err := exists("tags", "uuid", value.UUID)
		if err != nil {
			return err
		}
		if found && resolution.content[value.UUID] != "USE_INCOMING" {
			continue
		}
		if found {
			if _, err := tx.ExecContext(ctx, `UPDATE tags SET name=?,normalized_name=?,sort_name=?,slug=?,use_in_recommendation=?,allow_direct_assignment=?,metadata_revision=?,updated_at_utc=? WHERE uuid=?`, value.Name, normalizedKey(value.Name), value.SortName, value.Slug, value.UseInRecommendation, value.DirectAssignmentAllowed(), value.MetadataRevision, value.UpdatedAt, value.UUID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM tag_aliases WHERE tag_uuid=?`, value.UUID); err != nil {
				return err
			}
			if err := insertAliases(ctx, tx, "tag_aliases", "tag_uuid", value.UUID, value.Aliases); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tags(uuid,name,normalized_name,sort_name,slug,use_in_recommendation,allow_direct_assignment,metadata_revision,created_at_utc,updated_at_utc) VALUES(?,?,?,?,?,?,?,?,?,?)`, value.UUID, value.Name, normalizedKey(value.Name), value.SortName, value.Slug, value.UseInRecommendation, value.DirectAssignmentAllowed(), value.MetadataRevision, value.CreatedAt, value.UpdatedAt); err != nil {
			return err
		}
		if err := insertAliases(ctx, tx, "tag_aliases", "tag_uuid", value.UUID, value.Aliases); err != nil {
			return err
		}
	}
	for _, value := range catalog.Characters {
		if resolution.mapped[value.UUID] != "" {
			continue
		}
		if target := resolution.mapped[value.WorkUUID]; target != "" {
			value.WorkUUID = target
		}
		found, err := exists("characters", "uuid", value.UUID)
		if err != nil {
			return err
		}
		if found && resolution.content[value.UUID] != "USE_INCOMING" {
			continue
		}
		if found {
			if err := replacePortableNamedEntity(ctx, tx, "characters", "character_aliases", "character_uuid", value.NamedEntity, value.WorkUUID); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO characters(uuid,work_uuid,name,normalized_name,sort_name,slug,metadata_revision,created_at_utc,updated_at_utc) VALUES(?,?,?,?,?,?,?,?,?)`, value.UUID, value.WorkUUID, value.Name, normalizedKey(value.Name), value.SortName, value.Slug, value.MetadataRevision, value.CreatedAt, value.UpdatedAt); err != nil {
			return err
		}
		if err := insertAliases(ctx, tx, "character_aliases", "character_uuid", value.UUID, value.Aliases); err != nil {
			return err
		}
	}
	for _, value := range catalog.Accounts {
		if resolution.mapped[value.UUID] != "" {
			continue
		}
		ownerMapped := false
		if target := resolution.mapped[value.CoserUUID]; target != "" {
			value.CoserUUID = target
			ownerMapped = true
		}
		found, err := exists("coser_social_accounts", "account_uuid", value.UUID)
		if err != nil {
			return err
		}
		if found && resolution.content[value.UUID] != "USE_INCOMING" {
			continue
		}
		if found {
			if _, err := tx.ExecContext(ctx, `UPDATE coser_social_accounts SET coser_uuid=?,platform_key=?,label=?,handle=?,url=?,status=?,visible=?,position=? WHERE account_uuid=?`, value.CoserUUID, value.PlatformKey, value.Label, value.Handle, value.URL, value.Status, value.Visible, value.Position, value.UUID); err != nil {
				return err
			}
			continue
		}
		if resolution.accountPosition[value.UUID] == "KEEP_LOCAL" || ownerMapped && portableAccountPositionOccupied(ctx, tx, value.CoserUUID, value.Position) {
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position),0)+1024 FROM coser_social_accounts WHERE coser_uuid=?`, value.CoserUUID).Scan(&value.Position); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO coser_social_accounts(account_uuid,coser_uuid,platform_key,label,handle,url,status,visible,position) VALUES(?,?,?,?,?,?,?,?,?)`, value.UUID, value.CoserUUID, value.PlatformKey, value.Label, value.Handle, value.URL, value.Status, value.Visible, value.Position); err != nil {
			return err
		}
	}
	for _, value := range catalog.TagEdges {
		originalKey := value.ParentUUID + ":" + value.ChildUUID
		if target := resolution.mapped[value.ParentUUID]; target != "" {
			value.ParentUUID = target
		}
		if target := resolution.mapped[value.ChildUUID]; target != "" {
			value.ChildUUID = target
		}
		decision := resolution.tagEdge[originalKey]
		if decision == "KEEP_LOCAL" {
			continue
		}
		if decision == "USE_INCOMING" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM tag_edges WHERE parent_uuid=? AND child_uuid=?`, value.ParentUUID, value.ChildUUID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM tag_edges WHERE child_uuid=? AND position=?`, value.ChildUUID, value.Position); err != nil {
				return err
			}
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tag_edges WHERE parent_uuid=? AND child_uuid=? AND position=?`, value.ParentUUID, value.ChildUUID, value.Position).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO tag_edges(parent_uuid,child_uuid,position) VALUES(?,?,?)`, value.ParentUUID, value.ChildUUID, value.Position); err != nil {
				return err
			}
		}
	}
	for _, value := range catalog.SlugRedirects {
		if target := resolution.mapped[value.TargetUUID]; target != "" {
			value.TargetUUID = target
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM slug_redirects WHERE entity_kind=? AND old_slug=? AND target_uuid=?`, value.Kind, value.OldSlug, value.TargetUUID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO slug_redirects(entity_kind,old_slug,target_uuid,created_at_utc) VALUES(?,?,?,?)`, value.Kind, value.OldSlug, value.TargetUUID, value.CreatedAt); err != nil {
				return err
			}
		}
	}
	return nil
}

func portableAccountPositionOccupied(ctx context.Context, tx *sql.Tx, coserUUID string, position int64) bool {
	var count int
	return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM coser_social_accounts WHERE coser_uuid=? AND position=?`, coserUUID, position).Scan(&count) == nil && count != 0
}

func replacePortableNamedEntity(ctx context.Context, tx *sql.Tx, table, aliasTable, aliasColumn string, value portablecatalog.NamedEntity, workUUID string) error {
	query := fmt.Sprintf(`UPDATE %s SET name=?,sort_name=?,slug=?,metadata_revision=?,updated_at_utc=? WHERE uuid=?`, table)
	args := []any{value.Name, value.SortName, value.Slug, value.MetadataRevision, value.UpdatedAt, value.UUID}
	if table == "characters" {
		query = `UPDATE characters SET work_uuid=?,name=?,normalized_name=?,sort_name=?,slug=?,metadata_revision=?,updated_at_utc=? WHERE uuid=?`
		args = []any{workUUID, value.Name, normalizedKey(value.Name), value.SortName, value.Slug, value.MetadataRevision, value.UpdatedAt, value.UUID}
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s=?`, aliasTable, aliasColumn), value.UUID); err != nil {
		return err
	}
	return insertAliases(ctx, tx, aliasTable, aliasColumn, value.UUID, value.Aliases)
}
