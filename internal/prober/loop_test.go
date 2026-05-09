package prober

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tripplemay/proxywatch/internal/store"
)

func newStoreT(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRunOnceWritesProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := newStoreT(t)
	p := &ActiveProber{
		Target:   srv.URL,
		Timeout:  2 * time.Second,
		Client:   srv.Client(),
		IPLookup: func() (string, error) { return "5.6.7.8", nil },
	}

	if err := RunOnce(s, p, nil); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	rows, err := s.RecentProbes(10, "active")
	if err != nil {
		t.Fatalf("RecentProbes: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len=%d, want 1", len(rows))
	}
	if rows[0].HTTPCode != 200 || rows[0].ExitIP != "5.6.7.8" {
		t.Errorf("got %+v", rows[0])
	}
}
