package api

import (
	"io/fs"
	"net/http"

	"github.com/tripplemay/proxywatch/internal/decision"
	"github.com/tripplemay/proxywatch/internal/store"
)

type Server struct {
	store    *store.Store
	authKey  string
	version  string
	staticFS fs.FS
	machine  *decision.Machine
}

func NewServer(s *store.Store, authKey, version string) *Server {
	return &Server{store: s, authKey: authKey, version: version}
}

// WithStatic configures the embedded frontend file system.
func (s *Server) WithStatic(fsys fs.FS) *Server {
	s.staticFS = fsys
	return s
}

func (s *Server) WithMachine(m *decision.Machine) *Server {
	s.machine = m
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)

	api := http.NewServeMux()
	api.HandleFunc("/api/status", s.handleStatus)
	api.HandleFunc("/api/test-notify", s.handleTestNotify)
	api.HandleFunc("/api/confirm-rotation", s.handleConfirmRotation)
	api.HandleFunc("/api/resume-automation", s.handleResumeAutomation)
	api.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleGetSettings(w, r)
		case http.MethodPut:
			s.handlePutSettings(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.Handle("/api/", BearerAuth(s.authKey, api))

	if s.staticFS != nil {
		mux.Handle("/", http.FileServer(http.FS(s.staticFS)))
	}
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("ok"))
}
