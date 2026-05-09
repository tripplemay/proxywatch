package prober

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tripplemay/proxywatch/internal/decision"
	"github.com/tripplemay/proxywatch/internal/store"
)

func RunOnce(s *store.Store, p *ActiveProber, m *decision.Machine) error {
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
	if m != nil {
		// Distinguish proxy-gateway-down (transport failure, no HTTP response)
		// from upstream errors (got an HTTP response, even if 4xx/5xx).
		// HTTPCode == 0 + RawError != "" means we couldn't even reach upstream
		// — that's a proxy connectivity issue, not a rotation signal.
		if r.HTTPCode == 0 && r.RawError != "" {
			m.OnProxyDown(r.TS)
		} else {
			m.OnProxyUp(r.TS)
			m.OnActive(r.TS, r.OK)
		}
		m.Tick(r.TS)
	}
	return nil
}

func Loop(ctx context.Context, s *store.Store, p *ActiveProber, m *decision.Machine, getInterval func() time.Duration, log *slog.Logger) {
	for {
		if err := RunOnce(s, p, m); err != nil {
			log.Error("active probe", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(getInterval()):
		}
	}
}
