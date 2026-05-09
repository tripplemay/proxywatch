package decision

import (
	"sync"
	"time"
)

// Window tracks recent HTTP status codes that count as "trigger" events
// (currently 403 and 429). It evicts entries older than `duration`.
type Window struct {
	mu       sync.Mutex
	duration time.Duration
	events   []event
}

type event struct {
	at   time.Time
	code int
}

func NewWindow(d time.Duration) *Window {
	return &Window{duration: d}
}

func IsTriggerCode(code int) bool {
	return code == 403 || code == 429
}

func (w *Window) Add(at time.Time, code int) {
	if !IsTriggerCode(code) {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, event{at: at, code: code})
}

func (w *Window) Count(now time.Time) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	cutoff := now.Add(-w.duration)
	keep := w.events[:0]
	for _, e := range w.events {
		if e.at.After(cutoff) {
			keep = append(keep, e)
		}
	}
	w.events = keep
	return len(w.events)
}
