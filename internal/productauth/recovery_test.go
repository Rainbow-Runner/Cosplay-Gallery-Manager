package productauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecoveryTokenResetsPasswordOnceAndRevokesSessions(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	if _, _, err := service.CreateRecoveryToken(ctx); err != ErrNotConfigured {
		t.Fatalf("pre-setup recovery = %v", err)
	}
	if err := service.CompleteSetup(ctx, SetupInput{RuntimeEnvironment: "NATIVE", Locale: "en-GB", Timezone: "UTC", CoserMetadataRoot: "/metadata", BackupRoot: "/backups", Password: "original password"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetTrustedMode(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	session, _, err := service.CreateSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := service.CreateRecoveryToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, expires, err := service.CreateRecoveryToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RecoverPassword(ctx, first, "replacement password"); err != ErrInvalidRecoveryToken {
		t.Fatalf("superseded token = %v", err)
	}
	if expires.Sub(service.now()) > 10*time.Minute || expires.Sub(service.now()) < 9*time.Minute {
		t.Fatalf("expiry = %v", expires)
	}
	request := httptest.NewRequest(http.MethodPost, "/session/recover", strings.NewReader(`{"token":"`+second+`","newPassword":"replacement password"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	service.RecoveryHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("recovery = %d %s", response.Code, response.Body.String())
	}
	if err := service.VerifyPassword(ctx, "replacement password"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecoverPassword(ctx, second, "another password"); err != ErrInvalidRecoveryToken {
		t.Fatalf("replay = %v", err)
	}
	if valid, err := service.AuthenticateToken(ctx, session); err != nil || valid {
		t.Fatalf("old session = %v, %v", valid, err)
	}
	status, err := service.Status(ctx)
	if err != nil || status.TrustedMode {
		t.Fatalf("trusted mode after recovery = %+v, %v", status, err)
	}
}

func TestRecoveryTokenExpires(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	if err := service.CompleteSetup(ctx, SetupInput{RuntimeEnvironment: "NATIVE", Locale: "en-GB", Timezone: "UTC", CoserMetadataRoot: "/metadata", BackupRoot: "/backups", Password: "original password"}); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return clock }
	token, _, err := service.CreateRecoveryToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(10 * time.Minute)
	if err := service.RecoverPassword(ctx, token, "replacement password"); err != ErrInvalidRecoveryToken {
		t.Fatalf("expired token = %v", err)
	}
	if err := service.VerifyPassword(ctx, "original password"); err != nil {
		t.Fatal(err)
	}
}
