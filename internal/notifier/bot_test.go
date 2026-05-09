package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// makeTestTelegram creates a Telegram pointed at the given httptest server.
func makeTestTelegram(srv *httptest.Server) *Telegram {
	return &Telegram{
		Token:   "testtoken",
		ChatID:  "42",
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
	}
}

// stubTGServer returns a server that accepts any POST and records hits.
func stubTGServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestBotIgnoresWrongChatID(t *testing.T) {
	srv, hits := stubTGServer(t)
	tg := makeTestTelegram(srv)

	handlerCalled := false
	b := &Bot{
		Telegram:   tg,
		AuthChatID: "42",
		Commands: map[string]CommandHandler{
			"/status": func(ctx context.Context, args string) string {
				handlerCalled = true
				return "ok"
			},
		},
		Callbacks: map[string]CallbackHandler{},
	}

	// message from wrong chat (ID=99, not 42)
	b.handleMessage(context.Background(), &tgMessage{
		MessageID: 1,
		Chat:      tgChat{ID: 99},
		From:      tgUser{ID: 99},
		Text:      "/status",
	})

	if handlerCalled {
		t.Error("handler should not be called for wrong chat ID")
	}
	if atomic.LoadInt32(hits) > 0 {
		t.Errorf("expected no Send calls, got %d", atomic.LoadInt32(hits))
	}
}

func TestBotDispatchesAuthorizedCommand(t *testing.T) {
	srv, hits := stubTGServer(t)
	tg := makeTestTelegram(srv)

	handlerCalled := false
	b := &Bot{
		Telegram:   tg,
		AuthChatID: "42",
		Commands: map[string]CommandHandler{
			"/status": func(ctx context.Context, args string) string {
				handlerCalled = true
				return "all good"
			},
		},
		Callbacks: map[string]CallbackHandler{},
	}

	b.handleMessage(context.Background(), &tgMessage{
		MessageID: 2,
		Chat:      tgChat{ID: 42},
		From:      tgUser{ID: 42},
		Text:      "/status",
	})

	if !handlerCalled {
		t.Error("expected handler to be called for authorized command")
	}
	if atomic.LoadInt32(hits) < 1 {
		t.Errorf("expected at least 1 sendMessage hit, got %d", atomic.LoadInt32(hits))
	}
}

func TestBotDispatchesCallback(t *testing.T) {
	srv, _ := stubTGServer(t)
	tg := makeTestTelegram(srv)

	handlerCalled := false
	b := &Bot{
		Telegram:   tg,
		AuthChatID: "42",
		Commands:   map[string]CommandHandler{},
		Callbacks: map[string]CallbackHandler{
			"confirm": func(ctx context.Context, data string) (string, bool) {
				handlerCalled = true
				return "confirmed", false
			},
		},
	}

	b.handleCallback(context.Background(), &tgCallbackQuery{
		ID:   "cq1",
		From: tgUser{ID: 42},
		Data: "confirm",
	})

	if !handlerCalled {
		t.Error("expected callback handler to be called for authorized callback")
	}
}

func TestBotIgnoresUnauthorizedCallback(t *testing.T) {
	srv, _ := stubTGServer(t)
	tg := makeTestTelegram(srv)

	handlerCalled := false
	b := &Bot{
		Telegram:   tg,
		AuthChatID: "42",
		Commands:   map[string]CommandHandler{},
		Callbacks: map[string]CallbackHandler{
			"confirm": func(ctx context.Context, data string) (string, bool) {
				handlerCalled = true
				return "confirmed", false
			},
		},
	}

	// from.ID = 99, not 42
	b.handleCallback(context.Background(), &tgCallbackQuery{
		ID:   "cq2",
		From: tgUser{ID: 99},
		Data: "confirm",
	})

	if handlerCalled {
		t.Error("handler should not be called for unauthorized callback")
	}
}

// TestBotPollOnce verifies that pollOnce parses updates and advances the offset.
func TestBotPollOnce(t *testing.T) {
	var requestCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			// Return one update with a message
			resp := getUpdatesResp{
				OK: true,
				Result: []tgUpdate{
					{
						UpdateID: 101,
						Message: &tgMessage{
							MessageID: 5,
							Chat:      tgChat{ID: 42},
							From:      tgUser{ID: 42},
							Text:      "/status",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			// answerCallbackQuery or subsequent poll — just ok
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
		}
	}))
	t.Cleanup(srv.Close)

	tg := &Telegram{
		Token:   "tok",
		ChatID:  "42",
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
	}

	dispatched := false
	b := &Bot{
		Telegram:   tg,
		AuthChatID: "42",
		Commands: map[string]CommandHandler{
			"/status": func(ctx context.Context, args string) string {
				dispatched = true
				return ""
			},
		},
		Callbacks: map[string]CallbackHandler{},
	}

	if err := b.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if !dispatched {
		t.Error("expected /status handler to be dispatched from pollOnce")
	}
	if b.offset != 101 {
		t.Errorf("expected offset 101, got %d", b.offset)
	}
}
