package productserver

import (
	"context"
	"time"
)

const backgroundRecoveryInterval = time.Hour

func backgroundWaitDuration(now, deadline time.Time) time.Duration {
	d := backgroundRecoveryInterval
	if !deadline.IsZero() && deadline.Sub(now) < d {
		d = deadline.Sub(now)
	}
	// Durable times have millisecond precision and lease claims may use a
	// strict comparison. Also prevent hot loops on blocked/failed work.
	if d < 500*time.Millisecond {
		d = 500 * time.Millisecond
	}
	return d
}

func waitBackground(ctx context.Context, events <-chan struct{}, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-events:
		return true
	case <-timer.C:
		return true
	}
}
