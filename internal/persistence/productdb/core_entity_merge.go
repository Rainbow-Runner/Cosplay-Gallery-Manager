package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/portableid"
)

var ErrCoreEntityMergeConflict = errors.New("core entity merge has unresolved conflicts")

type CoreEntityMergeConflict struct {
	Code    string
	Details string
}

type CoreEntityMergePreview struct {
	Kind               portableid.Kind
	SourceUUID         string
	TargetUUID         string
	SourceRevision     int64
	TargetRevision     int64
	AffectedGalleryIDs []int64
	Conflicts          []CoreEntityMergeConflict
}

// PreviewMerge performs all conflict checks used by Merge. A merge is allowed
// only when this preview is conflict-free; callers resolve conflicts through
// ordinary edits before retrying, so no relationship is silently discarded.
func (s *CoreEntityStore) PreviewMerge(ctx context.Context, kind portableid.Kind, sourceUUID, targetUUID string) (CoreEntityMergePreview, error) {
	if sourceUUID == targetUUID {
		return CoreEntityMergePreview{}, errors.New("merge source and target must differ")
	}
	table, err := tableForCoreKind(kind)
	if err != nil {
		return CoreEntityMergePreview{}, err
	}
	preview := CoreEntityMergePreview{Kind: kind, SourceUUID: sourceUUID, TargetUUID: targetUUID}
	for uuid, destination := range map[string]*int64{sourceUUID: &preview.SourceRevision, targetUUID: &preview.TargetRevision} {
		if err := s.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT metadata_revision FROM %s WHERE uuid = ?`, table.name), uuid).Scan(destination); err != nil {
			return CoreEntityMergePreview{}, err
		}
		record, err := lookupPortableUUID(ctx, s.db, uuid)
		if err != nil || record.Kind != kind || record.State != PortableUUIDActive {
			if err == nil {
				err = ErrPortableUUIDNotActive
			}
			return CoreEntityMergePreview{}, err
		}
	}
	preview.AffectedGalleryIDs, err = mergeAffectedGalleryIDs(ctx, s.db, kind, sourceUUID)
	if err != nil {
		return CoreEntityMergePreview{}, err
	}
	preview.Conflicts, err = mergeConflicts(ctx, s.db, kind, sourceUUID, targetUUID)
	return preview, err
}

func (s *CoreEntityStore) Merge(ctx context.Context, kind portableid.Kind, sourceUUID, targetUUID string, expectedSourceRevision, expectedTargetRevision int64, now time.Time) (CoreEntityMergePreview, error) {
	preview, err := s.PreviewMerge(ctx, kind, sourceUUID, targetUUID)
	if err != nil {
		return CoreEntityMergePreview{}, err
	}
	if preview.SourceRevision != expectedSourceRevision || preview.TargetRevision != expectedTargetRevision {
		return preview, ErrCoreMetadataRevisionConflict
	}
	if len(preview.Conflicts) > 0 {
		return preview, ErrCoreEntityMergeConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return preview, err
	}
	defer func() { _ = tx.Rollback() }()
	table, _ := tableForCoreKind(kind)
	for uuid, revision := range map[string]int64{sourceUUID: expectedSourceRevision, targetUUID: expectedTargetRevision} {
		var current int64
		if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT metadata_revision FROM %s WHERE uuid = ?`, table.name), uuid).Scan(&current); err != nil {
			return preview, err
		}
		if current != revision {
			return preview, ErrCoreMetadataRevisionConflict
		}
	}
	if err := mergeRelationships(ctx, tx, kind, sourceUUID, targetUUID); err != nil {
		return preview, err
	}
	if err := mergeEntityAliases(ctx, tx, kind, sourceUUID, targetUUID); err != nil {
		return preview, err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET metadata_revision = metadata_revision + 1, updated_at_utc = ? WHERE uuid = ?`, table.name), formatTime(normalisedTime(now)), targetUUID); err != nil {
		return preview, err
	}
	var sourceSlug string
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT slug FROM %s WHERE uuid = ?`, table.name), sourceUUID).Scan(&sourceSlug); err != nil {
		return preview, err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE uuid = ?`, table.name), sourceUUID); err != nil {
		return preview, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO portable_uuid_aliases (alias_uuid, target_uuid, entity_kind, merged_at_utc) VALUES (?, ?, ?, ?)`, sourceUUID, targetUUID, kind, formatTime(normalisedTime(now))); err != nil {
		return preview, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE slug_redirects SET target_uuid = ? WHERE entity_kind = ? AND target_uuid = ?`, targetUUID, kind, sourceUUID); err != nil {
		return preview, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO slug_redirects (entity_kind, old_slug, target_uuid, created_at_utc) VALUES (?, ?, ?, ?)
		ON CONFLICT(entity_kind, old_slug) DO UPDATE SET target_uuid = excluded.target_uuid, created_at_utc = excluded.created_at_utc`, kind, sourceSlug, targetUUID, formatTime(normalisedTime(now))); err != nil {
		return preview, err
	}
	for _, galleryID := range preview.AffectedGalleryIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_manifest_sync SET status = CASE WHEN status = 'CLEAN' THEN 'DB_DIRTY' ELSE status END WHERE gallery_id = ?`, galleryID); err != nil {
			return preview, err
		}
	}
	if kind == portableid.KindCoser {
		if _, err := tx.ExecContext(ctx, `UPDATE coser_manifest_sync SET status = CASE WHEN status = 'CLEAN' THEN 'DB_DIRTY' ELSE status END WHERE coser_uuid = ?`, targetUUID); err != nil {
			return preview, err
		}
	}
	if err := tx.Commit(); err != nil {
		return preview, err
	}
	return preview, nil
}

type mergeQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func mergeAffectedGalleryIDs(ctx context.Context, queryer mergeQueryer, kind portableid.Kind, sourceUUID string) ([]int64, error) {
	var query string
	switch kind {
	case portableid.KindCoser:
		query = `SELECT gallery_id FROM gallery_credits WHERE coser_uuid = ?`
	case portableid.KindWork:
		query = `SELECT DISTINCT gc.gallery_id FROM gallery_cast gc JOIN characters c ON c.uuid = gc.character_uuid WHERE c.work_uuid = ?`
	case portableid.KindCharacter:
		query = `SELECT gallery_id FROM gallery_cast WHERE character_uuid = ?`
	case portableid.KindTag:
		query = `SELECT gallery_id FROM gallery_tags WHERE tag_uuid = ?`
	default:
		return nil, fmt.Errorf("unsupported merge kind %s", kind)
	}
	rows, err := queryer.QueryContext(ctx, query, sourceUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[int64]struct{}{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		set[id] = struct{}{}
	}
	ids := make([]int64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, rows.Err()
}

func mergeConflicts(ctx context.Context, queryer mergeQueryer, kind portableid.Kind, sourceUUID, targetUUID string) ([]CoreEntityMergeConflict, error) {
	var conflicts []CoreEntityMergeConflict
	addCountConflict := func(code, query string, args ...any) error {
		var count int64
		if err := queryer.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			conflicts = append(conflicts, CoreEntityMergeConflict{Code: code, Details: fmt.Sprintf("%d conflicting relationship(s)", count)})
		}
		return nil
	}
	switch kind {
	case portableid.KindCoser:
		source, err := findCoser(ctx, queryer.(galleryQueryer), sourceUUID)
		if err != nil {
			return nil, err
		}
		target, err := findCoser(ctx, queryer.(galleryQueryer), targetUUID)
		if err != nil {
			return nil, err
		}
		if (source.ProfileSummary != "" && source.ProfileSummary != target.ProfileSummary) ||
			(source.Biography != "" && source.Biography != target.Biography) ||
			(source.CountryOrRegion != "" && source.CountryOrRegion != target.CountryOrRegion) ||
			(source.AvatarPath != "" && source.AvatarPath != target.AvatarPath) ||
			(source.BannerPath != "" && source.BannerPath != target.BannerPath) {
			conflicts = append(conflicts, CoreEntityMergeConflict{Code: "COSER_PROFILE", Details: "source profile or managed assets differ from target"})
		}
		if err := addCountConflict("DUPLICATE_GALLERY_CREDIT", `SELECT COUNT(*) FROM gallery_credits s JOIN gallery_credits t ON t.gallery_id=s.gallery_id WHERE s.coser_uuid=? AND t.coser_uuid=?`, sourceUUID, targetUUID); err != nil {
			return nil, err
		}
	case portableid.KindWork:
		if err := addCountConflict("DUPLICATE_CHARACTER_NAME", `SELECT COUNT(*) FROM characters s JOIN characters t ON t.work_uuid=? AND t.normalized_name=s.normalized_name WHERE s.work_uuid=?`, targetUUID, sourceUUID); err != nil {
			return nil, err
		}
	case portableid.KindCharacter:
		if err := addCountConflict("DUPLICATE_GALLERY_CAST", `SELECT COUNT(*) FROM gallery_cast s JOIN gallery_cast t ON t.gallery_credit_id=s.gallery_credit_id AND t.character_uuid=? WHERE s.character_uuid=?`, targetUUID, sourceUUID); err != nil {
			return nil, err
		}
	case portableid.KindTag:
		var sourceRecommendation, targetRecommendation, sourceAssignable, targetAssignable bool
		if err := queryer.QueryRowContext(ctx, `SELECT use_in_recommendation,allow_direct_assignment FROM tags WHERE uuid=?`, sourceUUID).Scan(&sourceRecommendation, &sourceAssignable); err != nil {
			return nil, err
		}
		if err := queryer.QueryRowContext(ctx, `SELECT use_in_recommendation,allow_direct_assignment FROM tags WHERE uuid=?`, targetUUID).Scan(&targetRecommendation, &targetAssignable); err != nil {
			return nil, err
		}
		if sourceRecommendation != targetRecommendation {
			conflicts = append(conflicts, CoreEntityMergeConflict{Code: "TAG_RECOMMENDATION", Details: "recommendation flags differ"})
		}
		if sourceAssignable != targetAssignable {
			conflicts = append(conflicts, CoreEntityMergeConflict{Code: "TAG_DIRECT_ASSIGNMENT", Details: "direct Gallery assignment flags differ"})
		}
		if err := addCountConflict("DUPLICATE_GALLERY_TAG", `SELECT COUNT(*) FROM gallery_tags s JOIN gallery_tags t ON t.gallery_id=s.gallery_id WHERE s.tag_uuid=? AND t.tag_uuid=?`, sourceUUID, targetUUID); err != nil {
			return nil, err
		}
		tagConflicts, err := previewTagEdges(ctx, queryer, sourceUUID, targetUUID)
		if err != nil {
			return nil, err
		}
		conflicts = append(conflicts, tagConflicts...)
	}
	return conflicts, nil
}

func mergeRelationships(ctx context.Context, tx *sql.Tx, kind portableid.Kind, sourceUUID, targetUUID string) error {
	switch kind {
	case portableid.KindCoser:
		var highwater int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position),0) FROM coser_social_accounts WHERE coser_uuid=?`, targetUUID).Scan(&highwater); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT account_uuid FROM coser_social_accounts WHERE coser_uuid=? ORDER BY position,account_uuid`, sourceUUID)
		if err != nil {
			return err
		}
		var accounts []string
		for rows.Next() {
			var uuid string
			if err := rows.Scan(&uuid); err != nil {
				rows.Close()
				return err
			}
			accounts = append(accounts, uuid)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, accountUUID := range accounts {
			highwater += 1024
			if _, err := tx.ExecContext(ctx, `UPDATE coser_social_accounts SET coser_uuid=?, position=? WHERE account_uuid=?`, targetUUID, highwater, accountUUID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE gallery_credits SET coser_uuid=? WHERE coser_uuid=?`, targetUUID, sourceUUID)
		return err
	case portableid.KindWork:
		_, err := tx.ExecContext(ctx, `UPDATE characters SET work_uuid=?, metadata_revision=metadata_revision+1 WHERE work_uuid=?`, targetUUID, sourceUUID)
		return err
	case portableid.KindCharacter:
		_, err := tx.ExecContext(ctx, `UPDATE gallery_cast SET character_uuid=? WHERE character_uuid=?`, targetUUID, sourceUUID)
		return err
	case portableid.KindTag:
		if _, err := tx.ExecContext(ctx, `UPDATE gallery_tags SET tag_uuid=? WHERE tag_uuid=?`, targetUUID, sourceUUID); err != nil {
			return err
		}
		return replaceTagEdges(ctx, tx, sourceUUID, targetUUID)
	default:
		return fmt.Errorf("unsupported merge kind %s", kind)
	}
}

