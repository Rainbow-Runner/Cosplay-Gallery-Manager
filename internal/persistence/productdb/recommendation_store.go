package productdb

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/browse"
)

// StrongRecommendations uses binary relationship features: multiple shared
// people or roles do not stack within one feature in MVP.
func (s *BrowseStore) StrongRecommendations(ctx context.Context, sourceSetID string, scope browse.Scope) ([]browse.GalleryRecommendation, error) {
	settingsValue, err := (&SettingsStore{db: s.db}).Find(ctx)
	if err != nil {
		return nil, err
	}
	sourceID, err := s.visibleGalleryID(ctx, sourceSetID, scope)
	if err != nil {
		return nil, err
	}
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return nil, err
	}
	args := []any{sourceID, sourceID, sourceID, sourceID, sourceID, sourceID, sourceID, sourceID, sourceID}
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	args = append(args, sourceID, sourceID, sourceID)
	args = append(args, settingsValue.RelatedLimit)
	rows, err := s.db.QueryContext(ctx, `SELECT gallery.id,
		EXISTS(SELECT 1 FROM gallery_cast source_cast JOIN gallery_cast candidate_cast ON candidate_cast.character_uuid=source_cast.character_uuid
		 WHERE source_cast.gallery_id=? AND candidate_cast.gallery_id=gallery.id),
		EXISTS(SELECT 1 FROM gallery_cast source_cast JOIN characters source_character ON source_character.uuid=source_cast.character_uuid
		 JOIN gallery_cast candidate_cast ON candidate_cast.gallery_id=gallery.id
		 JOIN characters candidate_character ON candidate_character.uuid=candidate_cast.character_uuid
		 WHERE source_cast.gallery_id=? AND candidate_character.work_uuid=source_character.work_uuid),
		EXISTS(SELECT 1 FROM gallery_credits source_credit JOIN gallery_credits candidate_credit ON candidate_credit.coser_uuid=source_credit.coser_uuid
		 WHERE source_credit.gallery_id=? AND candidate_credit.gallery_id=gallery.id),
		(EXISTS(SELECT 1 FROM gallery_cast WHERE gallery_id=?)=EXISTS(SELECT 1 FROM gallery_cast WHERE gallery_id=gallery.id)),
		(CASE WHEN EXISTS(SELECT 1 FROM gallery_cast source_cast JOIN gallery_cast candidate_cast ON candidate_cast.character_uuid=source_cast.character_uuid
		 WHERE source_cast.gallery_id=? AND candidate_cast.gallery_id=gallery.id) THEN 100 ELSE 0 END+
		 CASE WHEN EXISTS(SELECT 1 FROM gallery_cast source_cast JOIN characters source_character ON source_character.uuid=source_cast.character_uuid
		 JOIN gallery_cast candidate_cast ON candidate_cast.gallery_id=gallery.id JOIN characters candidate_character ON candidate_character.uuid=candidate_cast.character_uuid
		 WHERE source_cast.gallery_id=? AND candidate_character.work_uuid=source_character.work_uuid) THEN 50 ELSE 0 END+
		 CASE WHEN EXISTS(SELECT 1 FROM gallery_credits source_credit JOIN gallery_credits candidate_credit ON candidate_credit.coser_uuid=source_credit.coser_uuid
		 WHERE source_credit.gallery_id=? AND candidate_credit.gallery_id=gallery.id) THEN 40 ELSE 0 END+
		 CASE WHEN (EXISTS(SELECT 1 FROM gallery_cast WHERE gallery_id=?)=EXISTS(SELECT 1 FROM gallery_cast WHERE gallery_id=gallery.id)) THEN 5 ELSE 0 END) score
		FROM galleries gallery JOIN gallery_sources source ON source.gallery_id=gallery.id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE gallery.id<>? AND `+browseVisibleGalleryPredicate+scopeSQL+`
		AND (EXISTS(SELECT 1 FROM gallery_cast source_cast JOIN gallery_cast candidate_cast ON candidate_cast.character_uuid=source_cast.character_uuid
		 WHERE source_cast.gallery_id=? AND candidate_cast.gallery_id=gallery.id)
		 OR EXISTS(SELECT 1 FROM gallery_cast source_cast JOIN characters source_character ON source_character.uuid=source_cast.character_uuid
		 JOIN gallery_cast candidate_cast ON candidate_cast.gallery_id=gallery.id JOIN characters candidate_character ON candidate_character.uuid=candidate_cast.character_uuid
		 WHERE source_cast.gallery_id=? AND candidate_character.work_uuid=source_character.work_uuid)
		 OR EXISTS(SELECT 1 FROM gallery_credits source_credit JOIN gallery_credits candidate_credit ON candidate_credit.coser_uuid=source_credit.coser_uuid
		 WHERE source_credit.gallery_id=? AND candidate_credit.gallery_id=gallery.id))
		ORDER BY score DESC,gallery.added_at_utc DESC,gallery.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type candidate struct {
		id                          int64
		character, work, coser, typ int
		score                       float64
	}
	var candidates []candidate
	for rows.Next() {
		var value candidate
		if err := rows.Scan(&value.id, &value.character, &value.work, &value.coser, &value.typ, &value.score); err != nil {
			return nil, err
		}
		candidates = append(candidates, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]browse.GalleryRecommendation, 0, len(candidates))
	for _, candidate := range candidates {
		card, err := s.galleryCardByID(ctx, scope, candidate.id)
		if err != nil {
			return nil, err
		}
		value := browse.GalleryRecommendation{Card: card, Score: candidate.score}
		if candidate.character == 1 {
			value.Reasons = append(value.Reasons, browse.ReasonCharacter)
		}
		if candidate.work == 1 {
			value.Reasons = append(value.Reasons, browse.ReasonWork)
		}
		if candidate.coser == 1 {
			value.Reasons = append(value.Reasons, browse.ReasonCoser)
		}
		if candidate.typ == 1 {
			value.Reasons = append(value.Reasons, browse.ReasonType)
		}
		result = append(result, value)
	}
	return result, nil
}

func (s *BrowseStore) TagRecommendations(ctx context.Context, sourceSetID string, scope browse.Scope) ([]browse.GalleryRecommendation, error) {
	settingsValue, err := (&SettingsStore{db: s.db}).Find(ctx)
	if err != nil {
		return nil, err
	}
	sourceID, err := s.visibleGalleryID(ctx, sourceSetID, scope)
	if err != nil {
		return nil, err
	}
	galleryTimes, err := s.visibleGalleryTimes(ctx, scope)
	if err != nil {
		return nil, err
	}
	direct, parents, err := s.recommendationTags(ctx, scope)
	if err != nil {
		return nil, err
	}
	vectors := make(map[int64]map[string]float64, len(galleryTimes))
	documentFrequency := make(map[string]int)
	for galleryID := range galleryTimes {
		vector := expandTagVector(direct[galleryID], parents, settingsValue.TagParentWeight, settingsValue.TagMaximumDepth)
		vectors[galleryID] = vector
		for tag := range vector {
			documentFrequency[tag]++
		}
	}
	sourceVector := vectors[sourceID]
	if len(sourceVector) == 0 {
		return nil, nil
	}
	total := float64(len(galleryTimes))
	idf := make(map[string]float64, len(documentFrequency))
	for tag, frequency := range documentFrequency {
		idf[tag] = math.Log((total+1)/(float64(frequency)+1)) + 1
	}
	type scored struct {
		id    int64
		score float64
	}
	var candidates []scored
	for galleryID, vector := range vectors {
		if galleryID == sourceID || len(vector) == 0 {
			continue
		}
		score := weightedJaccard(sourceVector, vector, idf)
		if score >= settingsValue.TagMinimumScore {
			candidates = append(candidates, scored{id: galleryID, score: score})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if !galleryTimes[candidates[i].id].Equal(galleryTimes[candidates[j].id]) {
			return galleryTimes[candidates[i].id].After(galleryTimes[candidates[j].id])
		}
		return candidates[i].id > candidates[j].id
	})
	if len(candidates) > settingsValue.RelatedLimit {
		candidates = candidates[:settingsValue.RelatedLimit]
	}
	result := make([]browse.GalleryRecommendation, 0, len(candidates))
	for _, candidate := range candidates {
		card, err := s.galleryCardByID(ctx, scope, candidate.id)
		if err != nil {
			return nil, err
		}
		result = append(result, browse.GalleryRecommendation{Card: card, Score: candidate.score, Reasons: []browse.RecommendationReason{browse.ReasonTags}})
	}
	return result, nil
}

func (s *BrowseStore) visibleGalleryID(ctx context.Context, setID string, scope browse.Scope) (int64, error) {
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return 0, err
	}
	args := []any{setID}
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `SELECT gallery.id FROM galleries gallery JOIN gallery_sources source ON source.gallery_id=gallery.id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE gallery.set_id=? AND `+browseVisibleGalleryPredicate+scopeSQL, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrBrowseGalleryNotVisible
	}
	return id, err
}

func (s *BrowseStore) visibleGalleryTimes(ctx context.Context, scope browse.Scope) (map[int64]time.Time, error) {
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return nil, err
	}
	var args []any
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT gallery.id,gallery.added_at_utc FROM galleries gallery
		JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE `+browseVisibleGalleryPredicate+scopeSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]time.Time)
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		value, err := parseTime(raw)
		if err != nil {
			return nil, err
		}
		result[id] = value
	}
	return result, rows.Err()
}

