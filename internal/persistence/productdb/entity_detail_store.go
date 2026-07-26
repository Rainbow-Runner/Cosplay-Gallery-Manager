package productdb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/portableid"
)

func (s *BrowseStore) CoserDetail(ctx context.Context, value string, scope browse.Scope, page int) (browse.CoserDetail, error) {
	uuid, redirected, err := s.resolveEntityRoute(ctx, portableid.KindCoser, value)
	if err != nil {
		return browse.CoserDetail{}, err
	}
	var result browse.CoserDetail
	result.Redirected = redirected
	result.Entity.Kind = browse.SearchCoser
	var avatarAvailable, bannerAvailable int
	if err := s.db.QueryRowContext(ctx, `SELECT uuid,slug,name,profile_summary,biography,country_or_region,
		avatar_path<>'',banner_path<>'',metadata_revision FROM cosers WHERE uuid=?`, uuid).Scan(
		&result.Entity.UUID, &result.Entity.Slug, &result.Entity.Name, &result.ProfileSummary, &result.Biography, &result.CountryOrRegion,
		&avatarAvailable, &bannerAvailable, &result.Entity.AssetRevision); err != nil {
		return browse.CoserDetail{}, err
	}
	result.Entity.AvatarAvailable = avatarAvailable == 1
	result.BannerAvailable = bannerAvailable == 1
	config, _ := entityBrowseConfiguration(browse.SearchCoser)
	result.Entity.Aliases, err = s.entityAliases(ctx, config, uuid)
	if err != nil {
		return browse.CoserDetail{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT account_uuid,platform_key,label,handle,url,status,position FROM coser_social_accounts
		WHERE coser_uuid=? AND visible=1 ORDER BY position,account_uuid`, uuid)
	if err != nil {
		return browse.CoserDetail{}, err
	}
	for rows.Next() {
		var account browse.SocialAccount
		if err := rows.Scan(&account.UUID, &account.PlatformKey, &account.Label, &account.Handle, &account.URL, &account.Status, &account.Position); err != nil {
			rows.Close()
			return browse.CoserDetail{}, err
		}
		result.SocialAccounts = append(result.SocialAccounts, account)
	}
	if err := rows.Close(); err != nil {
		return browse.CoserDetail{}, err
	}
	result.Galleries, err = s.galleryPage(ctx, scope, page, browse.GallerySortRecentlyAdded,
		` AND EXISTS(SELECT 1 FROM gallery_credits detail_credit WHERE detail_credit.gallery_id=gallery.id AND detail_credit.coser_uuid=?)`, []any{uuid}, "")
	return result, err
}

func (s *BrowseStore) WorkDetail(ctx context.Context, value string, scope browse.Scope) (browse.WorkDetail, error) {
	uuid, redirected, err := s.resolveEntityRoute(ctx, portableid.KindWork, value)
	if err != nil {
		return browse.WorkDetail{}, err
	}
	result := browse.WorkDetail{Redirected: redirected, Entity: browse.EntityIndexItem{Kind: browse.SearchWork}}
	if err := s.db.QueryRowContext(ctx, `SELECT uuid,slug,name FROM works WHERE uuid=?`, uuid).Scan(&result.Entity.UUID, &result.Entity.Slug, &result.Entity.Name); err != nil {
		return browse.WorkDetail{}, err
	}
	config, _ := entityBrowseConfiguration(browse.SearchWork)
	result.Entity.Aliases, err = s.entityAliases(ctx, config, uuid)
	if err != nil {
		return browse.WorkDetail{}, err
	}
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return browse.WorkDetail{}, err
	}
	args := []any{uuid}
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT character.uuid,character.slug,character.name FROM characters character WHERE character.work_uuid=?
		AND EXISTS(SELECT 1 FROM gallery_cast relation JOIN galleries gallery ON gallery.id=relation.gallery_id
		JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE relation.character_uuid=character.uuid AND `+browseVisibleGalleryPredicate+scopeSQL+`)
		ORDER BY COALESCE(NULLIF(character.sort_name,''),character.name) COLLATE NOCASE,character.uuid`, args...)
	if err != nil {
		return browse.WorkDetail{}, err
	}
	defer rows.Close()
	for rows.Next() {
		item := browse.EntityIndexItem{Kind: browse.SearchCharacter}
		if err := rows.Scan(&item.UUID, &item.Slug, &item.Name); err != nil {
			return browse.WorkDetail{}, err
		}
		result.Characters = append(result.Characters, item)
	}
	return result, rows.Err()
}

func (s *BrowseStore) CharacterDetail(ctx context.Context, value string, scope browse.Scope, page int) (browse.CharacterDetail, error) {
	uuid, redirected, err := s.resolveEntityRoute(ctx, portableid.KindCharacter, value)
	if err != nil {
		return browse.CharacterDetail{}, err
	}
	result := browse.CharacterDetail{Redirected: redirected, Entity: browse.EntityIndexItem{Kind: browse.SearchCharacter}, Work: browse.EntityIndexItem{Kind: browse.SearchWork}}
	if err := s.db.QueryRowContext(ctx, `SELECT character.uuid,character.slug,character.name,work.uuid,work.slug,work.name FROM characters character
		JOIN works work ON work.uuid=character.work_uuid WHERE character.uuid=?`, uuid).Scan(&result.Entity.UUID, &result.Entity.Slug, &result.Entity.Name, &result.Work.UUID, &result.Work.Slug, &result.Work.Name); err != nil {
		return browse.CharacterDetail{}, err
	}
	result.Galleries, err = s.galleryPage(ctx, scope, page, browse.GallerySortRecentlyAdded, ` AND EXISTS(SELECT 1 FROM gallery_cast detail_cast WHERE detail_cast.gallery_id=gallery.id AND detail_cast.character_uuid=?)`, []any{uuid}, "")
	return result, err
}

func (s *BrowseStore) TagDetail(ctx context.Context, value string, scope browse.Scope, page int) (browse.TagDetail, error) {
	uuid, redirected, err := s.resolveEntityRoute(ctx, portableid.KindTag, value)
	if err != nil {
		return browse.TagDetail{}, err
	}
	result := browse.TagDetail{Redirected: redirected, Entity: browse.EntityIndexItem{Kind: browse.SearchTag}}
	if err := s.db.QueryRowContext(ctx, `SELECT uuid,slug,name FROM tags WHERE uuid=?`, uuid).Scan(&result.Entity.UUID, &result.Entity.Slug, &result.Entity.Name); err != nil {
		return browse.TagDetail{}, err
	}
	result.Galleries, err = s.galleryPage(ctx, scope, page, browse.GallerySortRecentlyAdded, ` AND EXISTS(
		WITH RECURSIVE descendants(uuid) AS (SELECT ? UNION SELECT edge.child_uuid FROM tag_edges edge JOIN descendants parent ON edge.parent_uuid=parent.uuid)
		SELECT 1 FROM gallery_tags relation JOIN descendants ON descendants.uuid=relation.tag_uuid WHERE relation.gallery_id=gallery.id)`, []any{uuid}, "")
	return result, err
}

func (s *BrowseStore) resolveEntityRoute(ctx context.Context, kind portableid.Kind, value string) (string, bool, error) {
	uuid, redirected, err := (&CoreEntityStore{db: s.db}).ResolveSlug(ctx, kind, value)
	if err == nil {
		return uuid, redirected, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	if _, parseErr := portableid.Parse(value); parseErr != nil {
		return "", false, err
	}
	table, tableErr := tableForCoreKind(kind)
	if tableErr != nil {
		return "", false, tableErr
	}
	var found string
	if queryErr := s.db.QueryRowContext(ctx, `SELECT uuid FROM `+table.name+` WHERE uuid=?`, value).Scan(&found); queryErr != nil {
		return "", false, queryErr
	}
	return found, true, nil
}
