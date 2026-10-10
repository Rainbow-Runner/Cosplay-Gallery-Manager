package productauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFirstReachableClientCanCompleteSetupAndSetupIsAtomic(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	body := `{"runtimeEnvironment":"DOCKER","locale":"zh-CN","timezone":"Asia/Shanghai","coserMetadataRoot":"/var/lib/cgm/cosers","backupRoot":"/var/lib/cgm/backups","password":"correct horse battery staple"}`
	request := httptest.NewRequest(http.MethodPost, "http://192.168.1.20:9999/setup/complete", strings.NewReader(body))
	request.RemoteAddr = "192.168.1.30:12000"
	response := httptest.NewRecorder()
	service.CompleteSetupHandler(SetupOptions{RuntimeEnvironment: "DOCKER", ValidateStorage: func(SetupInput) error { return nil }}).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("LAN Setup = %d %s", response.Code, response.Body.String())
	}

	status, err := service.SetupStatus(ctx)
	if err != nil || !status.Complete {
		t.Fatalf("Setup status = %+v, %v", status, err)
	}
	if err := service.VerifyPassword(ctx, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRequest(http.MethodPost, "http://nas.local:9999/setup/complete", strings.NewReader(body))
	second.RemoteAddr = "198.51.100.25:13000"
	secondResponse := httptest.NewRecorder()
	service.CompleteSetupHandler(SetupOptions{RuntimeEnvironment: "DOCKER", ValidateStorage: func(SetupInput) error { return nil }}).ServeHTTP(secondResponse, second)
	if secondResponse.Code != http.StatusBadRequest {
		t.Fatalf("second Setup = %d", secondResponse.Code)
	}
}

func TestSetupStatusDoesNotAdvertiseTicketFlow(t *testing.T) {
	service := testService(t)
	request := httptest.NewRequest(http.MethodGet, "http://192.168.1.20:9999/setup/status", nil)
	request.RemoteAddr = "192.168.1.30:12000"
	response := httptest.NewRecorder()
	service.SetupStatusHandler(SetupOptions{RuntimeEnvironment: "DOCKER", CoserMetadataRoot: "/var/lib/cgm/cosers", BackupRoot: "/var/lib/cgm/backups"}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Setup status = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "ticketRequired") {
		t.Fatalf("Setup status still exposes ticket mode: %s", response.Body.String())
	}
}
