package notifier

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tripplemay/proxywatch/internal/store"
)

func newStoreT(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestQueueDrainSendsAndMarksSent(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s := newStoreT(t)
	tg := &Telegram{Token: "t", ChatID: "c", BaseURL: srv.URL, HTTP: srv.Client()}
	q := &Queue{Store: s, Telegram: tg}

	id, _ := s.EnqueueNotification(store.Notification{TS: time.Now(), Level: "info", Text: "hi"})
	if err := q.DrainOnce(context.Background()); err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("hits=%d, want 1", hits)
	}
	pending, _ := s.PendingNotifications(10)
	if len(pending) != 0 {
		t.Errorf("pending len=%d, want 0; id=%d still listed", len(pending), id)
	}
}

func TestQueueDrainRecordsFailureAndContinues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	s := newStoreT(t)
	tg := &Telegram{Token: "t", ChatID: "c", BaseURL: srv.URL, HTTP: srv.Client()}
	q := &Queue{Store: s, Telegram: tg, MaxRetries: 5}

	s.EnqueueNotification(store.Notification{TS: time.Now(), Level: "info", Text: "1"})
	if err := q.DrainOnce(context.Background()); err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
	pending, _ := s.PendingNotifications(10)
	if len(pending) != 1 || pending[0].RetryCount != 1 {
		t.Errorf("after failure, retry_count=%d, len=%d", pending[0].RetryCount, len(pending))
	}
}

func TestQueueDrainSendsWithButtonsWhenPresent(t *testing.T) {
	var hits, gotReplyMarkup int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "reply_markup") {
			atomic.AddInt32(&gotReplyMarkup, 1)
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s := newStoreT(t)
	tg := &Telegram{Token: "t", ChatID: "c", BaseURL: srv.URL, HTTP: srv.Client()}
	q := &Queue{Store: s, Telegram: tg}

	s.EnqueueNotification(store.Notification{
		TS:      time.Now(),
		Level:   "warning",
		Text:    "boom",
		Buttons: `[{"text":"OK","callback_data":"ok"}]`,
	})
	if err := q.DrainOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("hits=%d, want 1", hits)
	}
	if atomic.LoadInt32(&gotReplyMarkup) != 1 {
		t.Errorf("expected reply_markup in body, got %d", gotReplyMarkup)
	}
}
