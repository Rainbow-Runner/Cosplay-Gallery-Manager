package videoresource

import (
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
)

const RoutePrefix = "/resource/video/"

type AccessResolver func(*http.Request) (productdb.ResourceAccess, error)
type Handler struct {
	Database *productdb.Database
	Access   AccessResolver
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
	descriptor, err := handler.Database.MediaResources().AuthorizeDirectVideo(request.Context(), access, uuid, revision)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	file, info, err := mediaaccess.OpenDirectoryFile(descriptor.Source)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	defer file.Close()
	etag := `"` + descriptor.ItemUUID + "-r" + strconv.FormatInt(descriptor.ContentRevision, 10) + `-direct"`
	response.Header().Set("ETag", etag)
	response.Header().Set("Cache-Control", "private, no-cache")
	response.Header().Set("Content-Type", descriptor.MIMEType)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("Content-Disposition", "inline")
	response.Header().Set("Accept-Ranges", "bytes")
	// Video playback can legitimately outlive the server's ordinary request
	// write deadline. The client connection/context still cancels ServeContent.
	_ = http.NewResponseController(response).SetWriteDeadline(time.Time{})
	item := descriptor.ItemUUID
	if len(item) > 8 {
		item = item[:8]
	}
	slog.Debug("CGM_VIDEO_DIRECT_SERVED", "item", item, "revision", descriptor.ContentRevision)
	if request.Header.Get("If-None-Match") == etag {
		response.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(response, request, "", info.ModTime(), file)
}

func parse(value string) (string, int64, bool) {
	if !strings.HasPrefix(value, RoutePrefix) || path.Clean(value) != value {
		return "", 0, false
	}
	parts := strings.Split(strings.TrimPrefix(value, RoutePrefix), "/")
	if len(parts) != 3 || parts[2] != "direct" {
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
