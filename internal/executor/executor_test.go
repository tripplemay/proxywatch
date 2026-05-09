package executor

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tripplemay/proxywatch/internal/decision"
	"github.com/tripplemay/proxywatch/internal/store"
)

func newStoreT(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestExecutorAlertsOnEnterRotating(t *testing.T) {
	s := newStoreT(t)
	d := decision.Defaults()
	d.SuspectObservation = 50 * time.Millisecond
	m := decision.NewMachine(d)

	var alerts int32
	e := &Executor{
		Store:   s,
		Machine: m,
		Alert: func(text string, level string) {
			atomic.AddInt32(&alerts, 1)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go e.Run(ctx, 20*time.Millisecond)

	now := time.Now()
	for i := 0; i < 3; i++ {
		m.OnActive(now, false)
	}
	m.Tick(now) // → SUSPECT
	time.Sleep(100 * time.Millisecond)
	m.Tick(time.Now()) // → ROTATING (after observation)

	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&alerts) < 1 {
		t.Errorf("expected at least 1 alert, got %d", alerts)
	}
}
