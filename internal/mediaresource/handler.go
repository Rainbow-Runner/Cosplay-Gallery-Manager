// Package mediaresource serves only opaque, database-authorized generated
// resources. It has no route capable of accepting or exposing a physical
// source path.
package mediaresource

import (
	"errors"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
)

const (
	RoutePrefix        = "/resource/item/"
	PreviewRoutePrefix = "/resource/gallery-preview/"
)

type AccessResolver func(*http.Request) (productdb.ResourceAccess, error)

type Handler struct {
	Database *productdb.Database
	Cache    mediaprocessing.CacheWriter
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
		http.Error(response, "authentication required", http.StatusUnauthorized)
		return
	}
	var descriptor productdb.MediaResourceDescriptor
	switch {
	case strings.HasPrefix(request.URL.Path, RoutePrefix):
		identity, ok := parseIdentity(request.URL.Path)
		if !ok {
			http.NotFound(response, request)
			return
		}
		descriptor, err = handler.Database.MediaResources().AuthorizeDerivative(request.Context(), access,
			identity.itemUUID, identity.variant, identity.contentRevision, identity.profileHash)
	case strings.HasPrefix(request.URL.Path, PreviewRoutePrefix):
		identity, ok := parsePreviewIdentity(request.URL.Path)
		if !ok {
			http.NotFound(response, request)
			return
		}
		descriptor, err = handler.Database.MediaResources().AuthorizeGalleryScrubber(request.Context(), access,
			identity.setID, identity.revision, identity.ordinal)
	default:
		http.NotFound(response, request)
		return
	}
	if err != nil {
		if errors.Is(err, productdb.ErrScrubberRevisionConflict) {
			response.Header().Set("Cache-Control", "no-store")
			http.Error(response, "preview revision changed", http.StatusConflict)
			return
		}
		// Forbidden and unknown resources deliberately have the same response.
		http.NotFound(response, request)
		return
	}
	file, info, err := handler.Cache.OpenGenerated(descriptor.CacheRelativePath)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	defer file.Close()
	// Opening the authorized file proves a real cache hit. LRU bookkeeping is
	// best-effort so a transient database write failure never breaks viewing.
	_ = handler.Database.Derivatives().Touch(request.Context(), descriptor.ItemUUID, descriptor.Variant,
		descriptor.ContentRevision, descriptor.ProfileHash, time.Now())
	etag := `"` + descriptor.ItemUUID + "-r" + strconv.FormatInt(descriptor.ContentRevision, 10) + "-" + descriptor.ProfileHash + "-" + descriptor.Variant + `"`
	response.Header().Set("ETag", etag)
	response.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	response.Header().Set("Content-Type", descriptor.MIMEType)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("Content-Disposition", "inline")
	if request.Header.Get("If-None-Match") == etag {
		response.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(response, request, "", info.ModTime().UTC().Round(time.Second), file)
}

type previewIdentity struct {
	setID    string
	revision int64
	ordinal  int
}

func parsePreviewIdentity(urlPath string) (previewIdentity, bool) {
	if !strings.HasPrefix(urlPath, PreviewRoutePrefix) || path.Clean(urlPath) != urlPath {
		return previewIdentity{}, false
	}
	parts := strings.Split(strings.TrimPrefix(urlPath, PreviewRoutePrefix), "/")
	if len(parts) != 3 {
		return previewIdentity{}, false
	}
	if _, err := portableid.Parse(parts[0]); err != nil {
		return previewIdentity{}, false
	}
	revision, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || revision < 0 {
		return previewIdentity{}, false
	}
	ordinal, err := strconv.Atoi(parts[2])
	if err != nil || ordinal < 0 || ordinal > 999 {
		return previewIdentity{}, false
	}
	return previewIdentity{setID: parts[0], revision: revision, ordinal: ordinal}, true
}

type resourceIdentity struct {
	itemUUID        string
	contentRevision int64
	profileHash     string
	variant         string
}

func parseIdentity(urlPath string) (resourceIdentity, bool) {
	if !strings.HasPrefix(urlPath, RoutePrefix) || path.Clean(urlPath) != urlPath {
		return resourceIdentity{}, false
	}
	parts := strings.Split(strings.TrimPrefix(urlPath, RoutePrefix), "/")
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return resourceIdentity{}, false
	}
	if _, err := portableid.Parse(parts[0]); err != nil {
		return resourceIdentity{}, false
	}
	revision, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || revision <= 0 || len(parts[2]) > 200 || len(parts[3]) > 100 ||
		strings.ContainsAny(parts[2]+parts[3], "\\?&#%") {
		return resourceIdentity{}, false
	}
	return resourceIdentity{itemUUID: parts[0], contentRevision: revision, profileHash: parts[2], variant: parts[3]}, true
}
