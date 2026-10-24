package productserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"time"

	"github.com/stashapp/stash/internal/coversimilarity"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

const similarImageUploadPath = "/browse/similar-images/upload"

func (s *Server) similarImageUploadHandler(database *productdb.Database) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fail := func(status int, code string) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
		}
		if !s.Auth.AuthorizeRequest(r) {
			fail(http.StatusUnauthorized, "AUTH_REQUIRED")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			fail(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			return
		}
		distance := 4
		if values, ok := r.URL.Query()["maxDistance"]; ok {
			var err error
			if len(values) != 1 {
				fail(http.StatusBadRequest, "INVALID_DISTANCE")
				return
			}
			distance, err = strconv.Atoi(values[0])
			if err != nil || distance < 0 || distance > coversimilarity.MaximumPHashDistance {
				fail(http.StatusBadRequest, "INVALID_DISTANCE")
				return
			}
		}
		s.similarUploadOnce.Do(func() { s.similarUploadSlots = make(chan struct{}, 1) })
		select {
		case s.similarUploadSlots <- struct{}{}:
			defer func() { <-s.similarUploadSlots }()
		default:
			w.Header().Set("Retry-After", "1")
			fail(http.StatusTooManyRequests, "SEARCH_BUSY")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, coversimilarity.MaximumUploadBytes+(1<<20))
		reader, err := r.MultipartReader()
		if err != nil {
			fail(http.StatusBadRequest, "INVALID_UPLOAD")
			return
		}
		part, err := reader.NextPart()
		if err != nil || part.FormName() != "file" || part.FileName() == "" {
			fail(http.StatusBadRequest, "INVALID_UPLOAD")
			return
		}
		if r.Context().Err() != nil {
			return
		}
		query, err := coversimilarity.PrepareUpload(part)
		part.Close()
		if err != nil {
			var tooLarge *http.MaxBytesError
			switch {
			case errors.Is(err, coversimilarity.ErrUploadSize) || errors.As(err, &tooLarge):
				fail(http.StatusRequestEntityTooLarge, "IMAGE_TOO_LARGE")
			case errors.Is(err, coversimilarity.ErrUploadPixels):
				fail(http.StatusRequestEntityTooLarge, "IMAGE_PIXELS")
			default:
				fail(http.StatusUnsupportedMediaType, "INVALID_IMAGE")
			}
			return
		}
		if _, err = reader.NextPart(); !errors.Is(err, io.EOF) {
			fail(http.StatusBadRequest, "INVALID_UPLOAD")
			return
		}
		if r.Context().Err() != nil {
			return
		}
		// Match CARD_DISPLAY_960's existing WebP-80 encoding entirely in memory.
		// Without FFmpeg, the original/scaled/JPEG probes remain available.
		if s.VideoTools.FFmpeg.Available {
			if probe, e := uploadDisplayProbe(r.Context(), s.VideoTools.FFmpeg.Path, query.DisplayImage); e == nil {
				query.Probes = append(query.Probes, probe)
			}
		}
		result, err := database.Browse().SimilarImagesTo(r.Context(), query.Original, distance, query.Probes...)
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			if errors.Is(err, context.DeadlineExceeded) {
				fail(http.StatusGatewayTimeout, "SEARCH_TIMEOUT")
			} else {
				fail(http.StatusInternalServerError, "SEARCH_FAILED")
			}
			return
		}
		matches := make([]map[string]any, 0, len(result.Matches))
		for _, match := range result.Matches {
			image := match.Image
			resource := image.Resource
			matches = append(matches, map[string]any{"pHashDistance": match.PHashDistance, "dHashDistance": match.DHashDistance,
				"image": map[string]any{"itemUUID": image.ItemUUID, "gallerySetID": image.GallerySetID, "gallerySlug": image.GallerySlug, "galleryTitle": image.GalleryTitle, "fileName": image.FileName,
					"resource": map[string]any{"itemUUID": resource.ItemUUID, "contentRevision": resource.ContentRevision, "profileHash": resource.ProfileHash, "variant": resource.Variant, "mimeType": resource.MIMEType}}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"totalImages": result.TotalImages, "hashedImages": result.HashedImages, "comparedImages": result.ComparedImages, "matchedImages": result.MatchedImages,
			"complete": result.Complete, "hasMore": result.HasMore, "queryWidth": query.Original.Width, "queryHeight": query.Original.Height, "matches": matches})
	})
}

func uploadDisplayProbe(ctx context.Context, executable string, img image.Image) (coversimilarity.Signature, error) {
	if err := ctx.Err(); err != nil {
		return coversimilarity.Signature{}, err
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return coversimilarity.Signature{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-hide_banner", "-loglevel", "error", "-nostdin", "-threads", "1", "-i", "pipe:0", "-frames:v", "1", "-pix_fmt", "bgra", "-c:v", "libwebp", "-threads", "1", "-quality", "80", "-compression_level", "4", "-f", "webp", "pipe:1")
	var output uploadProbeBuffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = &encoded, &output, io.Discard
	if err := cmd.Run(); err != nil {
		return coversimilarity.Signature{}, err
	}
	return coversimilarity.Compute(bytes.NewReader(output.Bytes()))
}

type uploadProbeBuffer struct{ bytes.Buffer }

func (b *uploadProbeBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > coversimilarity.MaximumUploadBytes {
		return 0, coversimilarity.ErrUploadSize
	}
	return b.Buffer.Write(data)
}
