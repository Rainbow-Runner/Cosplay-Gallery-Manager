package productserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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
