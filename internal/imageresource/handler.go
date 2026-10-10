// Package imageresource serves an authorized original image through an opaque
// identity. It never exposes a physical media path to the browser.
package imageresource

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
	_ "golang.org/x/image/webp"
)

const (
	RoutePrefix             = "/resource/image/"
	staticOriginalMaxBytes  = int64(20 << 20)
	staticOriginalMaxPixels = 4096
)

type AccessResolver func(*http.Request) (productdb.ResourceAccess, error)

type Handler struct {
	Database     *productdb.Database
	Access       AccessResolver
	Materializer mediaaccess.Materializer
}

func (handler Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		response.Header().Set("Allow", "GET, HEAD")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if handler.Database == nil || handler.Access == nil {
		http.NotFound(response, request)
		return
	}
	access, err := handler.Access(request)
	if err != nil || !access.Authenticated {
		response.Header().Set("Cache-Control", "no-store")
		http.NotFound(response, request)
		return
	}
	uuid, revision, ok := parse(request.URL.Path)
	if !ok {
		http.NotFound(response, request)
		return
	}
	descriptor, err := handler.Database.MediaResources().AuthorizeDirectImage(request.Context(), access, uuid, revision)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	file, info, cleanup, err := handler.open(request, descriptor.Source)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	defer cleanup()
	mimeType, config, animatedWebP, err := inspect(file)
	if err != nil || !eligible(descriptor.MediaKind, mimeType, animatedWebP, info.Size(), config) {
		http.NotFound(response, request)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.NotFound(response, request)
		return
	}
	etag := `"` + descriptor.ItemUUID + "-r" + strconv.FormatInt(descriptor.ContentRevision, 10) + `-original"`
	response.Header().Set("ETag", etag)
	response.Header().Set("Cache-Control", "private, no-cache")
	response.Header().Set("Content-Type", mimeType)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("Content-Disposition", "inline")
	response.Header().Set("Accept-Ranges", "bytes")
	item := descriptor.ItemUUID
	if len(item) > 8 {
		item = item[:8]
	}
	slog.Debug("CGM_IMAGE_ORIGINAL_SERVED", "item", item, "revision", descriptor.ContentRevision, "media_kind", descriptor.MediaKind)
	if request.Header.Get("If-None-Match") == etag {
		response.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(response, request, "", info.ModTime(), file)
}

func (handler Handler) open(request *http.Request, source mediaaccess.Source) (*os.File, os.FileInfo, func(), error) {
	if source.Type == gallery.SourceTypeDirectory {
		file, info, err := mediaaccess.OpenDirectoryFile(source)
		if err != nil {
			return nil, nil, func() {}, err
		}
		return file, info, func() { _ = file.Close() }, nil
	}
	materialized, err := handler.Materializer.Open(request.Context(), source)
	if err != nil {
		return nil, nil, func() {}, err
	}
	file, err := os.Open(materialized.Path)
	if err != nil {
		_ = materialized.Close()
		return nil, nil, func() {}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		_ = materialized.Close()
		return nil, nil, func() {}, err
	}
	return file, info, func() { _ = file.Close(); _ = materialized.Close() }, nil
}

func inspect(file *os.File) (string, image.Config, bool, error) {
	sample := make([]byte, 64<<10)
	n, readErr := file.Read(sample)
	if readErr != nil && readErr != io.EOF {
		return "", image.Config{}, false, readErr
	}
	sample = sample[:n]
	mimeType := imageMIME(sample)
	if mimeType == "" {
		return "", image.Config{}, false, io.ErrUnexpectedEOF
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", image.Config{}, false, err
	}
	config, _, err := image.DecodeConfig(file)
	return mimeType, config, mimeType == "image/webp" && bytes.Contains(sample, []byte("ANIM")), err
}

func imageMIME(value []byte) string {
	switch {
	case len(value) >= 3 && bytes.Equal(value[:3], []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case len(value) >= 8 && bytes.Equal(value[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(value) >= 6 && (bytes.Equal(value[:6], []byte("GIF87a")) || bytes.Equal(value[:6], []byte("GIF89a"))):
		return "image/gif"
	case len(value) >= 12 && bytes.Equal(value[:4], []byte("RIFF")) && bytes.Equal(value[8:12], []byte("WEBP")):
		return "image/webp"
	default:
		return ""
	}
}

func eligible(kind gallery.MediaKind, mimeType string, animatedWebP bool, size int64, config image.Config) bool {
	if config.Width <= 0 || config.Height <= 0 {
		return false
	}
	if kind == gallery.MediaKindAnimatedImage {
		return mimeType == "image/gif" || (mimeType == "image/webp" && animatedWebP)
	}
	if kind != gallery.MediaKindStaticImage || size > staticOriginalMaxBytes || config.Width > staticOriginalMaxPixels || config.Height > staticOriginalMaxPixels {
		return false
	}
	return mimeType == "image/jpeg" || mimeType == "image/png" || (mimeType == "image/webp" && !animatedWebP)
}

func parse(value string) (string, int64, bool) {
	if !strings.HasPrefix(value, RoutePrefix) || path.Clean(value) != value {
		return "", 0, false
	}
	parts := strings.Split(strings.TrimPrefix(value, RoutePrefix), "/")
	if len(parts) != 3 || parts[2] != "original" {
		return "", 0, false
	}
	if _, err := portableid.Parse(parts[0]); err != nil {
		return "", 0, false
	}
	revision, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || revision <= 0 {
		return "", 0, false
	}
	return parts[0], revision, true
}
