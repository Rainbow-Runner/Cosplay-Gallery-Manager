package productapi

import (
	"context"
	"testing"
)

func TestVideoHardwareSettingsRequireAProbedBackendAndSafeDevice(t *testing.T) {
	resolver := &mutationResolver{Resolver: &Resolver{Operations: fakeOperationsService{}}}
	for _, valid := range []struct{ mode, device string }{{"SOFTWARE", ""}, {"AUTO", ""}, {"NVENC", ""}, {"NVENC", "nvidia0"}} {
		if err := resolver.validateVideoHardwareSettings(context.Background(), valid.mode, valid.device); err != nil {
			t.Fatalf("valid setting %q/%q: %v", valid.mode, valid.device, err)
		}
	}
	for _, invalid := range []struct{ mode, device string }{{"SOFTWARE", "nvidia0"}, {"QSV", ""}, {"VAAPI", ""}, {"NVENC", "nvidia9"}} {
		if err := resolver.validateVideoHardwareSettings(context.Background(), invalid.mode, invalid.device); err == nil {
			t.Fatalf("invalid setting %q/%q was accepted", invalid.mode, invalid.device)
		}
	}
}
