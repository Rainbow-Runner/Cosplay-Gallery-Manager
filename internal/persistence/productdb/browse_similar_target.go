package productdb

import (
	"container/heap"
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/coversimilarity"
)

const similarTargetImageLimit = 100000

type SimilarImageMatch struct {
	Image                        SimilarImage
	PHashDistance, DHashDistance int
}

type SimilarTargetResult struct {
	TotalImages, HashedImages, ComparedImages, MatchedImages int
	Complete, HasMore                                        bool
	Matches                                                  []SimilarImageMatch
}

// SimilarImagesTo streams candidates directly, not the capped all-pairs result.
// Its memory is bounded by the best 200 matches, independent of library size.
func (s *BrowseStore) SimilarImagesTo(ctx context.Context, target coversimilarity.Signature, distance int, alternatives ...coversimilarity.Signature) (SimilarTargetResult, error) {
	result := SimilarTargetResult{Matches: []SimilarImageMatch{}}
	if distance < 0 || distance > coversimilarity.MaximumPHashDistance || target.Width < 1 || target.Height < 1 {
		return result, errors.New("invalid target image search options")
	}
	if len(alternatives) > 8 {
		return result, errors.New("too many target probes")
	}
	probes := append([]coversimilarity.Signature{target}, alternatives...)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	from := similarImageBrowseFrom()
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(sig.item_uuid)`+from, coversimilarity.Version).Scan(&result.TotalImages, &result.HashedImages); err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT item.item_uuid,gallery.id,gallery.set_id,gallery.slug,gallery.title,item.relative_path,
		item.full_fingerprint,item.content_revision,derivative.profile_hash,derivative.variant,derivative.mime_type,
		sig.dhash_h,sig.dhash_v,sig.phash,sig.width,sig.height`+from+` AND sig.item_uuid IS NOT NULL ORDER BY item.id LIMIT ?`, coversimilarity.Version, similarTargetImageLimit)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	best := &similarTargetHeap{}
	heap.Init(best)
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		image, err := scanSimilarImage(rows)
		if err != nil {
			return result, err
		}
		result.ComparedImages++
		p, d := 65, 129
		for _, probe := range probes {
			if !coversimilarity.SimilarAspect(probe, image.signature) {
				continue
			}
			candidateP, candidateD := coversimilarity.PHashDistance(probe, image.signature), coversimilarity.DHashDistance(probe, image.signature)
			if candidateP < p || (candidateP == p && candidateD < d) {
				p, d = candidateP, candidateD
			}
		}
		if p > distance {
			continue
		}
		result.MatchedImages++
		match := SimilarImageMatch{Image: image, PHashDistance: p, DHashDistance: d}
		if best.Len() < similarPairLimit {
			heap.Push(best, match)
		} else if targetMatchLess(match, (*best)[0]) {
			(*best)[0] = match
			heap.Fix(best, 0)
		}
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	rows.Close()
	if err = tx.Commit(); err != nil {
		return result, err
	}
	result.Complete = result.ComparedImages == result.HashedImages
	result.HasMore = result.MatchedImages > best.Len()
	result.Matches = []SimilarImageMatch(*best)
	sort.Slice(result.Matches, func(i, j int) bool { return targetMatchLess(result.Matches[i], result.Matches[j]) })
	return result, nil
}

func targetMatchLess(a, b SimilarImageMatch) bool {
	if a.PHashDistance != b.PHashDistance {
		return a.PHashDistance < b.PHashDistance
	}
	if a.DHashDistance != b.DHashDistance {
		return a.DHashDistance < b.DHashDistance
	}
	return a.Image.ItemUUID < b.Image.ItemUUID
}

type similarTargetHeap []SimilarImageMatch

func (h similarTargetHeap) Len() int           { return len(h) }
func (h similarTargetHeap) Less(i, j int) bool { return targetMatchLess(h[j], h[i]) }
func (h similarTargetHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *similarTargetHeap) Push(v any)        { *h = append(*h, v.(SimilarImageMatch)) }
func (h *similarTargetHeap) Pop() any {
	old := *h
	v := old[len(old)-1]
	*h = old[:len(old)-1]
	return v
}
