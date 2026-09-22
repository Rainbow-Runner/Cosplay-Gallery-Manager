package productdb

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/stashapp/stash/internal/portablecatalog"
)

type PortablePreflightIssue struct {
	Code     string
	Severity string
	Count    int
}

type PortableCatalogPreflight struct {
	IdentityCount          int
	IdentityByKind         map[string]int
	IdentityByState        map[string]int
	CoserCount             int
	WorkCount              int
	CharacterCount         int
	TagCount               int
	AccountCount           int
	GalleryCount           int
	IncompleteGalleryCount int
	AssetCount             int
	Assets                 []PortableAssetSource
	Issues                 []PortablePreflightIssue
}

func (p PortableCatalogPreflight) WarningCount() int {
	return portableIssueCount(p.Issues, "WARNING")
}

func (p PortableCatalogPreflight) BlockingCount() int {
	return portableIssueCount(p.Issues, "BLOCKING")
}

func portableIssueCount(issues []PortablePreflightIssue, severity string) int {
	total := 0
	for _, issue := range issues {
		if issue.Severity == severity {
			total += issue.Count
		}
	}
	return total
}

// PortableCatalogPreflight performs bounded-memory checks in one read
// transaction. It intentionally returns counts and stable issue codes rather
// than entity names or local paths so callers can safely audit the result.
func (db *Database) PortableCatalogPreflight(ctx context.Context) (PortableCatalogPreflight, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PortableCatalogPreflight{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result := PortableCatalogPreflight{
		IdentityByKind:  map[string]int{},
		IdentityByState: map[string]int{},
		Assets:          []PortableAssetSource{},
		Issues:          []PortablePreflightIssue{},
	}
	rows, err := tx.QueryContext(ctx, `SELECT registry.entity_kind,
		CASE WHEN alias.alias_uuid IS NOT NULL THEN 'ALIAS' WHEN tombstone.uuid IS NOT NULL THEN 'TOMBSTONE' ELSE 'ACTIVE' END,
		COUNT(*) FROM portable_uuid_registry registry
		LEFT JOIN portable_uuid_aliases alias ON alias.alias_uuid=registry.uuid
		LEFT JOIN portable_uuid_tombstones tombstone ON tombstone.uuid=registry.uuid
		GROUP BY registry.entity_kind,2`)
	if err != nil {
		return PortableCatalogPreflight{}, err
	}
	for rows.Next() {
		var kind, state string
		var count int
		if err := rows.Scan(&kind, &state, &count); err != nil {
			rows.Close()
			return PortableCatalogPreflight{}, err
		}
		result.IdentityCount += count
		result.IdentityByKind[kind] += count
		result.IdentityByState[state] += count
	}
	if err := rows.Close(); err != nil {
		return PortableCatalogPreflight{}, err
	}
	if err := rows.Err(); err != nil {
		return PortableCatalogPreflight{}, err
	}
	for table, target := range map[string]*int{
		"cosers": &result.CoserCount, "works": &result.WorkCount, "characters": &result.CharacterCount,
		"tags": &result.TagCount, "coser_social_accounts": &result.AccountCount, "galleries": &result.GalleryCount,
	} {
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(target); err != nil {
			return PortableCatalogPreflight{}, err
		}
	}
	if err := preflightPortableObjectIdentities(ctx, tx, &result); err != nil {
		return PortableCatalogPreflight{}, err
	}
	if err := preflightPortableGalleries(ctx, tx, &result); err != nil {
		return PortableCatalogPreflight{}, err
	}
	if err := preflightPortableAssets(ctx, tx, &result); err != nil {
		return PortableCatalogPreflight{}, err
	}
	if err := tx.Commit(); err != nil {
		return PortableCatalogPreflight{}, err
	}
	sort.Slice(result.Issues, func(i, j int) bool { return result.Issues[i].Code < result.Issues[j].Code })
	return result, nil
}

func (p *PortableCatalogPreflight) addIssue(code, severity string, count int) {
	if count <= 0 {
		return
	}
	for index := range p.Issues {
		if p.Issues[index].Code == code && p.Issues[index].Severity == severity {
			p.Issues[index].Count += count
			return
		}
	}
	p.Issues = append(p.Issues, PortablePreflightIssue{Code: code, Severity: severity, Count: count})
}

func preflightPortableObjectIdentities(ctx context.Context, tx *sql.Tx, result *PortableCatalogPreflight) error {
	checks := []struct {
		kind, table, column string
	}{
		{"COSER", "cosers", "uuid"}, {"WORK", "works", "uuid"}, {"CHARACTER", "characters", "uuid"},
		{"TAG", "tags", "uuid"}, {"SOCIAL_ACCOUNT", "coser_social_accounts", "account_uuid"}, {"GALLERY", "galleries", "set_id"},
	}
	for _, check := range checks {
		query := fmt.Sprintf(`SELECT COUNT(*) FROM portable_uuid_registry registry
			LEFT JOIN portable_uuid_aliases alias ON alias.alias_uuid=registry.uuid
			LEFT JOIN portable_uuid_tombstones tombstone ON tombstone.uuid=registry.uuid
			LEFT JOIN %s object ON object.%s=registry.uuid
			WHERE registry.entity_kind=? AND alias.alias_uuid IS NULL AND tombstone.uuid IS NULL AND object.%s IS NULL`, check.table, check.column, check.column)
		var count int
		if err := tx.QueryRowContext(ctx, query, check.kind).Scan(&count); err != nil {
			return err
		}
		result.addIssue("PORTABLE_ACTIVE_IDENTITY_WITHOUT_OBJECT", "BLOCKING", count)
		reverseQuery := fmt.Sprintf(`SELECT COUNT(*) FROM %s object
			LEFT JOIN portable_uuid_registry registry ON registry.uuid=object.%s
			LEFT JOIN portable_uuid_aliases alias ON alias.alias_uuid=object.%s
			LEFT JOIN portable_uuid_tombstones tombstone ON tombstone.uuid=object.%s
			WHERE registry.uuid IS NULL OR registry.entity_kind<>? OR alias.alias_uuid IS NOT NULL OR tombstone.uuid IS NOT NULL`, check.table, check.column, check.column, check.column)
		if err := tx.QueryRowContext(ctx, reverseQuery, check.kind).Scan(&count); err != nil {
			return err
		}
		result.addIssue("PORTABLE_OBJECT_WITHOUT_ACTIVE_IDENTITY", "BLOCKING", count)
	}
	var brokenAliases int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_uuid_aliases alias
		LEFT JOIN portable_uuid_registry source ON source.uuid=alias.alias_uuid
		LEFT JOIN portable_uuid_registry target ON target.uuid=alias.target_uuid
		LEFT JOIN portable_uuid_tombstones source_tombstone ON source_tombstone.uuid=alias.alias_uuid
		LEFT JOIN portable_uuid_aliases target_alias ON target_alias.alias_uuid=alias.target_uuid
		LEFT JOIN portable_uuid_tombstones target_tombstone ON target_tombstone.uuid=alias.target_uuid
		WHERE source.uuid IS NULL OR target.uuid IS NULL OR source.entity_kind<>alias.entity_kind OR target.entity_kind<>alias.entity_kind
		OR source_tombstone.uuid IS NOT NULL OR target_alias.alias_uuid IS NOT NULL OR target_tombstone.uuid IS NOT NULL`).Scan(&brokenAliases); err != nil {
		return err
	}
	result.addIssue("PORTABLE_IDENTITY_ALIAS_INVALID", "BLOCKING", brokenAliases)
	var brokenTombstones int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_uuid_tombstones tombstone
		LEFT JOIN portable_uuid_registry registry ON registry.uuid=tombstone.uuid
		WHERE registry.uuid IS NULL OR registry.entity_kind<>tombstone.entity_kind`).Scan(&brokenTombstones); err != nil {
		return err
	}
	result.addIssue("PORTABLE_IDENTITY_TOMBSTONE_INVALID", "BLOCKING", brokenTombstones)
	for _, relation := range []struct {
		code, query string
	}{
		{"PORTABLE_CHARACTER_WORK_INVALID", `SELECT COUNT(*) FROM characters character
			LEFT JOIN portable_uuid_registry registry ON registry.uuid=character.work_uuid AND registry.entity_kind='WORK'
			LEFT JOIN portable_uuid_aliases alias ON alias.alias_uuid=character.work_uuid
			LEFT JOIN portable_uuid_tombstones tombstone ON tombstone.uuid=character.work_uuid
			WHERE registry.uuid IS NULL OR alias.alias_uuid IS NOT NULL OR tombstone.uuid IS NOT NULL`},
		{"PORTABLE_SOCIAL_ACCOUNT_COSER_INVALID", `SELECT COUNT(*) FROM coser_social_accounts account
			LEFT JOIN portable_uuid_registry registry ON registry.uuid=account.coser_uuid AND registry.entity_kind='COSER'
			LEFT JOIN portable_uuid_aliases alias ON alias.alias_uuid=account.coser_uuid
			LEFT JOIN portable_uuid_tombstones tombstone ON tombstone.uuid=account.coser_uuid
			WHERE registry.uuid IS NULL OR alias.alias_uuid IS NOT NULL OR tombstone.uuid IS NOT NULL`},
		{"PORTABLE_TAG_EDGE_INVALID", `SELECT COUNT(*) FROM tag_edges edge
			LEFT JOIN portable_uuid_registry parent ON parent.uuid=edge.parent_uuid AND parent.entity_kind='TAG'
			LEFT JOIN portable_uuid_registry child ON child.uuid=edge.child_uuid AND child.entity_kind='TAG'
			LEFT JOIN portable_uuid_aliases parent_alias ON parent_alias.alias_uuid=edge.parent_uuid
			LEFT JOIN portable_uuid_aliases child_alias ON child_alias.alias_uuid=edge.child_uuid
			LEFT JOIN portable_uuid_tombstones parent_tombstone ON parent_tombstone.uuid=edge.parent_uuid
			LEFT JOIN portable_uuid_tombstones child_tombstone ON child_tombstone.uuid=edge.child_uuid
			WHERE parent.uuid IS NULL OR child.uuid IS NULL OR parent_alias.alias_uuid IS NOT NULL OR child_alias.alias_uuid IS NOT NULL
			OR parent_tombstone.uuid IS NOT NULL OR child_tombstone.uuid IS NOT NULL OR edge.position<=0`},
		{"PORTABLE_SLUG_REDIRECT_INVALID", `SELECT COUNT(*) FROM slug_redirects redirect
			LEFT JOIN portable_uuid_registry registry ON registry.uuid=redirect.target_uuid AND registry.entity_kind=redirect.entity_kind
			LEFT JOIN portable_uuid_aliases alias ON alias.alias_uuid=redirect.target_uuid
			LEFT JOIN portable_uuid_tombstones tombstone ON tombstone.uuid=redirect.target_uuid
			WHERE redirect.entity_kind IN ('COSER','WORK','CHARACTER','TAG')
			AND (registry.uuid IS NULL OR alias.alias_uuid IS NOT NULL OR tombstone.uuid IS NOT NULL)`},
		{"PORTABLE_TAG_GRAPH_CYCLE", `WITH RECURSIVE reach(start_uuid,node_uuid) AS (
			SELECT parent_uuid,child_uuid FROM tag_edges
			UNION SELECT reach.start_uuid,edge.child_uuid FROM reach JOIN tag_edges edge ON edge.parent_uuid=reach.node_uuid
		) SELECT COUNT(DISTINCT start_uuid) FROM reach WHERE start_uuid=node_uuid`},
	} {
		var count int
		if err := tx.QueryRowContext(ctx, relation.query).Scan(&count); err != nil {
			return err
		}
		result.addIssue(relation.code, "BLOCKING", count)
	}
	return nil
}

