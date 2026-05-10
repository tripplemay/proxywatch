package store

import (
	"testing"
	"time"
)

func TestEnqueueAndPending(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	id, err := s.EnqueueNotification(Notification{
		TS:    now,
		Level: "warning",
		Text:  "test alert",
	})
	if err != nil {
		t.Fatalf("EnqueueNotification: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	pending, err := s.PendingNotifications(100)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("len=%d, want 1", len(pending))
	}

	if err := s.MarkNotificationSent(id, now); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	pending, err = s.PendingNotifications(100)
	if err != nil {
		t.Fatalf("PendingNotifications 2: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("after MarkSent, pending=%d, want 0", len(pending))
	}
}

func TestRecordNotificationFailureIncrements(t *testing.T) {
	s := newStore(t)
	id, _ := s.EnqueueNotification(Notification{TS: time.Now(), Level: "info", Text: "x"})
	if err := s.RecordNotificationFailure(id, "503"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if err := s.RecordNotificationFailure(id, "503"); err != nil {
		t.Fatalf("RecordFailure 2: %v", err)
	}
	pending, _ := s.PendingNotifications(100)
	if pending[0].RetryCount != 2 {
		t.Errorf("retry_count=%d, want 2", pending[0].RetryCount)
	}
}

func TestEnqueueWithButtonsRoundTrips(t *testing.T) {
	s := newStore(t)
	id, err := s.EnqueueNotification(Notification{
		TS:      time.Now(),
		Level:   "warning",
		Text:    "with buttons",
		Buttons: `[{"text":"OK","callback_data":"ok"}]`,
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if id == 0 {
		t.Fatal("zero id")
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("len=%d", len(pending))
	}
	if pending[0].Buttons != `[{"text":"OK","callback_data":"ok"}]` {
		t.Errorf("buttons roundtrip: got %q", pending[0].Buttons)
	}
}
