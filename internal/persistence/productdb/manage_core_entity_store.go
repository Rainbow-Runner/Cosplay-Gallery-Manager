package productdb

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"

	"github.com/stashapp/stash/internal/coreentity"
)

type ManageCoreEntity struct {
	Kind                string
	UUID                string
	Name                string
	SortName            string
	Aliases             []string
	Slug                string
	MetadataRevision    int64
	WorkUUID            string
	UseInRecommendation bool
	ProfileSummary      string
	Biography           string
	CountryOrRegion     string
	AvatarPath          string
	BannerPath          string
	AvatarCrop          *coreentity.AvatarCrop
	BannerFocalPoint    *coreentity.FocalPoint
	SocialAccounts      []coreentity.SocialAccount
	Parents             []ManageCoreEntityRef
}

type ManageCoreEntityRef struct {
	UUID             string
	Name             string
	MetadataRevision int64
}

type ManageCoserNameConflict struct {
	Coser         ManageCoreEntity
	MatchedValues []string
	GalleryCount  int
}

// ManageCoserNameConflicts returns normalized exact primary-name and Alias
// matches. Coser names intentionally remain non-unique, so this is a review
// aid rather than a creation constraint.
func (s *CoreEntityStore) ManageCoserNameConflicts(ctx context.Context, name string, limit int) ([]ManageCoserNameConflict, error) {
	wanted := normalizedKey(name)
	if wanted == "" || runeLength(name) > 300 || limit < 1 || limit > 20 {
		return nil, errors.New("invalid Coser name conflict check")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT coser.uuid,coser.name,alias.alias
		FROM cosers coser LEFT JOIN coser_aliases alias ON alias.coser_uuid=coser.uuid
		ORDER BY COALESCE(NULLIF(coser.sort_name,''),coser.name),coser.uuid,alias.position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type match struct {
		uuid   string
		values []string
	}
	var matches []match
	positions := map[string]int{}
	for rows.Next() {
		var uuid, primary string
		var alias sql.NullString
		if err := rows.Scan(&uuid, &primary, &alias); err != nil {
			return nil, err
		}
		var values []string
		if normalizedKey(primary) == wanted {
			values = append(values, primary)
		}
		if alias.Valid && normalizedKey(alias.String) == wanted {
			values = append(values, alias.String)
		}
		if len(values) == 0 {
			continue
		}
		position, found := positions[uuid]
		if !found {
			if len(matches) >= limit {
				continue
			}
			position = len(matches)
			positions[uuid] = position
			matches = append(matches, match{uuid: uuid})
		}
		for _, value := range values {
			duplicate := false
			for _, existing := range matches[position].values {
				duplicate = duplicate || normalizedKey(existing) == normalizedKey(value)
			}
			if !duplicate {
				matches[position].values = append(matches[position].values, value)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]ManageCoserNameConflict, 0, len(matches))
	for _, candidate := range matches {
		coser, err := s.ManageFind(ctx, "COSER", candidate.uuid)
		if err != nil {
			return nil, err
		}
		var galleryCount int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT gallery_id) FROM gallery_credits WHERE coser_uuid=?`, candidate.uuid).Scan(&galleryCount); err != nil {
			return nil, err
		}
		result = append(result, ManageCoserNameConflict{Coser: coser, MatchedValues: candidate.values, GalleryCount: galleryCount})
	}
	return result, nil
}

// ManageOptions searches every entity of a kind, including standalone Cosers
// and entities without a currently browsable Gallery. It is intentionally a
// Manage-only selector and never inherits a Browse scope.
func (s *CoreEntityStore) ManageOptions(ctx context.Context, kind, query string, limit int) ([]ManageCoreEntity, error) {
	table, _, err := manageEntityTable(kind)
	if err != nil {
		return nil, err
	}
	aliasTable, aliasKey, err := manageEntityAliasTable(kind)
	if err != nil {
		return nil, err
	}
	query = normalizedDisplay(strings.TrimSpace(query))
	if len([]rune(query)) > 300 || limit < 1 || limit > 50 {
		return nil, errors.New("invalid core entity option search")
	}
	var rows queryRows
	if query == "" {
		rows, err = s.db.QueryContext(ctx, `SELECT uuid FROM `+table+` ORDER BY COALESCE(NULLIF(sort_name,''),name),uuid LIMIT ?`, limit)
	} else {
		prefix, contains := literalLike(query)+"%", "%"+literalLike(query)+"%"
		rows, err = s.db.QueryContext(ctx, `WITH names AS (
			SELECT uuid,name value,0 is_alias FROM `+table+` UNION ALL
			SELECT uuid,sort_name,0 FROM `+table+` WHERE sort_name<>'' UNION ALL
			SELECT `+aliasKey+`,alias,1 FROM `+aliasTable+`), matches AS (
			SELECT uuid,MIN(CASE WHEN lower(value)=lower(?) THEN 1+is_alias WHEN value LIKE ? ESCAPE '\' THEN 3+is_alias ELSE 5+is_alias END) rank
			FROM names WHERE value LIKE ? ESCAPE '\' GROUP BY uuid)
			SELECT entity.uuid FROM matches JOIN `+table+` entity ON entity.uuid=matches.uuid
			ORDER BY matches.rank,COALESCE(NULLIF(entity.sort_name,''),entity.name),entity.uuid LIMIT ?`, query, prefix, contains, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var uuids []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, err
		}
		uuids = append(uuids, uuid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]ManageCoreEntity, 0, len(uuids))
	for _, uuid := range uuids {
		value, err := s.ManageFind(ctx, kind, uuid)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

type queryRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}

func manageEntityAliasTable(kind string) (string, string, error) {
	switch kind {
	case "COSER":
		return "coser_aliases", "coser_uuid", nil
	case "WORK":
		return "work_aliases", "work_uuid", nil
	case "CHARACTER":
		return "character_aliases", "character_uuid", nil
	case "TAG":
		return "tag_aliases", "tag_uuid", nil
	default:
		return "", "", errors.New("unsupported core entity kind")
	}
}

type ManageCoreEntityPage struct {
	Items      []ManageCoreEntity
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}

func (s *CoreEntityStore) ManagePage(ctx context.Context, kind string, page int) (ManageCoreEntityPage, error) {
	table, size, err := manageEntityTable(kind)
	if err != nil {
		return ManageCoreEntityPage{}, err
	}
	if page < 1 || page > 1_000_000 {
		return ManageCoreEntityPage{}, errors.New("core entity page is out of range")
	}
	result := ManageCoreEntityPage{Page: page, PageSize: size}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&result.TotalItems); err != nil {
		return ManageCoreEntityPage{}, err
	}
	result.TotalPages = int(math.Ceil(float64(result.TotalItems) / float64(size)))
	rows, err := s.db.QueryContext(ctx, `SELECT uuid FROM `+table+` ORDER BY COALESCE(NULLIF(sort_name,''),name),uuid LIMIT ? OFFSET ?`, size, (page-1)*size)
	if err != nil {
		return ManageCoreEntityPage{}, err
	}
	defer rows.Close()
	var uuids []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return ManageCoreEntityPage{}, err
		}
		uuids = append(uuids, uuid)
	}
	if err := rows.Err(); err != nil {
		return ManageCoreEntityPage{}, err
	}
	for _, uuid := range uuids {
		value, err := s.ManageFind(ctx, kind, uuid)
		if err != nil {
			return ManageCoreEntityPage{}, err
		}
		result.Items = append(result.Items, value)
	}
	return result, nil
}

func (s *CoreEntityStore) ManageFind(ctx context.Context, kind, uuid string) (ManageCoreEntity, error) {
	switch kind {
	case "COSER":
		value, err := findCoser(ctx, s.db, uuid)
		if err != nil {
			return ManageCoreEntity{}, err
		}
		result := ManageCoreEntity{Kind: kind, UUID: value.UUID, Name: value.Name, SortName: value.SortName, Aliases: value.Aliases, Slug: value.Slug, MetadataRevision: value.MetadataRevision, ProfileSummary: value.ProfileSummary, Biography: value.Biography, CountryOrRegion: value.CountryOrRegion, AvatarPath: value.AvatarPath, BannerPath: value.BannerPath, AvatarCrop: value.AvatarCrop, BannerFocalPoint: value.BannerFocalPoint}
		accounts, err := s.manageSocialAccounts(ctx, uuid)
		result.SocialAccounts = accounts
		return result, err
	case "WORK":
		value, err := findWork(ctx, s.db, uuid)
		if err != nil {
			return ManageCoreEntity{}, err
		}
		return ManageCoreEntity{Kind: kind, UUID: value.UUID, Name: value.Name, SortName: value.SortName, Aliases: value.Aliases, Slug: value.Slug, MetadataRevision: value.MetadataRevision}, nil
	case "CHARACTER":
		value, err := findCharacter(ctx, s.db, uuid)
		if err != nil {
			return ManageCoreEntity{}, err
		}
		return ManageCoreEntity{Kind: kind, UUID: value.UUID, Name: value.Name, SortName: value.SortName, Aliases: value.Aliases, Slug: value.Slug, MetadataRevision: value.MetadataRevision, WorkUUID: value.WorkUUID}, nil
	case "TAG":
		value, err := findTag(ctx, s.db, uuid)
		if err != nil {
			return ManageCoreEntity{}, err
		}
		result := ManageCoreEntity{Kind: kind, UUID: value.UUID, Name: value.Name, SortName: value.SortName, Aliases: value.Aliases, Slug: value.Slug, MetadataRevision: value.MetadataRevision, UseInRecommendation: value.UseInRecommendation}
		rows, err := s.db.QueryContext(ctx, `SELECT parent.uuid,parent.name,parent.metadata_revision FROM tag_edges edge JOIN tags parent ON parent.uuid=edge.parent_uuid WHERE edge.child_uuid=? ORDER BY edge.position,parent.uuid`, uuid)
		if err != nil {
			return ManageCoreEntity{}, err
		}
		for rows.Next() {
			var parent ManageCoreEntityRef
			if err := rows.Scan(&parent.UUID, &parent.Name, &parent.MetadataRevision); err != nil {
				rows.Close()
				return ManageCoreEntity{}, err
			}
			result.Parents = append(result.Parents, parent)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return ManageCoreEntity{}, err
		}
		if err := rows.Close(); err != nil {
			return ManageCoreEntity{}, err
		}
		return result, nil
	default:
		return ManageCoreEntity{}, errors.New("unsupported core entity kind")
	}
}

func (s *CoreEntityStore) FindCoser(ctx context.Context, uuid string) (coreentity.Coser, error) {
	return findCoser(ctx, s.db, uuid)
}

func (s *CoreEntityStore) manageSocialAccounts(ctx context.Context, coserUUID string) ([]coreentity.SocialAccount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT account_uuid,coser_uuid,platform_key,label,handle,url,status,visible,position FROM coser_social_accounts WHERE coser_uuid=? ORDER BY position,account_uuid`, coserUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []coreentity.SocialAccount
	for rows.Next() {
		var value coreentity.SocialAccount
		var visible int
		if err := rows.Scan(&value.UUID, &value.CoserUUID, &value.PlatformKey, &value.Label, &value.Handle, &value.URL, &value.Status, &visible, &value.Position); err != nil {
			return nil, err
		}
		value.Visible = visible == 1
		values = append(values, value)
	}
	return values, rows.Err()
}

func manageEntityTable(kind string) (string, int, error) {
	switch kind {
	case "COSER":
		return "cosers", 30, nil
	case "WORK":
		return "works", 60, nil
	case "CHARACTER":
		return "characters", 60, nil
	case "TAG":
		return "tags", 60, nil
	default:
		return "", 0, errors.New("unsupported core entity kind")
	}
}
