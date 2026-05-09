package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tripplemay/proxywatch/internal/decision"
	"github.com/tripplemay/proxywatch/internal/store"
)

func newStoreT(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStatusHandlerEmptyStore(t *testing.T) {
	s := newStoreT(t)
	srv := NewServer(s, "k", "0.1.0")
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/status", nil)
	r.Header.Set("Authorization", "Bearer k")
	srv.Handler().ServeHTTP(rec, r)

	if rec.Code != 200 {
		t.Fatalf("code=%d, want 200", rec.Code)
	}
	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["state"] != "HEALTHY" {
		t.Errorf("state=%v, want HEALTHY", got["state"])
	}
	if got["last_active_probe"] != nil {
		t.Errorf("expected nil last_active_probe, got %v", got["last_active_probe"])
	}
}

func TestStatusHandlerWithProbe(t *testing.T) {
	s := newStoreT(t)
	s.InsertProbe(store.Probe{
		TS: time.Now(), Kind: "active", HTTPCode: 200, OK: true, ExitIP: "1.1.1.1",
	})

	srv := NewServer(s, "k", "0.1.0")
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/status", nil)
	r.Header.Set("Authorization", "Bearer k")
	srv.Handler().ServeHTTP(rec, r)

	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var got map[string]any
	json.NewDecoder(rec.Body).Decode(&got)
	if got["exit_ip"] != "1.1.1.1" {
		t.Errorf("exit_ip=%v, want 1.1.1.1", got["exit_ip"])
	}
}

func TestTestNotifyEnqueuesMessage(t *testing.T) {
	s := newStoreT(t)
	srv := NewServer(s, "k", "0.1.0")
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/test-notify", nil)
	r.Header.Set("Authorization", "Bearer k")
	srv.Handler().ServeHTTP(rec, r)

	if rec.Code != 202 {
		t.Errorf("code=%d, want 202", rec.Code)
	}
	pending, _ := s.PendingNotifications(10)
	if len(pending) != 1 {
		t.Errorf("pending=%d, want 1", len(pending))
	}
}

func TestConfirmRotationAdvancesMachine(t *testing.T) {
	s := newStoreT(t)
	m := decision.NewMachine(decision.Defaults())
	srv := NewServer(s, "k", "0.1.0").WithMachine(m)

	// drive machine to SUSPECT
	for i := 0; i < 3; i++ {
		m.OnActive(time.Now(), false)
	}
	m.Tick(time.Now())

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/confirm-rotation", nil)
	r.Header.Set("Authorization", "Bearer k")
	srv.Handler().ServeHTTP(rec, r)

	if rec.Code != 200 {
		t.Errorf("code=%d", rec.Code)
	}
	if m.State() != decision.StateVerifying {
		t.Errorf("state=%s, want VERIFYING", m.State())
	}
}
