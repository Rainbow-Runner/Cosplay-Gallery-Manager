package productserver

import (
	"bytes"
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

	"github.com/stashapp/stash/internal/cosermetadata"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/productauth"
)

type serverMetadataProvider struct{ image []byte }

func (serverMetadataProvider) Info() cosermetadata.ProviderInfo {
	return cosermetadata.ProviderInfo{Key: "fixture", Label: "Fixture"}
}
func (serverMetadataProvider) Search(context.Context, string) ([]cosermetadata.Candidate, error) {
	return []cosermetadata.Candidate{{Ref: "7", DisplayName: "Example", MatchQuality: 100}}, nil
}
func (serverMetadataProvider) FetchProfile(context.Context, string) (cosermetadata.Profile, error) {
	return cosermetadata.Profile{DisplayName: "Example", Avatar: &cosermetadata.RemoteAsset{Ref: "avatar"}, Accounts: []cosermetadata.SocialAccount{{PlatformKey: "twitter", Label: "X", Handle: "example", URL: "https://twitter.com/example"}}}, nil
}
func (p serverMetadataProvider) OpenAsset(context.Context, string) (cosermetadata.Asset, error) {
	return cosermetadata.Asset{Reader: io.NopCloser(bytes.NewReader(p.image)), ContentType: "image/jpeg", ByteSize: int64(len(p.image))}, nil
}

func TestMetadataEndpointIsAbsentWhenDisabledOrProviderRemoved(t *testing.T) {
	server := testServer(t)
	request := authenticatedRequest(t, server, http.MethodGet, "/manage/coser-metadata/providers", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled metadata endpoint = %d", response.Code)
	}
	server.Config.MetadataScrapingEnabled = true
	request = authenticatedRequest(t, server, http.MethodGet, "/manage/coser-metadata/providers", nil)
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"providers":[]`) {
		t.Fatalf("provider-removed metadata endpoint = %d %s", response.Code, response.Body.String())
	}
}

func TestReviewedMetadataImportUsesManagedAssetAndSocialStore(t *testing.T) {
	root := t.TempDir()
	database, err := productdb.Open(context.Background(), filepath.Join(root, "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	auth := productauth.NewForTesting(database, productauth.Argon2Parameters{MemoryKiB: 8 * 1024, Time: 1, Threads: 1, KeyLength: 32})
	config := Config{Listen: "127.0.0.1:9999", DatabasePath: database.Path(), CachePath: filepath.Join(root, "cache"), WorkerCount: 1, MetadataScrapingEnabled: true}
	server, err := NewWithMetadata(config, database, auth, serverMetadataProvider{image: testJPEG(t, 4, 4)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if err := auth.ConfigurePassword(context.Background(), "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	coserRoot, backupRoot := filepath.Join(root, "cosers"), filepath.Join(root, "backups")
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(context.Background(), `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, coserRoot, backupRoot, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	coser, err := database.CoreEntities().CreateCoser(context.Background(), productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Example"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	prepare := authenticatedRequest(t, server, http.MethodPost, "/manage/coser-metadata/prepare", strings.NewReader(`{"provider_key":"fixture","coser_uuid":"`+coser.UUID+`","candidate_ref":"7"}`))
	prepare.Header.Set("Content-Type", "application/json")
	prepareResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(prepareResponse, prepare)
	if prepareResponse.Code != http.StatusOK {
		t.Fatalf("prepare = %d %s", prepareResponse.Code, prepareResponse.Body.String())
	}
	var preview struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(prepareResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	applyBody, _ := json.Marshal(metadataApplyRequest{Token: preview.Token, ExpectedMetadataRevision: coser.MetadataRevision, ImportAvatar: true, AccountURLs: []string{"https://twitter.com/example"}})
	apply := authenticatedRequest(t, server, http.MethodPost, "/manage/coser-metadata/apply", bytes.NewReader(applyBody))
	apply.Header.Set("Content-Type", "application/json")
	applyResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(applyResponse, apply)
	if applyResponse.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", applyResponse.Code, applyResponse.Body.String())
	}
	updated, err := database.CoreEntities().ManageFind(context.Background(), "COSER", coser.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AvatarPath == "" || len(updated.SocialAccounts) != 1 || updated.MetadataRevision != coser.MetadataRevision+1 {
		t.Fatalf("updated Coser = %#v", updated)
	}
}

func authenticatedRequest(t *testing.T, server *Server, method, target string, body io.Reader) *http.Request {
	t.Helper()
	if err := server.Auth.ConfigurePassword(context.Background(), "correct horse battery staple"); err != nil && !strings.Contains(err.Error(), "already") {
		t.Fatal(err)
	}
	login := httptest.NewRequest(http.MethodPost, "/session/login", strings.NewReader(`{"password":"correct horse battery staple"}`))
	login.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(loginResponse, login)
	request := httptest.NewRequest(method, target, body)
	for _, cookie := range loginResponse.Result().Cookies() {
		request.AddCookie(cookie)
	}
	return request
}
