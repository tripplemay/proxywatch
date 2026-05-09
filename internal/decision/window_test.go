package decision

import (
	"testing"
	"time"
)

func TestWindowCountsWithinDuration(t *testing.T) {
	w := NewWindow(5 * time.Second)
	now := time.Now()
	w.Add(now.Add(-10*time.Second), 403)
	w.Add(now.Add(-3*time.Second), 429)
	w.Add(now.Add(-1*time.Second), 403)
	if got := w.Count(now); got != 2 {
		t.Errorf("Count = %d, want 2", got)
	}
}

func TestWindowOnlyCountsTriggerCodes(t *testing.T) {
	w := NewWindow(time.Minute)
	now := time.Now()
	for _, c := range []int{200, 401, 403, 429, 500} {
		w.Add(now, c)
	}
	if got := w.Count(now); got != 2 {
		t.Errorf("Count = %d, want 2 (only 403 and 429)", got)
	}
}
