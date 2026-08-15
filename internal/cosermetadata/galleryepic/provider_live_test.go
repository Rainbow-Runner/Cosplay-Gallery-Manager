package galleryepic

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/cosermetadata"
)

// This test is intentionally opt-in. CI and the offline release gates must
// never depend on a third-party service, while maintainers can explicitly run
// it when GalleryEpic changes its public HTML or asset delivery.
func TestLiveGalleryEpicProfileOptIn(t *testing.T) {
	if os.Getenv("CGM_LIVE_GALLERYEPIC_TEST") != "1" {
		t.Skip("set CGM_LIVE_GALLERYEPIC_TEST=1 for an explicit live compatibility check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	service := &cosermetadata.Service{Registry: mustRegistry(t, New())}
	candidates, err := service.Search(ctx, providerKey, "Yaokoututu")
	if err != nil || len(candidates) == 0 {
		t.Fatalf("live search = %#v, %v", candidates, err)
	}
	var selected string
	for _, candidate := range candidates {
		if candidate.Ref == "298" {
			selected = candidate.Ref
			break
		}
	}
	if selected == "" {
		t.Fatalf("known public fixture Coser was not returned: %#v", candidates)
	}
	preview, err := service.Prepare(ctx, providerKey, "live-fixture-coser", selected)
	if err != nil {
		t.Fatal(err)
	}
	if preview.DisplayName == "" || !preview.HasAvatar || !preview.HasBanner || len(preview.Accounts) < 2 {
		t.Fatalf("live profile is incomplete: %#v", preview)
	}
}

func mustRegistry(t *testing.T, providers ...cosermetadata.Provider) *cosermetadata.Registry {
	t.Helper()
	registry, err := cosermetadata.NewRegistry(providers...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