func (s *BrowseStore) recommendationTags(ctx context.Context, scope browse.Scope) (map[int64][]string, map[string][]string, error) {
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return nil, nil, err
	}
	var args []any
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT relation.gallery_id,relation.tag_uuid FROM gallery_tags relation
		JOIN tags tag ON tag.uuid=relation.tag_uuid JOIN galleries gallery ON gallery.id=relation.gallery_id
		JOIN gallery_sources source ON source.gallery_id=gallery.id LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE tag.use_in_recommendation=1 AND `+browseVisibleGalleryPredicate+scopeSQL, args...)
	if err != nil {
		return nil, nil, err
	}
	direct := make(map[int64][]string)
	for rows.Next() {
		var galleryID int64
		var tag string
		if err := rows.Scan(&galleryID, &tag); err != nil {
			rows.Close()
			return nil, nil, err
		}
		direct[galleryID] = append(direct[galleryID], tag)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	edges, err := s.db.QueryContext(ctx, `SELECT edge.child_uuid,edge.parent_uuid FROM tag_edges edge
		JOIN tags child ON child.uuid=edge.child_uuid JOIN tags parent ON parent.uuid=edge.parent_uuid
		WHERE child.use_in_recommendation=1 AND parent.use_in_recommendation=1`)
	if err != nil {
		return nil, nil, err
	}
	defer edges.Close()
	parents := make(map[string][]string)
	for edges.Next() {
		var child, parent string
		if err := edges.Scan(&child, &parent); err != nil {
			return nil, nil, err
		}
		parents[child] = append(parents[child], parent)
	}
	return direct, parents, edges.Err()
}

func expandTagVector(direct []string, parents map[string][]string, parentWeight float64, maximumDepth int) map[string]float64 {
	result := make(map[string]float64)
	type node struct {
		uuid  string
		depth int
	}
	queue := make([]node, 0, len(direct))
	for _, tag := range direct {
		result[tag] = 1
		queue = append(queue, node{uuid: tag})
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= maximumDepth {
			continue
		}
		for _, parent := range parents[current.uuid] {
			depth := current.depth + 1
			weight := math.Pow(parentWeight, float64(depth))
			if prior, exists := result[parent]; exists && prior >= weight {
				continue
			}
			result[parent] = weight
			queue = append(queue, node{uuid: parent, depth: depth})
		}
	}
	return result
}

func weightedJaccard(left, right map[string]float64, idf map[string]float64) float64 {
	keys := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}
	var numerator, denominator float64
	for key := range keys {
		leftValue, rightValue := left[key]*idf[key], right[key]*idf[key]
		numerator += math.Min(leftValue, rightValue)
		denominator += math.Max(leftValue, rightValue)
	}
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}
