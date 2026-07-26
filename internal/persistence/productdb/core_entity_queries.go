package productdb

import (
	"context"
	"database/sql"

	"github.com/stashapp/stash/internal/coreentity"
)

func findCoser(ctx context.Context, queryer galleryQueryer, uuid string) (coreentity.Coser, error) {
	var result coreentity.Coser
	var cropX, cropY, cropSize, focalX, focalY sql.NullFloat64
	var createdAt, updatedAt string
	err := queryer.QueryRowContext(ctx, `
		SELECT uuid, name, sort_name, slug, profile_summary, biography,
			country_or_region, avatar_path, banner_path,
			avatar_crop_x, avatar_crop_y, avatar_crop_size,
			banner_focal_x, banner_focal_y,
			metadata_revision, created_at_utc, updated_at_utc
		FROM cosers WHERE uuid = ?
	`, uuid).Scan(
		&result.UUID, &result.Name, &result.SortName, &result.Slug,
		&result.ProfileSummary, &result.Biography, &result.CountryOrRegion,
		&result.AvatarPath, &result.BannerPath, &cropX, &cropY, &cropSize, &focalX, &focalY,
		&result.MetadataRevision, &createdAt, &updatedAt,
	)
	if err != nil {
		return coreentity.Coser{}, err
	}
	result.CreatedAtUTC, err = parseTime(createdAt)
	if err != nil {
		return coreentity.Coser{}, err
	}
	result.UpdatedAtUTC, err = parseTime(updatedAt)
	if err != nil {
		return coreentity.Coser{}, err
	}
	if cropX.Valid && cropY.Valid && cropSize.Valid {
		result.AvatarCrop = &coreentity.AvatarCrop{X: cropX.Float64, Y: cropY.Float64, Size: cropSize.Float64}
	}
	if focalX.Valid && focalY.Valid {
		result.BannerFocalPoint = &coreentity.FocalPoint{X: focalX.Float64, Y: focalY.Float64}
	}
	result.Aliases, err = loadAliases(ctx, queryer, "coser_aliases", "coser_uuid", uuid)
	return result, err
}

func findWork(ctx context.Context, queryer galleryQueryer, uuid string) (coreentity.Work, error) {
	var result coreentity.Work
	var createdAt, updatedAt string
	err := queryer.QueryRowContext(ctx, `
		SELECT uuid, name, sort_name, slug, metadata_revision, created_at_utc, updated_at_utc
		FROM works WHERE uuid = ?
	`, uuid).Scan(&result.UUID, &result.Name, &result.SortName, &result.Slug,
		&result.MetadataRevision, &createdAt, &updatedAt)
	if err != nil {
		return coreentity.Work{}, err
	}
	result.CreatedAtUTC, err = parseTime(createdAt)
	if err != nil {
		return coreentity.Work{}, err
	}
	result.UpdatedAtUTC, err = parseTime(updatedAt)
	if err != nil {
		return coreentity.Work{}, err
	}
	result.Aliases, err = loadAliases(ctx, queryer, "work_aliases", "work_uuid", uuid)
	return result, err
}

func findCharacter(ctx context.Context, queryer galleryQueryer, uuid string) (coreentity.Character, error) {
	var result coreentity.Character
	var createdAt, updatedAt string
	err := queryer.QueryRowContext(ctx, `
		SELECT uuid, work_uuid, name, sort_name, slug, metadata_revision, created_at_utc, updated_at_utc
		FROM characters WHERE uuid = ?
	`, uuid).Scan(&result.UUID, &result.WorkUUID, &result.Name, &result.SortName,
		&result.Slug, &result.MetadataRevision, &createdAt, &updatedAt)
	if err != nil {
		return coreentity.Character{}, err
	}
	result.CreatedAtUTC, err = parseTime(createdAt)
	if err != nil {
		return coreentity.Character{}, err
	}
	result.UpdatedAtUTC, err = parseTime(updatedAt)
	if err != nil {
		return coreentity.Character{}, err
	}
	result.Aliases, err = loadAliases(ctx, queryer, "character_aliases", "character_uuid", uuid)
	return result, err
}

func findTag(ctx context.Context, queryer galleryQueryer, uuid string) (coreentity.Tag, error) {
	var result coreentity.Tag
	var recommendation int
	var createdAt, updatedAt string
	err := queryer.QueryRowContext(ctx, `
		SELECT uuid, name, sort_name, slug, use_in_recommendation,
			metadata_revision, created_at_utc, updated_at_utc
		FROM tags WHERE uuid = ?
	`, uuid).Scan(&result.UUID, &result.Name, &result.SortName, &result.Slug,
		&recommendation, &result.MetadataRevision, &createdAt, &updatedAt)
	if err != nil {
		return coreentity.Tag{}, err
	}
	result.UseInRecommendation = recommendation == 1
	result.CreatedAtUTC, err = parseTime(createdAt)
	if err != nil {
		return coreentity.Tag{}, err
	}
	result.UpdatedAtUTC, err = parseTime(updatedAt)
	if err != nil {
		return coreentity.Tag{}, err
	}
	result.Aliases, err = loadAliases(ctx, queryer, "tag_aliases", "tag_uuid", uuid)
	return result, err
}

type rowsQueryer interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

func loadAliases(ctx context.Context, queryer galleryQueryer, table string, ownerColumn string, uuid string) ([]string, error) {
	rowsQueryer, ok := queryer.(rowsQueryer)
	if !ok {
		return nil, nil
	}
	query := `SELECT alias FROM ` + table + ` WHERE ` + ownerColumn + ` = ? ORDER BY position`
	rows, err := rowsQueryer.QueryContext(ctx, query, uuid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var aliases []string
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		aliases = append(aliases, alias)
	}
	return aliases, rows.Err()
}
