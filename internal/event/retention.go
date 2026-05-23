package event

import (
	"context"
	"log/slog"
	"time"
)

// RunRetention loops every `interval`, deleting events older than `window`.
// Window is the privacy budget — the maximum age of any raw event in the log.
// CORE_PLATFORM.md Phase 6 says ~2 weeks; configurable via main.go flag.
//
// Run respects ctx cancellation and runs an initial pass immediately so a
// short-lived process doesn't have to wait a full interval.
func RunRetention(ctx context.Context, store *Store, window, interval time.Duration, logger *slog.Logger) {
	if window <= 0 || interval <= 0 {
		return
	}
	tick := func() {
		cutoff := time.Now().UTC().Add(-window)
		n, err := store.DeleteOlderThan(ctx, cutoff)
		if err != nil {
			if logger != nil {
				logger.Warn("retention sweep failed", "err", err)
			}
			return
		}
		if n > 0 && logger != nil {
			logger.Info("retention sweep", "deleted", n, "older_than", cutoff)
		}
	}
	tick()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}