func preflightPortableGalleries(ctx context.Context, tx *sql.Tx, result *PortableCatalogPreflight) error {
	libraryRoots := map[int64]string{}
	rows, err := tx.QueryContext(ctx, `SELECT id,root_path FROM media_libraries`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var root string
		if err := rows.Scan(&id, &root); err != nil {
			rows.Close()
			return err
		}
		libraryRoots[id] = root
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, `SELECT source.library_id,COALESCE(source.source_path,''),COALESCE(sync.status,'MISSING')
		FROM galleries gallery LEFT JOIN gallery_sources source ON source.gallery_id=gallery.id
		LEFT JOIN gallery_manifest_sync sync ON sync.gallery_id=gallery.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var libraryID sql.NullInt64
		var sourcePath, manifestStatus string
		if err := rows.Scan(&libraryID, &sourcePath, &manifestStatus); err != nil {
			return err
		}
		incomplete := false
		if manifestStatus != "CLEAN" {
			result.addIssue("PORTABLE_GALLERY_MANIFEST_NOT_CLEAN", "WARNING", 1)
			incomplete = true
		}
		if !libraryID.Valid {
			result.addIssue("PORTABLE_GALLERY_SOURCE_UNBOUND", "WARNING", 1)
			incomplete = true
		} else if _, ok := portableRelativeSource(libraryRoots[libraryID.Int64], sourcePath); !ok {
			result.addIssue("PORTABLE_GALLERY_SOURCE_OUTSIDE_LIBRARY", "WARNING", 1)
			incomplete = true
		}
		if incomplete {
			result.IncompleteGalleryCount++
		}
	}
	return rows.Err()
}

func preflightPortableAssets(ctx context.Context, tx *sql.Tx, result *PortableCatalogPreflight) error {
	rows, err := tx.QueryContext(ctx, `SELECT uuid,avatar_path,banner_path FROM cosers WHERE avatar_path<>'' OR banner_path<>'' ORDER BY uuid`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var uuid, avatar, banner string
		if err := rows.Scan(&uuid, &avatar, &banner); err != nil {
			return err
		}
		for _, value := range []struct{ relative, kind string }{{avatar, "AVATAR"}, {banner, "BANNER"}} {
			if value.relative == "" {
				continue
			}
			var ref *portablecatalog.AssetRef
			temporary := PortableCatalogSnapshot{}
			if err := addPortableAsset(uuid, value.relative, value.kind, &ref, &temporary); err != nil {
				result.addIssue("PORTABLE_COSER_ASSET_REFERENCE_INVALID", "BLOCKING", 1)
				continue
			}
			result.Assets = append(result.Assets, temporary.Assets[0])
			result.AssetCount++
		}
	}
	return rows.Err()
}
