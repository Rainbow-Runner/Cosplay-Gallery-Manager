package productdb

import (
	"context"
	"errors"
	"strings"

	"github.com/stashapp/stash/internal/browse"
)

// SearchPreview searches only Gallery and core entity names/sort names/Alias.
// Paths, filenames, Item Caption, descriptions, biographies and links never
// participate. Each entity group is independently capped at five results.
func (s *BrowseStore) SearchPreview(ctx context.Context, scope browse.Scope, query string) (browse.SearchPreview, error) {
	query = normalizedDisplay(strings.TrimSpace(query))
	if query == "" || len([]rune(query)) > 300 {
		return browse.SearchPreview{}, errors.New("search query must contain 1 to 300 characters")
	}
	result := browse.SearchPreview{Scope: scope, Query: query}
	var err error
	if result.Galleries, err = s.searchGalleries(ctx, scope, query); err != nil {
		return browse.SearchPreview{}, err
	}
	configs := []struct {
		target *[]browse.SearchHit
		value  coreSearchConfig
	}{
		{&result.Cosers, coreSearchConfig{browse.SearchCoser, "cosers", "coser_aliases", "coser_uuid", `EXISTS(SELECT 1 FROM gallery_credits relation JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE relation.coser_uuid=entity.uuid AND `}},
		{&result.Works, coreSearchConfig{browse.SearchWork, "works", "work_aliases", "work_uuid", `EXISTS(SELECT 1 FROM characters character JOIN gallery_cast relation ON relation.character_uuid=character.uuid JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE character.work_uuid=entity.uuid AND `}},
		{&result.Characters, coreSearchConfig{browse.SearchCharacter, "characters", "character_aliases", "character_uuid", `EXISTS(SELECT 1 FROM gallery_cast relation JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE relation.character_uuid=entity.uuid AND `}},
		{&result.Tags, coreSearchConfig{browse.SearchTag, "tags", "tag_aliases", "tag_uuid", `EXISTS(SELECT 1 FROM gallery_tags relation JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE relation.tag_uuid=entity.uuid AND `}},
	}
	for _, config := range configs {
		*config.target, err = s.searchCoreEntity(ctx, scope, query, config.value)
		if err != nil {
			return browse.SearchPreview{}, err
		}
	}
	return result, nil
}

func (s *BrowseStore) searchGalleries(ctx context.Context, scope browse.Scope, query string) ([]browse.SearchHit, error) {
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return nil, err
	}
	prefix, contains := literalLike(query)+"%", "%"+literalLike(query)+"%"
	args := []any{}
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	args = append(args, query, prefix, contains, query, prefix, contains, contains)
	rows, err := s.db.QueryContext(ctx, `WITH visible AS (
		SELECT gallery.id,gallery.set_id,gallery.slug,gallery.title,gallery.added_at_utc FROM galleries gallery
		JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE `+browseVisibleGalleryPredicate+scopeSQL+`), matches AS (
		SELECT id,CASE WHEN lower(title)=lower(?) THEN 1 WHEN title LIKE ? ESCAPE '\' THEN 3 ELSE 5 END rank
		FROM visible WHERE title LIKE ? ESCAPE '\'
		UNION ALL SELECT visible.id,CASE WHEN lower(alias.alias)=lower(?) THEN 2 WHEN alias.alias LIKE ? ESCAPE '\' THEN 4 ELSE 6 END
		FROM visible JOIN gallery_aliases alias ON alias.gallery_id=visible.id WHERE alias.alias LIKE ? ESCAPE '\'
		UNION ALL SELECT visible.id,7 FROM visible WHERE EXISTS(SELECT 1 FROM (
			SELECT coser.name value FROM gallery_credits credit JOIN cosers coser ON coser.uuid=credit.coser_uuid WHERE credit.gallery_id=visible.id
			UNION ALL SELECT alias.alias FROM gallery_credits credit JOIN coser_aliases alias ON alias.coser_uuid=credit.coser_uuid WHERE credit.gallery_id=visible.id
			UNION ALL SELECT character.name FROM gallery_cast cast_item JOIN characters character ON character.uuid=cast_item.character_uuid WHERE cast_item.gallery_id=visible.id
			UNION ALL SELECT alias.alias FROM gallery_cast cast_item JOIN character_aliases alias ON alias.character_uuid=cast_item.character_uuid WHERE cast_item.gallery_id=visible.id
			UNION ALL SELECT work.name FROM gallery_cast cast_item JOIN characters character ON character.uuid=cast_item.character_uuid JOIN works work ON work.uuid=character.work_uuid WHERE cast_item.gallery_id=visible.id
			UNION ALL SELECT alias.alias FROM gallery_cast cast_item JOIN characters character ON character.uuid=cast_item.character_uuid JOIN work_aliases alias ON alias.work_uuid=character.work_uuid WHERE cast_item.gallery_id=visible.id
			UNION ALL SELECT tag.name FROM gallery_tags relation JOIN tags tag ON tag.uuid=relation.tag_uuid WHERE relation.gallery_id=visible.id
			UNION ALL SELECT alias.alias FROM gallery_tags relation JOIN tag_aliases alias ON alias.tag_uuid=relation.tag_uuid WHERE relation.gallery_id=visible.id
		) related WHERE related.value LIKE ? ESCAPE '\'))
		SELECT visible.set_id,visible.slug,visible.title,MIN(matches.rank) rank FROM matches JOIN visible ON visible.id=matches.id
		GROUP BY visible.id ORDER BY rank,visible.added_at_utc DESC,visible.id DESC LIMIT 5`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []browse.SearchHit
	for rows.Next() {
		value := browse.SearchHit{Kind: browse.SearchGallery}
		if err := rows.Scan(&value.UUID, &value.Slug, &value.Name, &value.MatchLevel); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

type coreSearchConfig struct {
	kind          browse.SearchEntityKind
	table         string
	aliasTable    string
	aliasKey      string
	visiblePrefix string
}

func (s *BrowseStore) searchCoreEntity(ctx context.Context, scope browse.Scope, query string, config coreSearchConfig) ([]browse.SearchHit, error) {
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return nil, err
	}
	prefix, contains := literalLike(query)+"%", "%"+literalLike(query)+"%"
	args := []any{query, prefix, contains}
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	statement := `WITH names AS (
		SELECT uuid,name value,0 is_alias FROM ` + config.table + ` UNION ALL
		SELECT uuid,sort_name,0 FROM ` + config.table + ` WHERE sort_name<>'' UNION ALL
		SELECT ` + config.aliasKey + `,alias,1 FROM ` + config.aliasTable + `), matches AS (
		SELECT uuid,MIN(CASE WHEN lower(value)=lower(?) THEN 1+is_alias WHEN value LIKE ? ESCAPE '\' THEN 3+is_alias ELSE 5+is_alias END) rank
		FROM names WHERE value LIKE ? ESCAPE '\' GROUP BY uuid)
		SELECT entity.uuid,entity.slug,entity.name,matches.rank FROM matches JOIN ` + config.table + ` entity ON entity.uuid=matches.uuid
		WHERE ` + config.visiblePrefix + browseVisibleGalleryPredicate + scopeSQL + `)
		ORDER BY matches.rank,entity.name COLLATE NOCASE,entity.uuid LIMIT 5`
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []browse.SearchHit
	for rows.Next() {
		value := browse.SearchHit{Kind: config.kind}
		if err := rows.Scan(&value.UUID, &value.Slug, &value.Name, &value.MatchLevel); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func literalLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}
