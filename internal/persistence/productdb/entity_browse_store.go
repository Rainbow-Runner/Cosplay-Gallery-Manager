package productdb

import (
	"context"
	"errors"
	"math"

	"github.com/stashapp/stash/internal/browse"
)

func (s *BrowseStore) EntityIndex(ctx context.Context, kind browse.SearchEntityKind, scope browse.Scope, page int, sortBy browse.EntitySort) (browse.EntityPage, error) {
	if page < 1 || page > 1_000_000 {
		return browse.EntityPage{}, errors.New("entity page is out of range")
	}
	config, err := entityBrowseConfiguration(kind)
	if err != nil {
		return browse.EntityPage{}, err
	}
	if sortBy == "" {
		sortBy = browse.EntitySortName
	}
	if sortBy != browse.EntitySortName && (sortBy != browse.EntitySortRecentlyAdded || kind != browse.SearchCoser) {
		return browse.EntityPage{}, errors.New("unsupported entity index sort")
	}
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return browse.EntityPage{}, err
	}
	visibility := config.visiblePrefix + browseVisibleGalleryPredicate + scopeSQL + `)`
	var args []any
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+config.table+` entity WHERE `+visibility, args...).Scan(&total); err != nil {
		return browse.EntityPage{}, err
	}
	order := `COALESCE(NULLIF(entity.sort_name,''),entity.name) COLLATE NOCASE,entity.name COLLATE NOCASE,entity.uuid`
	queryArgs := append([]any{}, args...)
	if sortBy == browse.EntitySortRecentlyAdded {
		recentScope := ""
		if scopeArg != "" {
			recentScope = ` AND gallery_recent.content_rating=?`
			queryArgs = append(queryArgs, scopeArg)
		}
		order = `(SELECT MAX(gallery_recent.added_at_utc) FROM gallery_credits recent_relation
			JOIN galleries gallery_recent ON gallery_recent.id=recent_relation.gallery_id
			JOIN gallery_sources recent_source ON recent_source.gallery_id=gallery_recent.id
			LEFT JOIN gallery_personal_states recent_personal ON recent_personal.gallery_id=gallery_recent.id
			WHERE recent_relation.coser_uuid=entity.uuid AND gallery_recent.state='ACTIVE' AND gallery_recent.added_at_utc IS NOT NULL
			AND recent_source.availability_state='AVAILABLE' AND recent_source.over_limit=0 AND COALESCE(recent_personal.hidden,0)=0
			AND NOT EXISTS(SELECT 1 FROM gallery_source_issues issue WHERE issue.source_id=recent_source.id
				AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL)` + recentScope + `) DESC,entity.uuid`
	}
	queryArgs = append(queryArgs, config.pageSize, (page-1)*config.pageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT entity.uuid,entity.slug,entity.name FROM `+config.table+` entity
		WHERE `+visibility+` ORDER BY `+order+` LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return browse.EntityPage{}, err
	}
	var result browse.EntityPage
	for rows.Next() {
		item := browse.EntityIndexItem{Kind: kind}
		if err := rows.Scan(&item.UUID, &item.Slug, &item.Name); err != nil {
			return browse.EntityPage{}, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return browse.EntityPage{}, err
	}
	if err := rows.Close(); err != nil {
		return browse.EntityPage{}, err
	}
	for index := range result.Items {
		result.Items[index].Aliases, err = s.entityAliases(ctx, config, result.Items[index].UUID)
		if err != nil {
			return browse.EntityPage{}, err
		}
		if kind == browse.SearchCoser {
			var available int
			if err := s.db.QueryRowContext(ctx, `SELECT avatar_path<>'',metadata_revision FROM cosers WHERE uuid=?`, result.Items[index].UUID).
				Scan(&available, &result.Items[index].AssetRevision); err != nil {
				return browse.EntityPage{}, err
			}
			result.Items[index].AvatarAvailable = available == 1
		}
	}
	result.Page, result.PageSize, result.TotalItems = page, config.pageSize, total
	result.TotalPages = int(math.Ceil(float64(total) / float64(config.pageSize)))
	return result, nil
}

type entityBrowseConfig struct {
	table         string
	aliasTable    string
	aliasKey      string
	visiblePrefix string
	pageSize      int
}

func entityBrowseConfiguration(kind browse.SearchEntityKind) (entityBrowseConfig, error) {
	switch kind {
	case browse.SearchCoser:
		return entityBrowseConfig{"cosers", "coser_aliases", "coser_uuid", `EXISTS(SELECT 1 FROM gallery_credits relation JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE relation.coser_uuid=entity.uuid AND `, 30}, nil
	case browse.SearchWork:
		return entityBrowseConfig{"works", "work_aliases", "work_uuid", `EXISTS(SELECT 1 FROM characters character JOIN gallery_cast relation ON relation.character_uuid=character.uuid JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE character.work_uuid=entity.uuid AND `, 60}, nil
	case browse.SearchCharacter:
		return entityBrowseConfig{"characters", "character_aliases", "character_uuid", `EXISTS(SELECT 1 FROM gallery_cast relation JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE relation.character_uuid=entity.uuid AND `, 60}, nil
	case browse.SearchTag:
		return entityBrowseConfig{"tags", "tag_aliases", "tag_uuid", `EXISTS(SELECT 1 FROM gallery_tags relation JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE relation.tag_uuid=entity.uuid AND `, 60}, nil
	default:
		return entityBrowseConfig{}, errors.New("unsupported entity index kind")
	}
}

func (s *BrowseStore) entityAliases(ctx context.Context, config entityBrowseConfig, uuid string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT alias FROM `+config.aliasTable+` WHERE `+config.aliasKey+`=? ORDER BY position`, uuid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
