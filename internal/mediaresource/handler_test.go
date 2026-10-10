package mediaresource

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

func TestHandlerRequiresAuthenticationScopeAndServesRangeWithoutPaths(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 16, 0, 0, 0, time.UTC)
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	galleryRecord, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Resource"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, galleryRecord.ID, productdb.CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: t.TempDir(), Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, galleryRecord.ID, source.ID, productdb.CreateItemInput{
		RelativePath: "photo.jpg", MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable,
		ProcessingState: gallery.ProcessingPending,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',content_rating='NON_ADULT',added_at_utc=? WHERE id=?`, now.Format(time.RFC3339Nano), galleryRecord.ID); err != nil {
		t.Fatal(err)
	}
	profile := mediaprocessing.DefaultProfileHash()
	cache := mediaprocessing.CacheWriter{Root: t.TempDir()}
	relative, err := cache.RelativePath(item.UUID, item.ContentRevision, mediaprocessing.VariantCard480, profile, "jpg")
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("0123456789")
	if _, _, err := cache.WriteAtomic(relative, func(writer io.Writer) error {
		_, err := writer.Write(content)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Derivatives().Publish(ctx, productdb.PublishDerivativeInput{
		ItemUUID: item.UUID, Variant: mediaprocessing.VariantCard480, CacheTier: mediaprocessing.CacheBase,
		ContentRevision: item.ContentRevision, ProfileHash: profile, CacheRelativePath: relative,
		MIMEType: "image/jpeg", ByteSize: int64(len(content)), Width: 480, Height: 640,
	}, now); err != nil {
		t.Fatal(err)
	}
	handler := Handler{Database: db, Cache: cache, Access: func(request *http.Request) (productdb.ResourceAccess, error) {
		return productdb.ResourceAccess{Authenticated: request.Header.Get("Authorization") == "session",
			Mode: productdb.ResourceBrowse, Scope: productdb.ResourceScope(request.Header.Get("X-Scope"))}, nil
	}}
	resourceURL := RoutePrefix + item.UUID + "/" + strconv.FormatInt(item.ContentRevision, 10) + "/" + profile + "/" + mediaprocessing.VariantCard480

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, resourceURL, nil))
	if unauthenticated.Code != http.StatusUnauthorized || unauthenticated.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unauthenticated response = %d %#v", unauthenticated.Code, unauthenticated.Header())
	}

	wrongScopeRequest := httptest.NewRequest(http.MethodGet, resourceURL, nil)
	wrongScopeRequest.Header.Set("Authorization", "session")
	wrongScopeRequest.Header.Set("X-Scope", string(productdb.ResourceScopeMagic))
	wrongScope := httptest.NewRecorder()
	handler.ServeHTTP(wrongScope, wrongScopeRequest)
	if wrongScope.Code != http.StatusNotFound {
		t.Fatalf("cross-scope status = %d", wrongScope.Code)
	}

	rangeRequest := httptest.NewRequest(http.MethodGet, resourceURL, nil)
	rangeRequest.Header.Set("Authorization", "session")
	rangeRequest.Header.Set("X-Scope", string(productdb.ResourceScopeList))
	rangeRequest.Header.Set("Range", "bytes=2-5")
	ranged := httptest.NewRecorder()
	handler.ServeHTTP(ranged, rangeRequest)
	if ranged.Code != http.StatusPartialContent || ranged.Body.String() != "2345" || ranged.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("range response = %d %q %#v", ranged.Code, ranged.Body.String(), ranged.Header())
	}
	etag := ranged.Header().Get("ETag")
	if etag == "" || ranged.Header().Get("Accept-Ranges") != "bytes" || ranged.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("resource headers = %#v", ranged.Header())
	}

	conditionalRequest := httptest.NewRequest(http.MethodGet, resourceURL, nil)
	conditionalRequest.Header.Set("Authorization", "session")
	conditionalRequest.Header.Set("X-Scope", string(productdb.ResourceScopeList))
	conditionalRequest.Header.Set("If-None-Match", etag)
	conditional := httptest.NewRecorder()
	handler.ServeHTTP(conditional, conditionalRequest)
	if conditional.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d", conditional.Code)
	}
	currentGallery, err := db.Galleries().Find(ctx, galleryRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	previewURL := PreviewRoutePrefix + galleryRecord.SetID + "/" + strconv.FormatInt(currentGallery.ScrubberRevision, 10) + "/0"
	previewRequest := httptest.NewRequest(http.MethodGet, previewURL, nil)
	previewRequest.Header.Set("Authorization", "session")
	previewRequest.Header.Set("X-Scope", string(productdb.ResourceScopeList))
	preview := httptest.NewRecorder()
	handler.ServeHTTP(preview, previewRequest)
	if preview.Code != http.StatusOK || preview.Body.String() != string(content) {
		t.Fatalf("ordinal preview = %d %q", preview.Code, preview.Body.String())
	}
	staleRequest := httptest.NewRequest(http.MethodGet, PreviewRoutePrefix+galleryRecord.SetID+"/0/0", nil)
	staleRequest.Header.Set("Authorization", "session")
	staleRequest.Header.Set("X-Scope", string(productdb.ResourceScopeList))
	stale := httptest.NewRecorder()
	handler.ServeHTTP(stale, staleRequest)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale ordinal preview status = %d", stale.Code)
	}
	for _, value := range []string{source.Path, relative} {
		if value != "" && containsResponseValue(ranged, value) {
			t.Fatalf("response leaked path %q", value)
		}
	}
}

func TestHandlerRejectsNonCanonicalOpaqueIdentity(t *testing.T) {
	handler := Handler{}
	for _, requestPath := range []string{
		RoutePrefix + "not-a-uuid/1/profile/CARD_480",
		RoutePrefix + "00000000-0000-4000-8000-000000000000/0/profile/CARD_480",
		RoutePrefix + "00000000-0000-4000-8000-000000000000/1/profile/../CARD_480",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("path %q status = %d", requestPath, response.Code)
		}
	}
}

func containsResponseValue(response *httptest.ResponseRecorder, value string) bool {
	if response.Body != nil && strings.Contains(string(response.Body.Bytes()), value) {
		return true
	}
	for _, values := range response.Header() {
		for _, headerValue := range values {
			if strings.Contains(headerValue, value) {
				return true
			}
		}
	}
	return false
}
