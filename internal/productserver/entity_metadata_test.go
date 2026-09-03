package productserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/entitymetadata"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/productauth"
)

type serverEntityMetadataProvider struct{}

func (serverEntityMetadataProvider) Info() entitymetadata.ProviderInfo {
	return entitymetadata.ProviderInfo{Key: "fixture", Label: "Fixture"}
}
func (serverEntityMetadataProvider) Search(context.Context, entitymetadata.SearchRequest) ([]entitymetadata.Candidate, error) {
	return []entitymetadata.Candidate{{Ref: "article", DisplayName: "鸣潮", MatchQuality: 100}}, nil
}
func (serverEntityMetadataProvider) FetchNames(context.Context, entitymetadata.EntityKind, string) (entitymetadata.NameProfile, error) {
	return entitymetadata.NameProfile{DisplayName: "鸣潮", SourceURL: "https://example.invalid/article", PageID: "12", RevisionID: "34",
		Suggestions: []entitymetadata.AliasSuggestion{{Value: "Wuthering Waves", Category: "official", DefaultSelected: true}, {Value: "鳴潮", Category: "original"}}}, nil
}

func TestEntityMetadataEndpointIsAbsentWhenDisabledAndEmptyWhenAdapterRemoved(t *testing.T) {
	server := testServer(t)
	request := authenticatedRequest(t, server, http.MethodGet, "/manage/entity-metadata/providers", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled entity metadata endpoint = %d", response.Code)
	}
	server.Config.EntityMetadataScrapingEnabled = true
	request = authenticatedRequest(t, server, http.MethodGet, "/manage/entity-metadata/providers", nil)
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"providers":[]`) {
		t.Fatalf("provider-removed endpoint = %d %s", response.Code, response.Body.String())
	}
}

func TestReviewedEntityMetadataImportOnlyAppliesPreviewAliases(t *testing.T) {
	root := t.TempDir()
	database, err := productdb.Open(context.Background(), filepath.Join(root, "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	auth := productauth.NewForTesting(database, productauth.Argon2Parameters{MemoryKiB: 8 * 1024, Time: 1, Threads: 1, KeyLength: 32})
	config := Config{Listen: "127.0.0.1:9999", DatabasePath: database.Path(), CachePath: filepath.Join(root, "cache"), WorkerCount: 1, EntityMetadataScrapingEnabled: true}
	server, err := NewWithProviders(config, database, auth, nil, []entitymetadata.Provider{serverEntityMetadataProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	work, err := database.CoreEntities().CreateWork(context.Background(), productdb.CreateNamedEntityInput{Name: "鸣潮"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	prepareBody, _ := json.Marshal(entityMetadataPrepareRequest{ProviderKey: "fixture", Kind: entitymetadata.KindWork, EntityUUID: work.UUID, CandidateRef: "article"})
	prepare := authenticatedRequest(t, server, http.MethodPost, "/manage/entity-metadata/prepare", bytes.NewReader(prepareBody))
	prepare.Header.Set("Content-Type", "application/json")
	prepareResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(prepareResponse, prepare)
	if prepareResponse.Code != http.StatusOK {
		t.Fatalf("prepare = %d %s", prepareResponse.Code, prepareResponse.Body.String())
	}
	var preview entitymetadata.PreparedPreview
	if err := json.Unmarshal(prepareResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}

	tamperedBody, _ := json.Marshal(entityMetadataApplyRequest{Token: preview.Token, ExpectedMetadataRevision: work.MetadataRevision, Aliases: []string{"not offered"}})
	tampered := authenticatedRequest(t, server, http.MethodPost, "/manage/entity-metadata/apply", bytes.NewReader(tamperedBody))
	tampered.Header.Set("Content-Type", "application/json")
	tamperedResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(tamperedResponse, tampered)
	if tamperedResponse.Code != http.StatusBadRequest {
		t.Fatalf("tampered selection = %d %s", tamperedResponse.Code, tamperedResponse.Body.String())
	}

	applyBody, _ := json.Marshal(entityMetadataApplyRequest{Token: preview.Token, ExpectedMetadataRevision: work.MetadataRevision, Aliases: []string{"Wuthering Waves", "鳴潮"}})
	apply := authenticatedRequest(t, server, http.MethodPost, "/manage/entity-metadata/apply", bytes.NewReader(applyBody))
	apply.Header.Set("Content-Type", "application/json")
	applyResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(applyResponse, apply)
	if applyResponse.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", applyResponse.Code, applyResponse.Body.String())
	}
	updated, err := database.CoreEntities().ManageFind(context.Background(), "WORK", work.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.Aliases, []string{"Wuthering Waves", "鳴潮"}) || updated.MetadataRevision != work.MetadataRevision+1 {
		t.Fatalf("updated Work = %#v", updated)
	}
}
