package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertProbeAndRecent(t *testing.T) {
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0)

	p := Probe{
		TS:        now,
		Kind:      "active",
		Target:    "https://api.openai.com/v1/models",
		HTTPCode:  200,
		LatencyMS: 312,
		ExitIP:    "172.58.213.36",
		OK:        true,
	}
	id, err := s.InsertProbe(p)
	if err != nil {
		t.Fatalf("InsertProbe: %v", err)
	}
	if id == 0 {
		t.Errorf("expected non-zero id, got %d", id)
	}

	got, err := s.RecentProbes(10, "")
	if err != nil {
		t.Fatalf("RecentProbes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	if got[0].HTTPCode != 200 || got[0].ExitIP != "172.58.213.36" {
		t.Errorf("round-trip mismatch: %+v", got[0])
	}
}

func TestRecentProbesFilteredByKind(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	for i, k := range []string{"active", "passive", "active"} {
		_, err := s.InsertProbe(Probe{
			TS:   now.Add(time.Duration(i) * time.Second),
			Kind: k,
			OK:   true,
		})
		if err != nil {
			t.Fatalf("InsertProbe[%d]: %v", i, err)
		}
	}
	got, err := s.RecentProbes(10, "active")
	if err != nil {
		t.Fatalf("RecentProbes: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("active filter: got %d, want 2", len(got))
	}
}
