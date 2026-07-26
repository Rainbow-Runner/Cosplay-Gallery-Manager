package productserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/coreentity"
	"github.com/stashapp/stash/internal/coserasset"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
)

const (
	coserAssetUploadPrefix   = "/manage/coser-assets/"
	coserAssetResourcePrefix = "/resource/coser/"
)

type coserAssetResponse struct {
	MetadataRevision int64  `json:"metadata_revision"`
	AvatarURL        string `json:"avatar_url"`
	BannerURL        string `json:"banner_url"`
}

func (s *Server) coserAssetUploadHandler(database *productdb.Database) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		if !s.Auth.AuthorizeRequest(request) {
			http.Error(response, "authentication required", http.StatusUnauthorized)
			return
		}
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", "POST")
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		coserUUID, kind, ok := parseCoserAssetUploadPath(request.URL.Path)
		if !ok {
			http.NotFound(response, request)
			return
		}
		request.Body = http.MaxBytesReader(response, request.Body, coserasset.MaxUploadBytes+1024*1024)
		if err := request.ParseMultipartForm(coserasset.MaxUploadBytes + 512*1024); err != nil {
			http.Error(response, "invalid managed image upload", http.StatusBadRequest)
			return
		}
		defer request.MultipartForm.RemoveAll()
		if len(request.MultipartForm.File) != 1 || len(request.MultipartForm.File["file"]) != 1 {
			http.Error(response, "exactly one managed image file is required", http.StatusBadRequest)
			return
		}
		expectedRevision, err := strconv.ParseInt(request.FormValue("expected_metadata_revision"), 10, 64)
		if err != nil || expectedRevision <= 0 {
			http.Error(response, "invalid expected revision", http.StatusBadRequest)
			return
		}
		file, _, err := request.FormFile("file")
		if err != nil {
			http.Error(response, "one managed image file is required", http.StatusBadRequest)
			return
		}
		defer file.Close()
		crop, focal, err := parseCoserFraming(request.MultipartForm, kind)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		roots, err := database.Operations().StorageRoots(request.Context())
		if err != nil {
			http.Error(response, "Coser metadata storage is unavailable", http.StatusServiceUnavailable)
			return
		}
		updated, err := (coserasset.Service{Database: database, Root: roots.CoserMetadataRoot}).Upload(request.Context(), coserasset.UploadInput{
			CoserUUID: coserUUID, ExpectedRevision: expectedRevision, Kind: kind, Reader: file,
			AvatarCrop: crop, BannerFocalPoint: focal,
		})
		if err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, productdb.ErrCoreMetadataRevisionConflict) {
				code = http.StatusConflict
			}
			_ = database.Operations().Audit(request.Context(), "COSER_ASSET_UPLOAD", "COSER", coserUUID, "FAILURE", "COSER_ASSET_UPLOAD_FAILED",
				map[string]any{"asset_kind": kind}, time.Now())
			http.Error(response, publicCoserAssetError(err), code)
			return
		}
		_ = database.Operations().Audit(request.Context(), "COSER_ASSET_UPLOAD", "COSER", coserUUID, "SUCCESS", "",
			map[string]any{"asset_kind": kind}, time.Now())
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(coserAssetResponse{
			MetadataRevision: updated.MetadataRevision,
			AvatarURL:        coserAssetURL(updated.UUID, updated.MetadataRevision, "avatar-480", updated.AvatarPath != ""),
			BannerURL:        coserAssetURL(updated.UUID, updated.MetadataRevision, "banner-1600", updated.BannerPath != ""),
		})
	})
}

func (s *Server) coserAssetResourceHandler(database *productdb.Database) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			response.Header().Set("Allow", "GET, HEAD")
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !s.Auth.AuthorizeRequest(request) {
			response.Header().Set("Cache-Control", "no-store")
			http.Error(response, "authentication required", http.StatusUnauthorized)
			return
		}
		coserUUID, revision, variant, ok := parseCoserAssetResourcePath(request.URL.Path)
		if !ok {
			http.NotFound(response, request)
			return
		}
		coser, err := database.CoreEntities().FindCoser(request.Context(), coserUUID)
		if err != nil || coser.MetadataRevision != revision {
			http.NotFound(response, request)
			return
		}
		original := coser.AvatarPath
		if strings.HasPrefix(variant, "banner-") {
			original = coser.BannerPath
		}
		if original == "" {
			http.NotFound(response, request)
			return
		}
		relative, err := coserasset.DerivativeRelative(original, variant)
		if err != nil {
			http.NotFound(response, request)
			return
		}
		roots, err := database.Operations().StorageRoots(request.Context())
		if err != nil {
			http.NotFound(response, request)
			return
		}
		file, info, err := coserasset.OpenManaged(roots.CoserMetadataRoot, coserUUID, relative)
		if err != nil {
			http.NotFound(response, request)
			return
		}
		defer file.Close()
		etag := fmt.Sprintf(`"%s-r%d-%s"`, coserUUID, revision, variant)
		response.Header().Set("ETag", etag)
		response.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		response.Header().Set("Content-Type", "image/jpeg")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Content-Disposition", "inline")
		if request.Header.Get("If-None-Match") == etag {
			response.WriteHeader(http.StatusNotModified)
			return
		}
		http.ServeContent(response, request, "", info.ModTime().UTC().Round(time.Second), file)
	})
}