func mergeEntityAliases(ctx context.Context, tx *sql.Tx, kind portableid.Kind, sourceUUID, targetUUID string) error {
	var table, owner string
	switch kind {
	case portableid.KindCoser:
		table, owner = "coser_aliases", "coser_uuid"
	case portableid.KindWork:
		table, owner = "work_aliases", "work_uuid"
	case portableid.KindCharacter:
		table, owner = "character_aliases", "character_uuid"
	case portableid.KindTag:
		table, owner = "tag_aliases", "tag_uuid"
	}
	var targetName, sourceName string
	entityTable, _ := tableForCoreKind(kind)
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT name FROM %s WHERE uuid=?`, entityTable.name), targetUUID).Scan(&targetName); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT name FROM %s WHERE uuid=?`, entityTable.name), sourceUUID).Scan(&sourceName); err != nil {
		return err
	}
	targetAliases, err := loadAliases(ctx, tx, table, owner, targetUUID)
	if err != nil {
		return err
	}
	sourceAliases, err := loadAliases(ctx, tx, table, owner, sourceUUID)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{normalizedKey(targetName): {}}
	merged := make([]string, 0, len(targetAliases)+len(sourceAliases)+1)
	for _, value := range append(append(targetAliases, sourceName), sourceAliases...) {
		key := normalizedKey(value)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, value)
	}
	if len(merged) > 100 {
		return errors.New("merge would exceed 100 aliases")
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s=?`, table, owner), sourceUUID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s=?`, table, owner), targetUUID); err != nil {
		return err
	}
	return insertAliases(ctx, tx, table, owner, targetUUID, merged)
}

