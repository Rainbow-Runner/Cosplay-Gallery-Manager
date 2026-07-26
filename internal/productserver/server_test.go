package productserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/productauth"
)

func TestHealthIsPublicButGraphQLRequiresSession(t *testing.T) {
	server := testServer(t)
	health := httptest.NewRecorder()
	server.Handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusNoContent || health.Body.Len() != 0 {
		t.Fatalf("health = %d %q", health.Code, health.Body.String())
	}
	graphql := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query":"{ browseUISettings { settingsRevision } }"}`))
	request.Header.Set("Content-Type", "application/json")
	server.Handler.ServeHTTP(graphql, request)
	if graphql.Code != http.StatusUnauthorized {
		t.Fatalf("GraphQL status = %d", graphql.Code)
	}
}

func TestSessionAuthorizesGraphQLAndOriginIsChecked(t *testing.T) {
	server := testServer(t)
	if err := server.Auth.ConfigurePassword(context.Background(), "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	loginRequest := httptest.NewRequest(http.MethodPost, "/session/login", strings.NewReader(`{"password":"correct horse battery staple"}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(loginResponse, loginRequest)
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login status/cookies = %d/%d", loginResponse.Code, len(cookies))
	}
	request := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query":"{ browseUISettings { settingsRevision } }"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookies[0])
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"settingsRevision":1`) {
		t.Fatalf("GraphQL = %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "http://example.test/graphql", strings.NewReader(`{"query":"{ browseUISettings { settingsRevision } }"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://evil.test")
	request.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d", response.Code)
	}
}

func TestSPAHandlerServesHashedAssetsAndRouteFallback(t *testing.T) {
	fileSystem := fstest.MapFS{
		"index.html":            &fstest.MapFile{Data: []byte("<title>Cosplay Gallery Manager</title>")},
		"assets/app-1234.js":    &fstest.MapFile{Data: []byte("application")},
		"assets/style-1234.css": &fstest.MapFile{Data: []byte("stylesheet")},
	}
	handler := spaHandlerFS(fileSystem)

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/app-1234.js", nil))
	if asset.Code != http.StatusOK || asset.Body.String() != "application" {
		t.Fatalf("asset = %d %q", asset.Code, asset.Body.String())
	}
	if got := asset.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("asset Cache-Control = %q", got)
	}

	route := httptest.NewRecorder()
	handler.ServeHTTP(route, httptest.NewRequest(http.MethodGet, "/gallery/example", nil))
	if route.Code != http.StatusOK || !strings.Contains(route.Body.String(), "Cosplay Gallery Manager") {
		t.Fatalf("route fallback = %d %q", route.Code, route.Body.String())
	}
	if got := route.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("route Cache-Control = %q", got)
	}

	if err := fstest.TestFS(fileSystem, "index.html", "assets/app-1234.js", "assets/style-1234.css"); err != nil {
		t.Fatal(err)
	}
}

func TestSPAHandlerDoesNotExposeFilesOutsideItsRoot(t *testing.T) {
	handler := spaHandlerFS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("product entry")},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/../ui/v2.5/build/index.html", nil))
	if response.Code != http.StatusOK || response.Body.String() != "product entry" {
		t.Fatalf("legacy path did not stay inside product SPA: %d %q", response.Code, response.Body.String())
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	database, err := productdb.Open(context.Background(), filepath.Join(root, "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	auth := productauth.NewForTesting(database, productauth.Argon2Parameters{MemoryKiB: 8 * 1024, Time: 1, Threads: 1, KeyLength: 32})
	server := New(Config{Listen: "127.0.0.1:9999", DatabasePath: database.Path(), CachePath: filepath.Join(root, "cache"), WorkerCount: 1}, database, auth)
	t.Cleanup(func() { _ = server.Close() })
	return server
}
