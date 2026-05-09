package store

import (
	"testing"
	"time"
)

func TestIncidentLifecycle(t *testing.T) {
	s := newStore(t)
	now := time.Now()

	id, err := s.OpenIncident(Incident{
		StartedAt:     now,
		TriggerReason: "passive_4xx",
		InitialState:  "SUSPECT",
	})
	if err != nil {
		t.Fatalf("OpenIncident: %v", err)
	}

	if err := s.IncrementRotationCount(id); err != nil {
		t.Fatalf("Increment: %v", err)
	}
	if err := s.IncrementRotationCount(id); err != nil {
		t.Fatalf("Increment 2: %v", err)
	}

	if err := s.CloseIncident(id, now.Add(2*time.Minute), "recovered"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	open, err := s.OpenIncidents()
	if err != nil {
		t.Fatalf("OpenIncidents: %v", err)
	}
	if len(open) != 0 {
		t.Errorf("expected no open incidents, got %d", len(open))
	}

	recent, err := s.RecentIncidents(10)
	if err != nil {
		t.Fatalf("RecentIncidents: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("len=%d, want 1", len(recent))
	}
	if recent[0].RotationCount != 2 {
		t.Errorf("rotation_count=%d, want 2", recent[0].RotationCount)
	}
	if recent[0].TerminalState != "recovered" {
		t.Errorf("terminal_state=%q, want recovered", recent[0].TerminalState)
	}
}
