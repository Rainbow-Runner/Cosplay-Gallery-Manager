package productapi

import (
	"log/slog"

	"github.com/stashapp/stash/internal/browse"
)

func logVideoPlaybackRequested(value browse.VideoPlaybackStatus) {
	slog.Info("CGM_VIDEO_PLAYBACK_REQUESTED", "mode", value.Mode, "status", value.Status, "error_code", value.ErrorCode)
}
