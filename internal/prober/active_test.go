package prober

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestActiveProbeSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	p := &ActiveProber{
		Target:   srv.URL,
		Timeout:  2 * time.Second,
		Client:   srv.Client(),
		IPLookup: func() (string, error) { return "1.2.3.4", nil },
	}
	r := p.Run()
	if !r.OK || r.HTTPCode != 200 {
		t.Errorf("expected ok+200, got %+v", r)
	}
	if r.ExitIP != "1.2.3.4" {
		t.Errorf("ExitIP=%q, want 1.2.3.4", r.ExitIP)
	}
	if r.LatencyMS < 0 {
		t.Error("LatencyMS should be >= 0")
	}
}

func TestActiveProbe403IsNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer srv.Close()

	p := &ActiveProber{
		Target:   srv.URL,
		Timeout:  2 * time.Second,
		Client:   srv.Client(),
		IPLookup: func() (string, error) { return "", nil },
	}
	r := p.Run()
	if r.OK {
		t.Error("403 should not be OK")
	}
	if r.HTTPCode != 403 {
		t.Errorf("HTTPCode=%d, want 403", r.HTTPCode)
	}
}

func TestActiveProbeNetworkError(t *testing.T) {
	p := &ActiveProber{
		Target:   "http://127.0.0.1:1", // refused
		Timeout:  500 * time.Millisecond,
		Client:   &http.Client{Timeout: 500 * time.Millisecond},
		IPLookup: func() (string, error) { return "", nil },
	}
	r := p.Run()
	if r.OK {
		t.Error("connection refused should not be OK")
	}
	if r.RawError == "" {
		t.Error("RawError should be populated on network failure")
	}
}
