package productserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/processingworker"
)

func TestCacheCleanupAPIRequiresAuthOriginPasswordConfirmationAndRechecksPreview(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	now := time.Now()
	if err := os.MkdirAll(server.Config.CachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := server.Auth.ConfigurePassword(ctx, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	g, err := server.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Cache"}, now)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := t.TempDir()
	for _, name := range []string{"photo.jpg", ".cosplay.json"} {
		if err := os.WriteFile(filepath.Join(sourceRoot, name), []byte("user data must stay"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	source, err := server.Database.Galleries().AddSource(ctx, g.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := server.Database.Galleries().AddItem(ctx, g.ID, source.ID, productdb.CreateItemInput{RelativePath: "photo.jpg", MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage, ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingPending}, now)
	if err != nil {
		t.Fatal(err)
	}
	cache := mediaprocessing.CacheWriter{Root: server.Config.CachePath}
	paths := []string{}
	for _, profile := range []string{"old", "new"} {
		relative, _ := cache.RelativePath(item.UUID, 1, mediaprocessing.VariantCard480, profile, "jpg")
		paths = append(paths, relative)
		_, size, err := cache.WriteAtomic(relative, func(w io.Writer) error { _, e := w.Write([]byte("generated")); return e })
		if err != nil {
			t.Fatal(err)
		}
		if _, err := server.Database.Derivatives().Publish(ctx, productdb.PublishDerivativeInput{ItemUUID: item.UUID, Variant: mediaprocessing.VariantCard480, CacheTier: mediaprocessing.CacheBase, ContentRevision: 1, ProfileHash: profile, CacheRelativePath: relative, MIMEType: "image/jpeg", ByteSize: size}, now); err != nil {
			t.Fatal(err)
		}
	}
	request := func(method, body, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, cacheCleanupReviewPath, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		server.Handler.ServeHTTP(w, r)
		return w
	}
	if response := request("GET", "", "", nil); response.Code != 401 {
		t.Fatalf("unauthenticated=%d", response.Code)
	}
	cookie := loginTestOwner(t, server)
	response := request("GET", "", "", cookie)
	if response.Code != 200 {
		t.Fatalf("review=%d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "items/") || strings.Contains(response.Body.String(), sourceRoot) || strings.Contains(response.Body.String(), server.Config.CachePath) {
		t.Fatal("preview leaks paths or caches response")
	}
	var review processingworker.CacheCleanupReview
	if err := json.Unmarshal(response.Body.Bytes(), &review); err != nil || len(review.Candidates) != 1 {
		t.Fatalf("review=%+v %v", review, err)
	}
	body := func(password, confirmation, id string) string {
		value, _ := json.Marshal(map[string]any{"ids": []string{id}, "password": password, "confirmation": confirmation})
		return string(value)
	}
	id := review.Candidates[0].ID
	if response := request("POST", body("correct horse battery staple", "CLEAN", id), "http://evil.invalid", cookie); response.Code != 403 {
		t.Fatalf("origin=%d", response.Code)
	}
	if response := request("POST", body("correct horse battery staple", "WRONG", id), "", cookie); response.Code != 400 {
		t.Fatalf("confirmation=%d", response.Code)
	}
	if response := request("POST", body("wrong", "CLEAN", id), "", cookie); response.Code != 403 {
		t.Fatalf("password=%d", response.Code)
	}
	if response := request("POST", body("correct horse battery staple", "CLEAN", strings.Repeat("0", 64)), "", cookie); response.Code != 409 {
		t.Fatalf("stale=%d", response.Code)
	}
	response = request("POST", body("correct horse battery staple", "CLEAN", id), "", cookie)
	if response.Code != 200 {
		t.Fatalf("cleanup=%d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cache.Root, paths[0])); !os.IsNotExist(err) {
		t.Fatal("obsolete base remains")
	}
	if _, err := os.Stat(filepath.Join(cache.Root, paths[1])); err != nil {
		t.Fatal("current base deleted")
	}
	for _, name := range []string{"photo.jpg", ".cosplay.json"} {
		value, err := os.ReadFile(filepath.Join(sourceRoot, name))
		if err != nil || string(value) != "user data must stay" {
			t.Fatal("source data changed")
		}
	}
	audit, err := server.Database.Operations().AuditPage(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(audit)
	if !strings.Contains(string(encoded), "CACHE_CLEANUP") || strings.Contains(string(encoded), sourceRoot) || strings.Contains(string(encoded), "correct horse") || strings.Contains(string(encoded), "items/") {
		t.Fatal("missing audit or leaked data")
	}
}