type tagEdge struct {
	parent   string
	child    string
	position int64
}

func transformedTagEdges(ctx context.Context, queryer mergeQueryer, sourceUUID, targetUUID string) ([]tagEdge, []CoreEntityMergeConflict, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT parent_uuid,child_uuid,position FROM tag_edges ORDER BY parent_uuid,child_uuid`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var edges []tagEdge
	var conflicts []CoreEntityMergeConflict
	pairs, childPositions := map[string]struct{}{}, map[string]struct{}{}
	for rows.Next() {
		var edge tagEdge
		if err := rows.Scan(&edge.parent, &edge.child, &edge.position); err != nil {
			return nil, nil, err
		}
		if edge.parent == sourceUUID {
			edge.parent = targetUUID
		}
		if edge.child == sourceUUID {
			edge.child = targetUUID
		}
		if edge.parent == edge.child {
			conflicts = append(conflicts, CoreEntityMergeConflict{Code: "TAG_SELF_EDGE", Details: "merge would collapse a parent edge into itself"})
			continue
		}
		pair := edge.parent + "|" + edge.child
		position := fmt.Sprintf("%s|%d", edge.child, edge.position)
		if _, duplicate := pairs[pair]; duplicate {
			conflicts = append(conflicts, CoreEntityMergeConflict{Code: "TAG_DUPLICATE_EDGE", Details: "merge would duplicate a parent edge"})
			continue
		}
		if _, duplicate := childPositions[position]; duplicate {
			conflicts = append(conflicts, CoreEntityMergeConflict{Code: "TAG_POSITION", Details: "merge would duplicate a child parent position"})
			continue
		}
		pairs[pair], childPositions[position] = struct{}{}, struct{}{}
		edges = append(edges, edge)
	}
	return edges, conflicts, rows.Err()
}

func previewTagEdges(ctx context.Context, queryer mergeQueryer, sourceUUID, targetUUID string) ([]CoreEntityMergeConflict, error) {
	edges, conflicts, err := transformedTagEdges(ctx, queryer, sourceUUID, targetUUID)
	if err != nil {
		return nil, err
	}
	children := map[string][]string{}
	for _, edge := range edges {
		children[edge.parent] = append(children[edge.parent], edge.child)
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(node string) bool {
		if visiting[node] {
			return true
		}
		if visited[node] {
			return false
		}
		visiting[node] = true
		for _, child := range children[node] {
			if visit(child) {
				return true
			}
		}
		visiting[node], visited[node] = false, true
		return false
	}
	for node := range children {
		if visit(node) {
			conflicts = append(conflicts, CoreEntityMergeConflict{Code: "TAG_CYCLE", Details: "merge would create a Tag DAG cycle"})
			break
		}
	}
	return conflicts, nil
}

func replaceTagEdges(ctx context.Context, tx *sql.Tx, sourceUUID, targetUUID string) error {
	edges, conflicts, err := transformedTagEdges(ctx, tx, sourceUUID, targetUUID)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return ErrCoreEntityMergeConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tag_edges WHERE parent_uuid=? OR child_uuid=?`, sourceUUID, sourceUUID); err != nil {
		return err
	}
	for _, edge := range edges {
		if edge.parent != targetUUID && edge.child != targetUUID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tag_edges(parent_uuid,child_uuid,position) VALUES(?,?,?)`, edge.parent, edge.child, edge.position); err != nil {
			return err
		}
	}
	return nil
}
