package productserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortablePackagesListAndCheckAreAuthenticatedAndConfined(t *testing.T) {
	server := testServer(t)
	if err := server.Auth.ConfigurePassword(context.Background(), "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bad.zip"), []byte("not a portable zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.zip")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.zip")); err != nil {
		t.Fatal(err)
	}
	handler := server.portablePackagesHandler(root)
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/manage/portable/packages", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized list = %d", unauthorized.Code)
	}
	cookie := loginTestOwner(t, server)
	listRequest := httptest.NewRequest(http.MethodGet, "/manage/portable/packages", nil)
	listRequest.AddCookie(cookie)
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, listRequest)
	var listing struct {
		Packages []portablePackageEntry `json:"packages"`
	}
	if listed.Code != http.StatusOK || json.Unmarshal(listed.Body.Bytes(), &listing) != nil || len(listing.Packages) != 1 || listing.Packages[0].Name != "bad.zip" || listing.Packages[0].CheckStatus != "UNRECOGNIZED" {
		t.Fatalf("listed packages = %d %s", listed.Code, listed.Body.String())
	}
	for _, name := range []string{"../outside.zip", "linked.zip"} {
		request := httptest.NewRequest(http.MethodPost, "/manage/portable/packages", strings.NewReader(`{"name":"`+name+`"}`))
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("package %q = %d", name, response.Code)
		}
	}
	checkRequest := httptest.NewRequest(http.MethodPost, "/manage/portable/packages", strings.NewReader(`{"name":"bad.zip"}`))
	checkRequest.AddCookie(cookie)
	checked := httptest.NewRecorder()
	handler.ServeHTTP(checked, checkRequest)
	var result portablePackageEntry
	if checked.Code != http.StatusOK || json.Unmarshal(checked.Body.Bytes(), &result) != nil || result.CheckStatus != "INVALID" {
		t.Fatalf("checked package = %d %s", checked.Code, checked.Body.String())
	}
}
