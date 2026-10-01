package productapi

import (
	"context"
	"errors"

	"github.com/stashapp/stash/internal/settings"
)

func (r *mutationResolver) validateVideoHardwareSettings(ctx context.Context, mode, device string) error {
	if mode == string(settings.VideoHardwareSoftware) {
		if device != "" {
			return errors.New("software video mode cannot select a hardware device")
		}
		return nil
	}
	if mode != string(settings.VideoHardwareAuto) && mode != string(settings.VideoHardwareNVENC) && mode != string(settings.VideoHardwareVAAPI) {
		return errors.New("invalid video hardware mode")
	}
	if r.Operations == nil {
		return errors.New("hardware capability status is unavailable")
	}
	status, err := r.Operations.VideoDependencyStatus(ctx)
	if err != nil {
		return err
	}
	for _, backend := range status.HardwareBackends {
		if backend.State == "AVAILABLE" && (mode == string(settings.VideoHardwareAuto) || backend.Backend == mode) && (device == "" || backend.Device == device) {
			return nil
		}
	}
	return errors.New("selected video hardware backend or device is not available")
}
