package productdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/settings"
)

func TestRuntimeSettingsDefaultsAndOptimisticUpdate(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	defaults, err := db.Settings().Find(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Revision != 1 || defaults.HomeScope != settings.HomeList || !defaults.GalleryCardScrubberEnabled ||
		defaults.GalleryDetailMediaFilterEnabled || defaults.RelatedLimit != 6 || defaults.RandomLimit != 24 ||
		defaults.TagParentWeight != 0.25 || defaults.TagMinimumScore != 0.05 || defaults.RandomGalleryRepeatDecay != 0.25 ||
		defaults.GalleryAnimatedPlaybackLimit != 12 || defaults.GalleryAnimatedLockIntervalMS != 800 {
		t.Fatalf("runtime defaults = %#v", defaults)
	}
	defaults.HomeScope = settings.HomeAll
	defaults.GalleryCardScrubberEnabled = false
	updated, err := db.Settings().Update(ctx, defaults.Revision, defaults, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.HomeScope != settings.HomeAll || updated.GalleryCardScrubberEnabled {
		t.Fatalf("updated runtime settings = %#v", updated)
	}
	if _, err := db.Settings().Update(ctx, defaults.Revision, defaults, time.Now()); !errors.Is(err, ErrSettingsRevisionConflict) {
		t.Fatalf("stale settings revision error = %v", err)
	}
	invalid := updated
	invalid.RandomStaticQuota = 0.8
	if _, err := db.Settings().Update(ctx, updated.Revision, invalid, time.Now()); err == nil {
		t.Fatal("invalid random quotas were accepted")
	}
	invalid = updated
	invalid.GalleryAnimatedPlaybackLimit = 17
	if _, err := db.Settings().Update(ctx, updated.Revision, invalid, time.Now()); err == nil {
		t.Fatal("invalid animated playback limit was accepted")
	}
	invalid = updated
	invalid.GalleryAnimatedLockIntervalMS = 699
	if _, err := db.Settings().Update(ctx, updated.Revision, invalid, time.Now()); err == nil {
		t.Fatal("invalid animated lock interval was accepted")
	}
	if _, err := db.ExecContext(ctx, `UPDATE runtime_settings SET gallery_animated_playback_limit=17 WHERE id=1`); err == nil {
		t.Fatal("database accepted an animated playback limit outside its CHECK constraint")
	}
}
