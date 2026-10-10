package productserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
)

// All startup and owner-requested probes share one slot and a cooldown.
func (s *Server) startHardwareProbe() bool {
	s.hardwareProbeMu.Lock()
	defer s.hardwareProbeMu.Unlock()
	if s.hardwareProbeRunning || time.Since(s.hardwareProbeLastStarted) < 30*time.Second {
		return false
	}
	s.hardwareProbeRunning = true
	s.hardwareProbeLastStarted = time.Now()
	s.VideoHardware.Set(mediaprocessing.HardwareAccelerationStatus{ProbeState: mediaprocessing.HardwareProbePending})
	go func() {
		defer func() { s.hardwareProbeMu.Lock(); s.hardwareProbeRunning = false; s.hardwareProbeMu.Unlock() }()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		status := mediaprocessing.ProbeHardwareAcceleration(ctx, s.VideoTools.FFmpeg.Path)
		s.VideoHardware.Set(status)
		slog.Info("CGM_VIDEO_HARDWARE_PROBE_COMPLETED", mediaprocessing.HardwareProbeLogFields(status)...)
	}()
	return true
}

func (s *Server) hardwareProbeHandler(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.startHardwareProbe() {
		response.Header().Set("Retry-After", "30")
		http.Error(response, "Hardware probe running or cooling down", http.StatusTooManyRequests)
		return
	}
	response.WriteHeader(http.StatusAccepted)
}