func parseCoserAssetUploadPath(value string) (string, productdb.CoserAssetKind, bool) {
	if path.Clean(value) != value || !strings.HasPrefix(value, coserAssetUploadPrefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(value, coserAssetUploadPrefix), "/")
	if len(parts) != 2 {
		return "", "", false
	}
	if _, err := portableid.Parse(parts[0]); err != nil {
		return "", "", false
	}
	kind := productdb.CoserAssetKind(strings.ToUpper(parts[1]))
	return parts[0], kind, kind == productdb.CoserAssetAvatar || kind == productdb.CoserAssetBanner
}

func parseCoserAssetResourcePath(value string) (string, int64, string, bool) {
	if path.Clean(value) != value || !strings.HasPrefix(value, coserAssetResourcePrefix) {
		return "", 0, "", false
	}
	parts := strings.Split(strings.TrimPrefix(value, coserAssetResourcePrefix), "/")
	if len(parts) != 3 {
		return "", 0, "", false
	}
	if _, err := portableid.Parse(parts[0]); err != nil {
		return "", 0, "", false
	}
	revision, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || revision <= 0 {
		return "", 0, "", false
	}
	switch parts[2] {
	case "avatar-480", "banner-960", "banner-1600":
		return parts[0], revision, parts[2], true
	default:
		return "", 0, "", false
	}
}

func parseCoserFraming(form *multipart.Form, kind productdb.CoserAssetKind) (*coreentity.AvatarCrop, *coreentity.FocalPoint, error) {
	parse := func(name string) (float64, bool, error) {
		values := form.Value[name]
		if len(values) == 0 || values[0] == "" {
			return 0, false, nil
		}
		value, err := strconv.ParseFloat(values[0], 64)
		return value, true, err
	}
	if kind == productdb.CoserAssetAvatar {
		x, xSet, xErr := parse("crop_x")
		y, ySet, yErr := parse("crop_y")
		size, sizeSet, sizeErr := parse("crop_size")
		if xErr != nil || yErr != nil || sizeErr != nil || xSet != ySet || xSet != sizeSet {
			return nil, nil, errors.New("avatar crop requires valid x, y, and size")
		}
		if !xSet {
			return nil, nil, nil
		}
		crop := &coreentity.AvatarCrop{X: x, Y: y, Size: size}
		if x < 0 || y < 0 || size <= 0 || x+size > 1 || y+size > 1 {
			return nil, nil, errors.New("avatar crop must be a normalized in-bounds square")
		}
		return crop, nil, nil
	}
	x, xSet, xErr := parse("focal_x")
	y, ySet, yErr := parse("focal_y")
	if xErr != nil || yErr != nil || xSet != ySet {
		return nil, nil, errors.New("banner focal point requires valid x and y")
	}
	if !xSet {
		return nil, nil, nil
	}
	if x < 0 || x > 1 || y < 0 || y > 1 {
		return nil, nil, errors.New("banner focal point must be normalized")
	}
	return nil, &coreentity.FocalPoint{X: x, Y: y}, nil
}

func publicCoserAssetError(err error) string {
	if errors.Is(err, productdb.ErrCoreMetadataRevisionConflict) {
		return "core entity metadata revision conflict"
	}
	message := err.Error()
	for _, allowedPrefix := range []string{
		"Coser managed image must contain", "Coser managed asset is not a supported image",
		"Coser managed assets allow only", "Coser managed image exceeds",
		"Coser managed image must be static", "Coser managed asset could not be decoded",
		"Coser avatar crop", "Coser banner focal point",
	} {
		if strings.HasPrefix(message, allowedPrefix) {
			return message
		}
	}
	return "Coser managed image upload failed"
}

func coserAssetURL(uuid string, revision int64, variant string, available bool) string {
	if !available {
		return ""
	}
	return fmt.Sprintf("%s%s/%d/%s", coserAssetResourcePrefix, uuid, revision, variant)
}
