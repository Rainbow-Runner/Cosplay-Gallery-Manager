package coversimilarity

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"io"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

const MaximumUploadBytes = 16 << 20
const MaximumUploadPixels = 30_000_000

var ErrUploadSize = errors.New("uploaded image exceeds byte limit")
var ErrUploadPixels = errors.New("uploaded image exceeds dimension limit")
var ErrUploadImage = errors.New("invalid or unsupported static image")

// ComputeUpload never writes files. It follows the display generator's EXIF
// orientation policy, but does not change the existing stored-signature format.
func ComputeUpload(reader io.Reader) (Signature, error) {
	img, err := decodeUpload(reader)
	if err != nil {
		return Signature{}, err
	}
	return computeImage(img)
}

type UploadQuery struct {
	Original     Signature
	Probes       []Signature
	DisplayImage image.Image
}

// Query probes model the existing display sizes without rewriting any stored
// hashes. A library item is still returned only once, at its best distance.
func PrepareUpload(reader io.Reader) (UploadQuery, error) {
	img, err := decodeUpload(reader)
	if err != nil {
		return UploadQuery{}, err
	}
	original, err := computeImage(img)
	if err != nil {
		return UploadQuery{}, err
	}
	query := UploadQuery{Original: original, Probes: []Signature{}, DisplayImage: img}
	for _, maximum := range []int{480, 960} {
		resized := img
		if img.Bounds().Dx() > maximum || img.Bounds().Dy() > maximum {
			resized = imaging.Fit(img, maximum, maximum, imaging.Lanczos)
		}
		probe, e := computeImage(resized)
		if e != nil {
			return UploadQuery{}, e
		}
		query.Probes = append(query.Probes, probe)
		if maximum == 960 {
			query.DisplayImage = resized
			continue
		}
		var encoded bytes.Buffer
		if e = jpeg.Encode(&encoded, resized, &jpeg.Options{Quality: 90}); e != nil {
			return UploadQuery{}, e
		}
		probe, e = Compute(&encoded)
		if e != nil {
			return UploadQuery{}, e
		}
		query.Probes = append(query.Probes, probe)
	}
	return query, nil
}

func decodeUpload(reader io.Reader) (image.Image, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaximumUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaximumUploadBytes {
		return nil, ErrUploadSize
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png" && format != "webp") || animatedUpload(data, format) {
		return nil, ErrUploadImage
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 10000 || config.Height > 10000 || int64(config.Width)*int64(config.Height) > MaximumUploadPixels {
		return nil, ErrUploadPixels
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, ErrUploadImage
	}
	return img, nil
}

func animatedUpload(data []byte, format string) bool {
	if format == "png" {
		for offset := 8; offset+12 <= len(data); {
			size := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
			if size+12 > uint64(len(data)-offset) {
				return true
			}
			if string(data[offset+4:offset+8]) == "acTL" {
				return true
			}
			offset += int(size) + 12
		}
	}
	if format == "webp" {
		for offset := 12; offset+8 <= len(data); {
			size := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
			if size+8 > uint64(len(data)-offset) {
				return true
			}
			kind := string(data[offset : offset+4])
			if kind == "ANIM" || kind == "ANMF" || (kind == "VP8X" && size > 0 && data[offset+8]&2 != 0) {
				return true
			}
			offset += int(size) + 8 + int(size&1)
		}
	}
	return false
}
