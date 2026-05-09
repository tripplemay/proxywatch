package decision

import (
	"sync"
	"time"
)

type Params struct {
	PassiveWindow             time.Duration
	PassiveThreshold          int
	ActiveFailureThreshold    int
	SuspectObservation        time.Duration
	RotatingTimeout           time.Duration
	VerifyingMaxAttempts      int
	Cooldown                  time.Duration
	RotationFailuresAlertOnly int
}

func Defaults() Params {
	return Params{
		PassiveWindow:             5 * time.Minute,
		PassiveThreshold:          3,
		ActiveFailureThreshold:    3,
		SuspectObservation:        60 * time.Second,
		RotatingTimeout:           10 * time.Minute,
		VerifyingMaxAttempts:      5,
		Cooldown:                  120 * time.Second,
		RotationFailuresAlertOnly: 2,
	}
}

type Machine struct {
	mu sync.Mutex

	params                 Params
	state                  State
	since                  time.Time
	window                 *Window
	consecutiveActiveFails int

	// rotation state
	rotationCount     int
	verifyingAttempts int
}

func NewMachine(p Params) *Machine {
	return &Machine{
		params: p,
		state:  StateHealthy,
		since:  time.Now(),
		window: NewWindow(p.PassiveWindow),
	}
}

// State returns current state (snapshot).
func (m *Machine) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// OnPassive records a passive log observation.
func (m *Machine) OnPassive(at time.Time, code int) {
	m.window.Add(at, code)
}

// OnActive records an active probe outcome.
func (m *Machine) OnActive(at time.Time, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ok {
		m.consecutiveActiveFails = 0
	} else {
		m.consecutiveActiveFails++
	}
}

// OnIPChange tells the machine the exit IP just changed.
func (m *Machine) OnIPChange() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateRotating {
		m.transition(StateVerifying, time.Now())
	}
}

// Confirm forces VERIFYING (used when user clicks "I rotated" button).
func (m *Machine) Confirm() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateRotating || m.state == StateSuspect {
		m.transition(StateVerifying, time.Now())
	}
}

// Tick advances the state machine based on current observations and time.
// Should be called periodically (e.g. every active probe + every passive batch).
func (m *Machine) Tick(now time.Time) State {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.state {
	case StateHealthy:
		// trigger conditions
		if m.window.Count(now) >= m.params.PassiveThreshold {
			m.transition(StateSuspect, now)
		} else if m.consecutiveActiveFails >= m.params.ActiveFailureThreshold {
			m.transition(StateSuspect, now)
		}
	case StateSuspect:
		// recovery: active OK and below passive threshold
		if m.consecutiveActiveFails == 0 && m.window.Count(now) < m.params.PassiveThreshold {
			// require some sustained healthy observation
			if now.Sub(m.since) >= 10*time.Second {
				m.transition(StateHealthy, now)
			}
		} else if now.Sub(m.since) >= m.params.SuspectObservation {
			m.transition(StateRotating, now)
		}
	case StateRotating:
		if now.Sub(m.since) >= m.params.RotatingTimeout {
			m.transition(StateAlertOnly, now)
		}
	case StateVerifying:
		// stay; transitions driven by OnActive results checked here
		if m.consecutiveActiveFails == 0 {
			m.transition(StateCooldown, now)
		} else if m.verifyingAttempts >= m.params.VerifyingMaxAttempts {
			m.rotationCount++
			m.verifyingAttempts = 0
			if m.rotationCount >= m.params.RotationFailuresAlertOnly {
				m.transition(StateAlertOnly, now)
			} else {
				m.transition(StateRotating, now)
			}
		}
	case StateCooldown:
		if now.Sub(m.since) >= m.params.Cooldown {
			m.rotationCount = 0
			m.transition(StateHealthy, now)
		}
	case StateAlertOnly:
		// only manual recovery
	}
	return m.state
}

// ResumeAutomation flips ALERT_ONLY back to HEALTHY (manual op).
func (m *Machine) ResumeAutomation() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateAlertOnly {
		m.rotationCount = 0
		m.transition(StateHealthy, time.Now())
	}
}

func (m *Machine) transition(to State, at time.Time) {
	m.state = to
	m.since = at
	if to == StateVerifying {
		m.verifyingAttempts = 0
	}
}
