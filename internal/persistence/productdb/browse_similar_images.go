package productdb

import (
	"container/heap"
	"context"
	"database/sql"
	"errors"
	"math/bits"
	"path"
	"sort"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/coversimilarity"
)

const similarImageLimit = 10000
const similarPairLimit = 200
const similarComparisonLimit = 1000000

type SimilarImage struct {
	ItemUUID, GallerySetID, GallerySlug, GalleryTitle, FileName string
	Resource                                                    browse.ResourceIdentity
	galleryID                                                   int64
	fingerprint                                                 string
	signature                                                   coversimilarity.Signature
}

type SimilarImagePair struct {
	Left, Right                  SimilarImage
	PHashDistance, DHashDistance int
	SameContent                  bool
}

type SimilarImageResult struct {
	TotalImages, HashedImages, ComparedImages, Comparisons, MatchedPairs int
	Complete, HasMore                                                    bool
	Pairs                                                                []SimilarImagePair
}

// SimilarImages is an explicit read-only operation. It never schedules hashes,
// decodes source files, or treats a perceptual match as identity evidence.
func (s *BrowseStore) SimilarImages(ctx context.Context, maxDistance int, mode string) (SimilarImageResult, error) {
	if maxDistance < 0 || maxDistance > coversimilarity.MaximumPHashDistance || (mode != "ALL" && mode != "SAME_GALLERY" && mode != "CROSS_GALLERY") {
		return SimilarImageResult{}, errors.New("invalid similar image search options")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result := SimilarImageResult{Complete: true, Pairs: []SimilarImagePair{}}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	from := ` FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id
		JOIN gallery_sources source ON source.id=item.source_id AND source.gallery_id=gallery.id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		LEFT JOIN media_derivatives derivative ON derivative.item_uuid=item.item_uuid
		 AND derivative.content_revision=item.content_revision AND derivative.is_current=1 AND derivative.state='READY'
		 AND derivative.variant=` + staticBaseVariantSQL("item.item_uuid") + `
		LEFT JOIN cover_image_signatures sig ON sig.item_uuid=item.item_uuid AND sig.content_revision=item.content_revision
		 AND sig.profile_hash=derivative.profile_hash AND sig.signature_version=?
		WHERE ` + browseVisibleGalleryPredicate + ` AND item.media_kind='STATIC_IMAGE'
		 AND item.availability_state='AVAILABLE' AND item.processing_state='READY' AND item.excluded=0`
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(sig.item_uuid)`+from, coversimilarity.Version).Scan(&result.TotalImages, &result.HashedImages); err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT item.item_uuid,gallery.id,gallery.set_id,gallery.slug,gallery.title,item.relative_path,
		item.full_fingerprint,item.content_revision,derivative.profile_hash,derivative.variant,derivative.mime_type,
		sig.dhash_h,sig.dhash_v,sig.phash,sig.width,sig.height`+from+` AND sig.item_uuid IS NOT NULL ORDER BY item.id LIMIT ?`, coversimilarity.Version, similarImageLimit)
	if err != nil {
		return result, err
	}
	var images []SimilarImage
	for rows.Next() {
		var image SimilarImage
		var relative, h, v, p string
		if err := rows.Scan(&image.ItemUUID, &image.galleryID, &image.GallerySetID, &image.GallerySlug, &image.GalleryTitle, &relative,
			&image.fingerprint, &image.Resource.ContentRevision, &image.Resource.ProfileHash, &image.Resource.Variant, &image.Resource.MIMEType,
			&h, &v, &p, &image.signature.Width, &image.signature.Height); err != nil {
			rows.Close()
			return result, err
		}
		image.Resource.ItemUUID = image.ItemUUID
		image.FileName = path.Base(relative)
		if image.signature.DHashH, err = coversimilarity.ParseHex(h); err != nil {
			rows.Close()
			return result, err
		}
		if image.signature.DHashV, err = coversimilarity.ParseHex(v); err != nil {
			rows.Close()
			return result, err
		}
		if image.signature.PHash, err = coversimilarity.ParseHex(p); err != nil {
			rows.Close()
			return result, err
		}
		images = append(images, image)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	if err := tx.Commit(); err != nil {
		return result, err
	}
	result.ComparedImages = len(images)
	result.Complete = result.HashedImages <= similarImageLimit
	return compareSimilarImages(ctx, images, maxDistance, mode, result, similarComparisonLimit)
}

// Seven disjoint pHash bands guarantee that hashes within six bits share at
// least one band. No nearest-neighbour top-N cutoff can silently omit matches.
func compareSimilarImages(ctx context.Context, images []SimilarImage, maxDistance int, mode string, result SimilarImageResult, budget int) (SimilarImageResult, error) {
	var bands [7]map[uint64][]int
	for i := range bands {
		bands[i] = make(map[uint64][]int)
	}
	best := &similarPairHeap{}
	heap.Init(best)
	stop := false
	seen := make([]int, len(images))
	for i, image := range images {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		shift := uint(0)
		for band := 0; band < 7 && !stop; band++ {
			width := uint(9)
			if band == 0 {
				width = 10
			}
			key := (image.signature.PHash >> shift) & ((uint64(1) << width) - 1)
			for _, j := range bands[band][key] {
				if seen[j] == i+1 {
					continue
				}
				seen[j] = i + 1
				if result.Comparisons >= budget {
					result.Complete = false
					stop = true
					break
				}
				result.Comparisons++
				if result.Comparisons%256 == 0 {
					if err := ctx.Err(); err != nil {
						return result, err
					}
				}
				other := images[j]
				if (mode == "SAME_GALLERY" && image.galleryID != other.galleryID) || (mode == "CROSS_GALLERY" && image.galleryID == other.galleryID) {
					continue
				}
				if !coversimilarity.SimilarAspect(image.signature, other.signature) {
					continue
				}
				p := bits.OnesCount64(image.signature.PHash ^ other.signature.PHash)
				if p > maxDistance {
					continue
				}
				pair := SimilarImagePair{Left: other, Right: image, PHashDistance: p, DHashDistance: coversimilarity.DHashDistance(image.signature, other.signature), SameContent: image.fingerprint != "" && image.fingerprint == other.fingerprint}
				result.MatchedPairs++
				if best.Len() < similarPairLimit {
					heap.Push(best, pair)
				} else if similarPairLess(pair, (*best)[0]) {
					(*best)[0] = pair
					heap.Fix(best, 0)
				}
			}
			bands[band][key] = append(bands[band][key], i)
			shift += width
		}
		if stop {
			break
		}
	}
	result.Pairs = []SimilarImagePair(*best)
	sort.Slice(result.Pairs, func(i, j int) bool { return similarPairLess(result.Pairs[i], result.Pairs[j]) })
	result.HasMore = result.MatchedPairs > len(result.Pairs)
	return result, nil
}

func similarPairLess(a, b SimilarImagePair) bool {
	if a.PHashDistance != b.PHashDistance {
		return a.PHashDistance < b.PHashDistance
	}
	if a.DHashDistance != b.DHashDistance {
		return a.DHashDistance < b.DHashDistance
	}
	if a.Left.ItemUUID != b.Left.ItemUUID {
		return a.Left.ItemUUID < b.Left.ItemUUID
	}
	return a.Right.ItemUUID < b.Right.ItemUUID
}

type similarPairHeap []SimilarImagePair

func (h similarPairHeap) Len() int           { return len(h) }
func (h similarPairHeap) Less(i, j int) bool { return similarPairLess(h[j], h[i]) }
func (h similarPairHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *similarPairHeap) Push(v any)        { *h = append(*h, v.(SimilarImagePair)) }
func (h *similarPairHeap) Pop() any          { old := *h; v := old[len(old)-1]; *h = old[:len(old)-1]; return v }
