package productauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/persistence/productdb"
)

var testParameters = Argon2Parameters{MemoryKiB: 8 * 1024, Time: 1, Threads: 1, KeyLength: 32}

func TestPasswordSessionAndRevocationLifecycle(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	if err := service.ConfigurePassword(ctx, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := service.ConfigurePassword(ctx, "replacement password"); err != ErrAlreadyConfigured {
		t.Fatalf("second ConfigurePassword error = %v", err)
	}
	if err := service.VerifyPassword(ctx, "incorrect password"); err != ErrInvalidPassword {
		t.Fatalf("incorrect password error = %v", err)
	}
	token, _, err := service.CreateSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated, err := service.AuthenticateToken(ctx, token); err != nil || !authenticated {
		t.Fatalf("AuthenticateToken = %v, %v", authenticated, err)
	}
	if err := service.ChangePassword(ctx, "correct horse battery staple", "a different secure password"); err != nil {
		t.Fatal(err)
	}
	if authenticated, err := service.AuthenticateToken(ctx, token); err != nil || authenticated {
		t.Fatalf("old Session after password change = %v, %v", authenticated, err)
	}
}

func TestTrustedModeRequiresPasswordAndRevision(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	if _, err := service.SetTrustedMode(ctx, 1, true); err != ErrNotConfigured {
		t.Fatalf("SetTrustedMode before Setup = %v", err)
	}
	if err := service.ConfigurePassword(ctx, "a secure local password"); err != nil {
		t.Fatal(err)
	}
	status, err := service.SetTrustedMode(ctx, 1, true)
	if err != nil || !status.TrustedMode || status.Revision != 2 {
		t.Fatalf("trusted status = %+v, %v", status, err)
	}
	if _, err := service.SetTrustedMode(ctx, 1, false); err != ErrRevisionConflict {
		t.Fatalf("stale revision error = %v", err)
	}
	if authenticated, err := service.AuthenticateToken(ctx, ""); err != nil || !authenticated {
		t.Fatalf("trusted request = %v, %v", authenticated, err)
	}
}

func TestLoginCookieIsHttpOnlyStrictAndRevocable(t *testing.T) {
	service := testService(t)
	if err := service.ConfigurePassword(context.Background(), "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/session/login", strings.NewReader(`{"password":"correct horse battery staple"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	service.LoginHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, body = %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Name != CookieName {
		t.Fatalf("unexpected Session cookie: %+v", cookies)
	}
	authorized := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	authorized.AddCookie(cookies[0])
	if !service.AuthorizeRequest(authorized) {
		t.Fatal("new Session cookie was not authorized")
	}
	logout := httptest.NewRequest(http.MethodPost, "/session/logout", nil)
	logout.AddCookie(cookies[0])
	service.LogoutHandler().ServeHTTP(httptest.NewRecorder(), logout)
	if service.AuthorizeRequest(authorized) {
		t.Fatal("revoked Session cookie remained authorized")
	}
}

func TestLoginRateLimitAppliesAfterFiveFailures(t *testing.T) {
	service := testService(t)
	if err := service.ConfigurePassword(context.Background(), "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 6; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/session/login", strings.NewReader(`{"password":"wrong password"}`))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "192.0.2.44:1234"
		response := httptest.NewRecorder()
		service.LoginHandler().ServeHTTP(response, request)
		if attempt <= 5 && response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d", attempt, response.Code)
		}
		if attempt == 6 && response.Code != http.StatusTooManyRequests {
			t.Fatalf("rate-limited status = %d", response.Code)
		}
	}
}

func testService(t *testing.T) *Service {
	t.Helper()
	database, err := productdb.Open(context.Background(), filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return NewForTesting(database, testParameters)
}
