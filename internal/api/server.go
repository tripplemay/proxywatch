package api

import (
	"net/http"

	"github.com/tripplemay/proxywatch/internal/store"
)

type Server struct {
	store   *store.Store
	authKey string
	version string
}

func NewServer(s *store.Store, authKey, version string) *Server {
	return &Server{store: s, authKey: authKey, version: version}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)

	api := http.NewServeMux()
	api.HandleFunc("/api/status", s.handleStatus)

	mux.Handle("/api/", BearerAuth(s.authKey, api))
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("ok"))
}
