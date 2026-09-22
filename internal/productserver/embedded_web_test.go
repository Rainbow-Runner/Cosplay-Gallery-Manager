//go:build cgm_web_embed

package productserver

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestProductBuildServesEmbeddedUIWithoutWebRoot(t *testing.T) {
	server := testServer(t)
	route := httptest.NewRecorder()
	server.Handler.ServeHTTP(route, httptest.NewRequest(http.MethodGet, "/manage/settings", nil))
	if route.Code != http.StatusOK || !strings.Contains(route.Body.String(), "<title>Cosplay Gallery Manager</title>") {
		t.Fatalf("embedded route = %d %q", route.Code, route.Body.String())
	}
	if got := route.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
		t.Fatalf("embedded route CSP = %q", got)
	}

	match := regexp.MustCompile(`src="/(assets/[^"]+\.js)"`).FindStringSubmatch(route.Body.String())
	if len(match) != 2 {
		t.Fatalf("embedded index has no hashed script: %q", route.Body.String())
	}
	asset := httptest.NewRecorder()
	server.Handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/"+match[1], nil))
	if asset.Code != http.StatusOK || asset.Body.Len() == 0 {
		t.Fatalf("embedded asset = %d (%d bytes)", asset.Code, asset.Body.Len())
	}
	if got := asset.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("embedded asset Cache-Control = %q", got)
	}
}
