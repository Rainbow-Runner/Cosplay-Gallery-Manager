package productserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/cosermetadata"
	"github.com/stashapp/stash/internal/entitymetadata"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/productauth"
)

func TestDockerConfigEnablesBothOwnerTriggeredMetadataTools(t *testing.T) {
	config, err := LoadConfig(filepath.Join("..", "..", "docker", "cgm", "cgm.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !config.MetadataScrapingEnabled || !config.EntityMetadataScrapingEnabled {
		t.Fatal("Docker must enable Coser profiles and Work/Character names")
	}
	if config.RuntimeEnvironment != "DOCKER" {
		t.Fatal("expected Docker runtime")
	}
	root := t.TempDir()
	config.DatabasePath = filepath.Join(root, "product.sqlite")
	config.CachePath = filepath.Join(root, "cache")
	database, err := productdb.Open(context.Background(), config.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	auth := productauth.NewForTesting(database, productauth.Argon2Parameters{MemoryKiB: 8 * 1024, Time: 1, Threads: 1, KeyLength: 32})
	server, err := NewWithProviders(config, database, auth, []cosermetadata.Provider{serverMetadataProvider{}}, []entitymetadata.Provider{serverEntityMetadataProvider{}})
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	for _, path := range []string{"/manage/coser-metadata/providers", "/manage/entity-metadata/providers"} {
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, authenticatedRequest(t, server, http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"key":"fixture"`) {
			t.Fatalf("%s = %d %s", path, response.Code, response.Body.String())
		}
		response = httptest.NewRecorder()
		server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s = %d", path, response.Code)
		}
	}
}
