package productdb

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/stashapp/stash/internal/coreentity"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
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

type ManageCoreEntityNameConflict struct {
	Entity           ManageCoreEntity
	MatchedValues    []string
	GalleryCount     int
	WorkName         string
	PrimaryNameMatch bool
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

// ManageCoreEntityNameConflicts returns normalized exact primary-name and
// Alias matches for Work and Character creation review. Work names remain
// non-unique. Character results intentionally span Works so the owner can
// decide whether an existing identity should be reused; the database remains
// authoritative for the same-Work primary-name uniqueness constraint.
func (s *CoreEntityStore) ManageCoreEntityNameConflicts(ctx context.Context, kind, name string, limit int) ([]ManageCoreEntityNameConflict, error) {
	if kind != "WORK" && kind != "CHARACTER" {
		return nil, errors.New("name conflict review only supports Work and Character")
	}
	wanted := normalizedKey(name)
	if wanted == "" || runeLength(name) > 300 || limit < 1 || limit > 20 {
		return nil, errors.New("invalid core entity name conflict check")
	}
	table, _, err := manageEntityTable(kind)
	if err != nil {
		return nil, err
	}
	aliasTable, aliasKey, err := manageEntityAliasTable(kind)
	if err != nil {
		return nil, err
	}
	primaryWhere := ""
	queryArguments := []any{wanted}
	if kind == "CHARACTER" {
		primaryWhere = " WHERE entity.normalized_name=?"
		queryArguments = []any{wanted, wanted}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT uuid,name,alias,primary_match FROM (
		SELECT entity.uuid,entity.name,NULL alias,1 primary_match,COALESCE(NULLIF(entity.sort_name,''),entity.name) sort_value,0 position
		FROM `+table+` entity`+primaryWhere+`
		UNION ALL
		SELECT entity.uuid,entity.name,alias.alias,0,COALESCE(NULLIF(entity.sort_name,''),entity.name),alias.position
		FROM `+aliasTable+` alias JOIN `+table+` entity ON entity.uuid=alias.`+aliasKey+` WHERE alias.normalized_alias=?
	) exact_matches ORDER BY sort_value,uuid,primary_match DESC,position`, queryArguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type match struct {
		uuid             string
		values           []string
		primaryNameMatch bool
	}
	var matches []match
	positions := map[string]int{}
	for rows.Next() {
		var uuid, primary string
		var alias sql.NullString
		var primaryMatch int
		if err := rows.Scan(&uuid, &primary, &alias, &primaryMatch); err != nil {
			return nil, err
		}
		primaryMatches := primaryMatch == 1 && normalizedKey(primary) == wanted
		var values []string
		if primaryMatches {
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
		matches[position].primaryNameMatch = matches[position].primaryNameMatch || primaryMatches
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
	result := make([]ManageCoreEntityNameConflict, 0, len(matches))
	for _, candidate := range matches {
		entity, err := s.ManageFind(ctx, kind, candidate.uuid)
		if err != nil {
			return nil, err
		}
		conflict := ManageCoreEntityNameConflict{Entity: entity, MatchedValues: candidate.values, PrimaryNameMatch: candidate.primaryNameMatch}
		if kind == "WORK" {
			err = s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT relation.gallery_id)
				FROM gallery_cast relation JOIN characters character ON character.uuid=relation.character_uuid
				WHERE character.work_uuid=?`, candidate.uuid).Scan(&conflict.GalleryCount)
		} else {
			err = s.db.QueryRowContext(ctx, `SELECT work.name,(SELECT COUNT(DISTINCT gallery_id) FROM gallery_cast WHERE character_uuid=character.uuid)
				FROM characters character JOIN works work ON work.uuid=character.work_uuid WHERE character.uuid=?`, candidate.uuid).Scan(&conflict.WorkName, &conflict.GalleryCount)
		}
		if err != nil {
			return nil, err
		}
		result = append(result, conflict)
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

type ManageCoreEntityPageOptions struct {
	Page             int
	PageSize         int
	Query            string
	CoserAssetFilter string
}

func (s *CoreEntityStore) ManagePage(ctx context.Context, kind string, page int) (ManageCoreEntityPage, error) {
	return s.ManagePageWithOptions(ctx, kind, ManageCoreEntityPageOptions{Page: page})
}

func (s *CoreEntityStore) ManagePageWithOptions(ctx context.Context, kind string, options ManageCoreEntityPageOptions) (ManageCoreEntityPage, error) {
	table, defaultSize, err := manageEntityTable(kind)
	if err != nil {
		return ManageCoreEntityPage{}, err
	}
	if options.Page < 1 || options.Page > 1_000_000 {
		return ManageCoreEntityPage{}, errors.New("core entity page is out of range")
	}
	pageSize := options.PageSize
	if pageSize == 0 {
		pageSize = defaultSize
	}
	if pageSize != 30 && pageSize != 60 && pageSize != 100 {
		return ManageCoreEntityPage{}, errors.New("core entity page size is unsupported")
	}
	query := normalizedDisplay(strings.TrimSpace(options.Query))
	if len([]rune(query)) > 300 {
		return ManageCoreEntityPage{}, errors.New("core entity query must contain at most 300 characters")
	}
	assetFilter := options.CoserAssetFilter
	if assetFilter == "" {
		assetFilter = "ALL"
	}
	if kind != "COSER" && assetFilter != "ALL" {
		return ManageCoreEntityPage{}, errors.New("Coser asset filter requires Coser kind")
	}
	where := "1=1"
	var args []any
	if query != "" {
		aliasTable, aliasKey, aliasErr := manageEntityAliasTable(kind)
		if aliasErr != nil {
			return ManageCoreEntityPage{}, aliasErr
		}
		contains := "%" + literalLike(query) + "%"
		where += ` AND (entity.name LIKE ? ESCAPE '\' OR entity.sort_name LIKE ? ESCAPE '\' OR EXISTS (
			SELECT 1 FROM ` + aliasTable + ` alias WHERE alias.` + aliasKey + `=entity.uuid AND alias.alias LIKE ? ESCAPE '\'))`
		args = append(args, contains, contains, contains)
	}
	if kind == "COSER" {
		switch assetFilter {
		case "ALL":
		case "MISSING_AVATAR":
			where += " AND entity.avatar_path=''"
		case "MISSING_BANNER":
			where += " AND entity.banner_path=''"
		case "INCOMPLETE":
			where += " AND (entity.avatar_path='' OR entity.banner_path='')"
		case "COMPLETE":
			where += " AND entity.avatar_path<>'' AND entity.banner_path<>''"
		default:
			return ManageCoreEntityPage{}, errors.New("unsupported Coser asset filter")
		}
	}
	result := ManageCoreEntityPage{Page: options.Page, PageSize: pageSize}
	type orderedEntity struct {
		uuid     string
		name     string
		sortName string
	}
	rows, err := s.db.QueryContext(ctx, `SELECT entity.uuid,entity.name,entity.sort_name FROM `+table+` entity WHERE `+where, args...)
	if err != nil {
		return ManageCoreEntityPage{}, err
	}
	var candidates []orderedEntity
	for rows.Next() {
		var candidate orderedEntity
		if err := rows.Scan(&candidate.uuid, &candidate.name, &candidate.sortName); err != nil {
			rows.Close()
			return ManageCoreEntityPage{}, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ManageCoreEntityPage{}, err
	}
	if err := rows.Close(); err != nil {
		return ManageCoreEntityPage{}, err
	}
	pinyin := collate.New(language.SimplifiedChinese, collate.IgnoreCase)
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		leftKey, rightKey := left.sortName, right.sortName
		if leftKey == "" {
			leftKey = left.name
		}
		if rightKey == "" {
			rightKey = right.name
		}
		if comparison := pinyin.CompareString(leftKey, rightKey); comparison != 0 {
			return comparison < 0
		}
		if comparison := pinyin.CompareString(left.name, right.name); comparison != 0 {
			return comparison < 0
		}
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		if left.name != right.name {
			return left.name < right.name
		}
		return left.uuid < right.uuid
	})
	result.TotalItems = len(candidates)
	result.TotalPages = int(math.Ceil(float64(result.TotalItems) / float64(pageSize)))
	start := (options.Page - 1) * pageSize
	if start < len(candidates) {
		end := min(start+pageSize, len(candidates))
		for _, candidate := range candidates[start:end] {
			value, findErr := s.ManageFind(ctx, kind, candidate.uuid)
			if findErr != nil {
				return ManageCoreEntityPage{}, findErr
			}
			result.Items = append(result.Items, value)
		}
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
