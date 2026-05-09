package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/tripplemay/proxywatch/internal/store"
)

type statusResponse struct {
	Version         string     `json:"version"`
	State           string     `json:"state"`
	ExitIP          string     `json:"exit_ip,omitempty"`
	LastActiveProbe *probeJSON `json:"last_active_probe,omitempty"`
}

type probeJSON struct {
	TSMS      int64  `json:"ts_ms"`
	HTTPCode  int    `json:"http_code"`
	LatencyMS int    `json:"latency_ms"`
	ExitIP    string `json:"exit_ip,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	state := "HEALTHY"
	if s.machine != nil {
		state = string(s.machine.State())
	}
	resp := statusResponse{Version: s.version, State: state}

	rows, err := s.store.RecentProbes(1, "active")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if len(rows) > 0 {
		p := rows[0]
		resp.LastActiveProbe = &probeJSON{
			TSMS:      p.TS.UnixMilli(),
			HTTPCode:  p.HTTPCode,
			LatencyMS: p.LatencyMS,
			ExitIP:    p.ExitIP,
			OK:        p.OK,
			Error:     p.RawError,
		}
		resp.ExitIP = p.ExitIP
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleTestNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	_, err := s.store.EnqueueNotification(store.Notification{
		TS:    time.Now(),
		Level: "info",
		Text:  "proxywatch test notification — if you see this, telegram works",
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
