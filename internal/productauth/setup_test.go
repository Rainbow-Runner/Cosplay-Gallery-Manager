package productauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetupTicketIsSingleUseAndSetupIsAtomic(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	ticket, _, err := service.CreateSetupTicket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := service.ExchangeSetupTicket(ctx, ticket)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ExchangeSetupTicket(ctx, ticket); err != ErrInvalidSetupTicket {
		t.Fatalf("reused ticket error = %v", err)
	}
	if valid, err := service.AuthenticateSetupSession(ctx, session); err != nil || !valid {
		t.Fatalf("Setup Session = %v, %v", valid, err)
	}
	input := SetupInput{RuntimeEnvironment: "DOCKER", Locale: "zh-CN", Timezone: "Asia/Shanghai",
		CoserMetadataRoot: "/metadata/cosers", BackupRoot: "/backups", Password: "correct horse battery staple"}
	if err := service.CompleteSetup(ctx, input); err != nil {
		t.Fatal(err)
	}
	status, err := service.SetupStatus(ctx)
	if err != nil || !status.Complete {
		t.Fatalf("Setup status = %+v, %v", status, err)
	}
	if valid, err := service.AuthenticateSetupSession(ctx, session); err != nil || valid {
		t.Fatalf("Setup Session after completion = %v, %v", valid, err)
	}
	if _, _, err := service.CreateSetupTicket(ctx); err != ErrSetupComplete {
		t.Fatalf("ticket after Setup error = %v", err)
	}
	if err := service.VerifyPassword(ctx, input.Password); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteSetupRequiresExchangedCookieWhileLoopbackIsDirect(t *testing.T) {
	service := testService(t)
	body := `{"runtimeEnvironment":"NATIVE","locale":"en-GB","timezone":"UTC","coserMetadataRoot":"/metadata","backupRoot":"/backup","password":"correct horse battery staple"}`
	remote := httptest.NewRequest(http.MethodPost, "/setup/complete", strings.NewReader(body))
	remote.RemoteAddr = "192.0.2.10:12000"
	response := httptest.NewRecorder()
	service.CompleteSetupHandler(true).ServeHTTP(response, remote)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("remote Setup without ticket = %d", response.Code)
	}
	loopback := httptest.NewRequest(http.MethodPost, "/setup/complete", strings.NewReader(body))
	loopback.RemoteAddr = "127.0.0.1:12000"
	response = httptest.NewRecorder()
	service.CompleteSetupHandler(true).ServeHTTP(response, loopback)
	if response.Code != http.StatusNoContent {
		t.Fatalf("loopback Setup = %d %s", response.Code, response.Body.String())
	}
}
