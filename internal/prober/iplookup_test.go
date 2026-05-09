package prober

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIPLookupTriesAllInOrder(t *testing.T) {
	calls := []string{}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "bad")
		w.WriteHeader(500)
	}))
	defer bad.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "good")
		w.Write([]byte("8.8.8.8"))
	}))
	defer good.Close()

	lookup := &IPLookup{
		Endpoints: []string{bad.URL, good.URL},
		Client:    bad.Client(),
		Timeout:   2 * time.Second,
	}
	ip, err := lookup.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if strings.TrimSpace(ip) != "8.8.8.8" {
		t.Errorf("ip=%q, want 8.8.8.8", ip)
	}
	if len(calls) != 2 || calls[0] != "bad" || calls[1] != "good" {
		t.Errorf("call order = %v, want [bad good]", calls)
	}
}

func TestIPLookupAllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	lookup := &IPLookup{
		Endpoints: []string{srv.URL, srv.URL},
		Client:    srv.Client(),
		Timeout:   2 * time.Second,
	}
	if _, err := lookup.Get(); err == nil {
		t.Error("expected error when all endpoints fail")
	}
}
