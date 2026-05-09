package executor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tripplemay/proxywatch/internal/decision"
	"github.com/tripplemay/proxywatch/internal/store"
)

// Executor watches the decision machine and enacts side effects on state transitions:
//   - Entering ROTATING: send alert, open incident
//   - IP change observed during ROTATING: notify machine
//   - Entering COOLDOWN: write rotation record + send "recovered" alert
//   - Entering ALERT_ONLY: send "automation paused" alert
type Executor struct {
	Store   *store.Store
	Machine *decision.Machine
	Alert   func(text string, level string)
	Log     *slog.Logger

	prevState     decision.State
	openIncident  int64
	rotationStart time.Time
	rotationOldIP string
}

func (e *Executor) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	e.prevState = decision.StateHealthy
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.tick(time.Now())
		}
	}
}

func (e *Executor) tick(now time.Time) {
	cur := e.Machine.State()
	if cur == e.prevState {
		// observe IP changes if in ROTATING
		if cur == decision.StateRotating {
			if newIP, ok := e.detectIPChange(); ok {
				e.Machine.OnIPChange()
				if e.Log != nil {
					e.Log.Info("auto-detected IP change", "old", e.rotationOldIP, "new", newIP)
				}
			}
		}
		return
	}
	// transition
	switch cur {
	case decision.StateRotating:
		// open incident, snapshot old IP, send alert
		e.openIncident, _ = e.Store.OpenIncident(store.Incident{
			StartedAt:     now,
			TriggerReason: "auto",
			InitialState:  string(cur),
		})
		e.rotationStart = now
		e.rotationOldIP = e.lastExitIP()
		if e.Alert != nil {
			e.Alert(fmt.Sprintf("⚠️ proxy unhealthy\ncurrent IP: %s\nplease rotate at miyaIP backend; proxywatch will auto-detect", e.rotationOldIP), "warning")
		}
	case decision.StateCooldown:
		newIP := e.lastExitIP()
		_, _ = e.Store.InsertRotation(store.Rotation{
			IncidentID:      e.openIncident,
			StartedAt:       e.rotationStart,
			EndedAt:         now,
			OldIP:           e.rotationOldIP,
			NewIP:           newIP,
			DetectionMethod: "auto",
			OK:              true,
		})
		_ = e.Store.CloseIncident(e.openIncident, now, "recovered")
		e.openIncident = 0
		if e.Alert != nil {
			e.Alert(fmt.Sprintf("✅ recovered\nold: %s\nnew: %s", e.rotationOldIP, newIP), "info")
		}
	case decision.StateAlertOnly:
		if e.Alert != nil {
			e.Alert("❌ automation paused — please investigate", "error")
		}
		if e.openIncident != 0 {
			_ = e.Store.CloseIncident(e.openIncident, now, "alert_only")
			e.openIncident = 0
		}
	}
	e.prevState = cur
}

func (e *Executor) lastExitIP() string {
	rows, err := e.Store.RecentProbes(1, "active")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return rows[0].ExitIP
}

func (e *Executor) detectIPChange() (string, bool) {
	cur := e.lastExitIP()
	return cur, cur != "" && cur != e.rotationOldIP
}
