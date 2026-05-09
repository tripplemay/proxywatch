package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuthRejectsMissingHeader(t *testing.T) {
	h := BearerAuth("k", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 401 {
		t.Errorf("code=%d, want 401", rec.Code)
	}
}

func TestBearerAuthRejectsWrongKey(t *testing.T) {
	h := BearerAuth("right", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	h.ServeHTTP(rec, r)
	if rec.Code != 401 {
		t.Errorf("code=%d, want 401", rec.Code)
	}
}

func TestBearerAuthAcceptsCorrectKey(t *testing.T) {
	h := BearerAuth("k", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer k")
	h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Errorf("code=%d, want 200", rec.Code)
	}
}
