package productdb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/stashapp/stash/internal/browse"
)

type SearchResultBatch struct {
	Items     []browse.SearchHit
	Total     int
	EndCursor string
	HasNext   bool
}
type searchCursor struct {
	Query  string
	Scope  browse.Scope
	Kind   browse.SearchEntityKind
	Offset int
}

func searchMatchTotal(items []browse.SearchHit) int {
	if len(items) == 0 {
		return 0
	}
	return items[0].TotalMatches
}

// Bounded transport batches support continuous scrolling, not numbered UI pages.
func (s *BrowseStore) SearchResults(ctx context.Context, scope browse.Scope, query string, kind browse.SearchEntityKind, first int, after string) (SearchResultBatch, error) {
	query = normalizedDisplay(strings.TrimSpace(query))
	if query == "" || len([]rune(query)) > 300 || first < 1 || first > 100 {
		return SearchResultBatch{}, errors.New("invalid search request")
	}
	if _, _, err := browseScopePredicate(scope); err != nil {
		return SearchResultBatch{}, err
	}
	cursor := searchCursor{Query: query, Scope: scope, Kind: kind}
	if after != "" {
		if len(after) > 4096 {
			return SearchResultBatch{}, errors.New("invalid search cursor")
		}
		raw, err := base64.RawURLEncoding.DecodeString(after)
		var previous searchCursor
		if err != nil || json.Unmarshal(raw, &previous) != nil || previous.Query != query || previous.Scope != scope || previous.Kind != kind || previous.Offset < 0 || previous.Offset > 10000000 {
			return SearchResultBatch{}, errors.New("invalid search cursor")
		}
		cursor.Offset = previous.Offset
	}
	var items []browse.SearchHit
	var err error
	if kind == browse.SearchGallery {
		items, err = s.searchGalleries(ctx, scope, query, first, cursor.Offset)
	} else {
		config, configErr := searchCoreConfig(kind)
		if configErr != nil {
			return SearchResultBatch{}, configErr
		}
		items, err = s.searchCoreEntity(ctx, scope, query, config, first, cursor.Offset)
	}
	if err != nil {
		return SearchResultBatch{}, err
	}
	total := searchMatchTotal(items)
	cursor.Offset += len(items)
	raw, _ := json.Marshal(cursor)
	return SearchResultBatch{Items: items, Total: total, EndCursor: base64.RawURLEncoding.EncodeToString(raw), HasNext: len(items) > 0 && cursor.Offset < total}, nil
}

func searchCoreConfig(kind browse.SearchEntityKind) (coreSearchConfig, error) {
	var config coreSearchConfig
	switch kind {
	case browse.SearchCoser:
		config = coreSearchConfig{kind, "cosers", "coser_aliases", "coser_uuid", `EXISTS(SELECT 1 FROM gallery_credits relation JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE relation.coser_uuid=entity.uuid AND `}
	case browse.SearchWork:
		config = coreSearchConfig{kind, "works", "work_aliases", "work_uuid", `EXISTS(SELECT 1 FROM characters character JOIN gallery_cast relation ON relation.character_uuid=character.uuid JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE character.work_uuid=entity.uuid AND `}
	case browse.SearchCharacter:
		config = coreSearchConfig{kind, "characters", "character_aliases", "character_uuid", `EXISTS(SELECT 1 FROM gallery_cast relation JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE relation.character_uuid=entity.uuid AND `}
	case browse.SearchTag:
		config = coreSearchConfig{kind, "tags", "tag_aliases", "tag_uuid", `EXISTS(SELECT 1 FROM gallery_tags relation JOIN galleries gallery ON gallery.id=relation.gallery_id JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE relation.tag_uuid=entity.uuid AND `}
	default:
		return coreSearchConfig{}, errors.New("invalid search kind")
	}
	return config, nil
}
