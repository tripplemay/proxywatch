package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSamplerLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("9.9.9.9"))
	}))
	defer srv.Close()

	s := &Sampler{
		Endpoints: []string{srv.URL},
		HTTP:      srv.Client(),
		Timeout:   2 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go s.Run(ctx, 50*time.Millisecond)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if s.Latest().IP != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := s.Latest()
	if got.IP != "9.9.9.9" {
		t.Errorf("Latest().IP=%q, want 9.9.9.9", got.IP)
	}
	if got.TSMS == 0 {
		t.Error("Latest().TSMS should be set")
	}
}

func TestSamplerFallbackOnError(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("4.4.4.4"))
	}))
	defer good.Close()

	s := &Sampler{
		Endpoints: []string{bad.URL, good.URL},
		HTTP:      bad.Client(),
		Timeout:   2 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go s.Run(ctx, 50*time.Millisecond)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if s.Latest().IP != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s.Latest().IP != "4.4.4.4" {
		t.Errorf("expected fallback to second endpoint, got %q", s.Latest().IP)
	}
}
