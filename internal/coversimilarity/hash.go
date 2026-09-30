// Package coversimilarity computes small, versioned signatures for selecting
// a replacement cover. Signatures are never used as Item identity evidence.
package coversimilarity

import (
	"bytes"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/bits"
	"sort"

	"github.com/corona10/goimagehash"
	"github.com/disintegration/imaging"
)

const Version = 1

// MaximumPHashDistance is an intentionally conservative initial gate for the
// 64-bit pHash. It must be calibrated against labelled CGM replacements and
// hard negatives before being loosened.
const MaximumPHashDistance = 6

type Signature struct {
	DHashH uint64
	DHashV uint64
	PHash  uint64
	Width  int
	Height int
}

func Compute(reader io.Reader) (Signature, error) {
	data, err := io.ReadAll(io.LimitReader(reader, (16<<20)+1))
	if err != nil || len(data) > 16<<20 {
		return Signature{}, errors.New("cover cache image exceeds signature byte limit")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Signature{}, err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 10000 || config.Height > 10000 || int64(config.Width)*int64(config.Height) > 30_000_000 {
		return Signature{}, errors.New("invalid cover signature dimensions")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Signature{}, err
	}
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	if width != config.Width || height != config.Height {
		return Signature{}, errors.New("invalid cover signature dimensions")
	}
	small := imaging.Resize(img, 9, 9, imaging.Lanczos)
	var gray [9][9]uint8
	for y := 0; y < 9; y++ {
		for x := 0; x < 9; x++ {
			pixel := small.At(x, y)
			r, g, b, _ := pixel.RGBA()
			gray[y][x] = uint8(((299*r + 587*g + 114*b) / 1000) >> 8)
		}
	}
	var horizontal, vertical uint64
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			bit := uint(y*8 + x)
			if gray[y][x] > gray[y][x+1] {
				horizontal |= uint64(1) << bit
			}
			if gray[y][x] > gray[y+1][x] {
				vertical |= uint64(1) << bit
			}
		}
	}
	phash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return Signature{}, err
	}
	return Signature{DHashH: horizontal, DHashV: vertical, PHash: phash.GetHash(), Width: width, Height: height}, nil
}

func DHashDistance(a, b Signature) int {
	return bits.OnesCount64(a.DHashH^b.DHashH) + bits.OnesCount64(a.DHashV^b.DHashV)
}

func PHashDistance(a, b Signature) int { return bits.OnesCount64(a.PHash ^ b.PHash) }

type Candidate struct {
	ID        string
	Signature Signature
}

// Select applies the two-level ranking and an absolute pHash gate. Two
// equally good (or duplicate) matches are valid, not an ambiguity error.
func Select(old Signature, candidates []Candidate) (string, int) {
	type ranked struct {
		id           string
		dhash, phash int
	}
	available := make([]ranked, 0, len(candidates))
	for _, candidate := range candidates {
		if !SimilarAspect(old, candidate.Signature) {
			continue
		}
		available = append(available, ranked{id: candidate.ID,
			dhash: DHashDistance(old, candidate.Signature), phash: PHashDistance(old, candidate.Signature)})
	}
	usedDHash := len(available) > 5
	if usedDHash {
		sort.Slice(available, func(i, j int) bool {
			if available[i].dhash != available[j].dhash {
				return available[i].dhash < available[j].dhash
			}
			return available[i].id < available[j].id
		})
		available = available[:5]
	}
	qualified := available[:0]
	for _, candidate := range available {
		if candidate.phash <= MaximumPHashDistance {
			qualified = append(qualified, candidate)
		}
	}
	if len(qualified) == 0 {
		return "", 0
	}
	sort.Slice(qualified, func(i, j int) bool {
		if qualified[i].phash != qualified[j].phash {
			return qualified[i].phash < qualified[j].phash
		}
		if usedDHash && qualified[i].dhash != qualified[j].dhash {
			return qualified[i].dhash < qualified[j].dhash
		}
		return qualified[i].id < qualified[j].id
	})
	return qualified[0].id, len(qualified)
}

// SimilarAspect tolerates small rounding differences introduced by resizing
// but rejects a different crop or orientation before hash comparison.
func SimilarAspect(a, b Signature) bool {
	if a.Width <= 0 || a.Height <= 0 || b.Width <= 0 || b.Height <= 0 {
		return false
	}
	left, right := int64(a.Width)*int64(b.Height), int64(b.Width)*int64(a.Height)
	if left < right {
		left, right = right, left
	}
	return left*100 <= right*105
}

func Hex(value uint64) string {
	var bytes [8]byte
	for i := 7; i >= 0; i-- {
		bytes[i] = byte(value)
		value >>= 8
	}
	return hex.EncodeToString(bytes[:])
}

func ParseHex(value string) (uint64, error) {
	if len(value) != 16 {
		return 0, errors.New("invalid cover signature length")
	}
	bytes, err := hex.DecodeString(value)
	if err != nil {
		return 0, err
	}
	var result uint64
	for _, b := range bytes {
		result = result<<8 | uint64(b)
	}
	return result, nil
}
