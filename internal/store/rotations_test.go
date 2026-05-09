package store

import (
	"testing"
	"time"
)

func TestRotationInsertAndRecent(t *testing.T) {
	s := newStore(t)
	now := time.Now()

	incidentID, err := s.OpenIncident(Incident{StartedAt: now, TriggerReason: "manual"})
	if err != nil {
		t.Fatalf("OpenIncident: %v", err)
	}

	rotID, err := s.InsertRotation(Rotation{
		IncidentID:      incidentID,
		StartedAt:       now,
		EndedAt:         now.Add(45 * time.Second),
		OldIP:           "172.58.213.36",
		NewIP:           "172.58.213.99",
		DetectionMethod: "auto",
		OK:              true,
	})
	if err != nil {
		t.Fatalf("InsertRotation: %v", err)
	}
	if rotID == 0 {
		t.Error("expected non-zero rotation id")
	}

	got, err := s.RecentRotations(10)
	if err != nil {
		t.Fatalf("RecentRotations: %v", err)
	}
	if len(got) != 1 || got[0].NewIP != "172.58.213.99" {
		t.Errorf("got %+v", got)
	}
}
