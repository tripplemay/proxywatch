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

func (s *Server) handleConfirmRotation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.machine == nil {
		http.Error(w, "machine not configured", 500)
		return
	}
	s.machine.Confirm()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleResumeAutomation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.machine == nil {
		http.Error(w, "machine not configured", 500)
		return
	}
	s.machine.ResumeAutomation()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{}
	keys := []string{
		"active_probe_interval_seconds",
		"passive_threshold",
		"active_failure_threshold",
		"suspect_observation_seconds",
		"cooldown_seconds",
		"telegram_bot_token",
		"telegram_chat_id",
	}
	for _, k := range keys {
		v, _, _ := s.store.GetKV(k)
		out[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "PUT only", http.StatusMethodNotAllowed)
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	for k, v := range body {
		if err := s.store.SetKV(k, v); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	w.WriteHeader(200)
}
