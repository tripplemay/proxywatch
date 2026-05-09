package prober

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tripplemay/proxywatch/internal/store"
)

// RunOnce executes a single active probe and persists the result.
func RunOnce(s *store.Store, p *ActiveProber) error {
	r := p.Run()
	_, err := s.InsertProbe(store.Probe{
		TS:        r.TS,
		Kind:      "active",
		Target:    r.Target,
		HTTPCode:  r.HTTPCode,
		LatencyMS: r.LatencyMS,
		ExitIP:    r.ExitIP,
		OK:        r.OK,
		RawError:  r.RawError,
	})
	if err != nil {
		return fmt.Errorf("persist probe: %w", err)
	}
	return nil
}

// Loop runs RunOnce on a ticker until ctx is cancelled.
// Interval is read fresh from getInterval each tick to allow live config changes.
func Loop(ctx context.Context, s *store.Store, p *ActiveProber, getInterval func() time.Duration, log *slog.Logger) {
	for {
		if err := RunOnce(s, p); err != nil {
			log.Error("active probe failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(getInterval()):
		}
	}
}
