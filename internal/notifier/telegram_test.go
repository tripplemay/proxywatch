package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegramSendSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/bot") {
			t.Errorf("path=%s, want /bot...", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		if payload["chat_id"] != "12345" {
			t.Errorf("chat_id=%v, want 12345", payload["chat_id"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Telegram{
		Token:   "tok",
		ChatID:  "12345",
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
	}
	if err := c.Send("hello"); err != nil {
		t.Errorf("Send: %v", err)
	}
}

func TestTelegramSendApiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
	}))
	defer srv.Close()

	c := &Telegram{Token: "tok", ChatID: "x", BaseURL: srv.URL, HTTP: srv.Client()}
	if err := c.Send("x"); err == nil {
		t.Error("expected error from API ok=false response")
	}
}
