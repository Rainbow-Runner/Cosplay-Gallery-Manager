package productserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestHardwareProbeRequiresSessionAndSharesCooldown(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/manage/video-hardware/probe", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", response.Code)
	}
	request.Header.Set("Origin", "https://untrusted.example")
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin = %d", response.Code)
	}
	request.Header.Del("Origin")
	// Isolate scheduler assertions from the real startup probe's goroutine.
	server = &Server{VideoHardware: mediaprocessing.NewHardwareAccelerationMonitor()}
	server.hardwareProbeMu.Lock()
	server.hardwareProbeRunning = true
	server.hardwareProbeMu.Unlock()
	if server.startHardwareProbe() {
		t.Fatal("overlapping probe accepted")
	}
	server.hardwareProbeMu.Lock()
	server.hardwareProbeRunning = false
	server.hardwareProbeLastStarted = time.Now()
	server.hardwareProbeMu.Unlock()
	response = httptest.NewRecorder()
	server.hardwareProbeHandler(response, request)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("cooldown = %d", response.Code)
	}
	if response.Header().Get("Retry-After") != "30" {
		t.Fatal("missing cooldown hint")
	}
	response = httptest.NewRecorder()
	server.hardwareProbeHandler(response, httptest.NewRequest(http.MethodGet, "/manage/video-hardware/probe", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", response.Code)
	}
	server.hardwareProbeMu.Lock()
	server.hardwareProbeLastStarted = time.Time{}
	server.hardwareProbeMu.Unlock()
	response = httptest.NewRecorder()
	server.hardwareProbeHandler(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("fresh probe = %d", response.Code)
	}
	if server.startHardwareProbe() {
		t.Fatal("immediate second probe accepted")
	}
}
