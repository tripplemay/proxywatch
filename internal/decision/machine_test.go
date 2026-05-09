package decision

import (
	"testing"
	"time"
)

func TestHealthyToSuspectOn4xxThreshold(t *testing.T) {
	m := NewMachine(Defaults())
	now := time.Now()
	for i := 0; i < 3; i++ {
		m.OnPassive(now.Add(time.Duration(i)*time.Second), 403)
	}
	if got := m.Tick(now.Add(3 * time.Second)); got != StateSuspect {
		t.Errorf("after 3 403s, state=%s, want SUSPECT", got)
	}
}

func TestHealthyToSuspectOnConsecutiveActiveFailures(t *testing.T) {
	m := NewMachine(Defaults())
	now := time.Now()
	m.OnActive(now, false)
	m.OnActive(now, false)
	m.OnActive(now, false)
	if got := m.Tick(now); got != StateSuspect {
		t.Errorf("state=%s, want SUSPECT", got)
	}
}

func TestSuspectBackToHealthyAfterRecovery(t *testing.T) {
	m := NewMachine(Defaults())
	now := time.Now()
	for i := 0; i < 3; i++ {
		m.OnActive(now, false)
	}
	m.Tick(now) // → SUSPECT
	// recovery: pass within suspect_observation
	m.OnActive(now.Add(10*time.Second), true)
	m.OnActive(now.Add(20*time.Second), true)
	m.OnActive(now.Add(30*time.Second), true)
	if got := m.Tick(now.Add(30 * time.Second)); got != StateHealthy {
		t.Errorf("state=%s, want HEALTHY", got)
	}
}

func TestSuspectToRotatingAfterObservationTimeout(t *testing.T) {
	d := Defaults()
	d.SuspectObservation = 100 * time.Millisecond
	m := NewMachine(d)
	now := time.Now()
	for i := 0; i < 3; i++ {
		m.OnActive(now, false)
	}
	m.Tick(now) // SUSPECT
	if got := m.Tick(now.Add(200 * time.Millisecond)); got != StateRotating {
		t.Errorf("state=%s, want ROTATING", got)
	}
}

func TestProxyDownDoesNotTriggerSuspect(t *testing.T) {
	m := NewMachine(Defaults())
	now := time.Now()
	for i := 0; i < 5; i++ {
		m.OnProxyDown(now)
	}
	if m.Tick(now) != StateHealthy {
		t.Errorf("state=%s, want HEALTHY (proxy down should not be a rotation trigger)", m.State())
	}
	if !m.IsProxyDown() {
		t.Error("IsProxyDown should be true")
	}
}
