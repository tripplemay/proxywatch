package main

import (
	"testing"
	"time"
)

func TestEvalStepErrorRateExceeded(t *testing.T) {
	e := &Eval{ErrorRateThreshold: 0.5}
	rows := []Row{
		{HTTPCode: 200},
		{HTTPCode: 200},
		{HTTPCode: 429},
		{HTTPCode: 429},
		{HTTPCode: 503},
	}
	if got := e.EvalStep(rows); got != StopErrorRate {
		t.Errorf("got %q, want %q (3/5 = 60%% > 50%%)", got, StopErrorRate)
	}
}

func TestEvalStepBelowThreshold(t *testing.T) {
	e := &Eval{ErrorRateThreshold: 0.5}
	rows := []Row{
		{HTTPCode: 200},
		{HTTPCode: 200},
		{HTTPCode: 200},
		{HTTPCode: 429},
	}
	if got := e.EvalStep(rows); got != "" {
		t.Errorf("got %q, want '' (1/4 = 25%%)", got)
	}
}

func TestEvalStepEmpty(t *testing.T) {
	e := &Eval{ErrorRateThreshold: 0.5}
	if got := e.EvalStep(nil); got != "" {
		t.Errorf("empty step should not stop, got %q", got)
	}
}

func TestEvalGlobalTimeLimit(t *testing.T) {
	start := time.Unix(0, 0)
	e := &Eval{StartTime: start, HardLimit: 25 * time.Minute, NoSuccessWindow: 30 * time.Second}
	if got := e.EvalGlobal(start.Add(20*time.Minute), nil); got != "" {
		t.Errorf("under limit got %q", got)
	}
	if got := e.EvalGlobal(start.Add(26*time.Minute), nil); got != StopTimeLimit {
		t.Errorf("over limit got %q, want %q", got, StopTimeLimit)
	}
}

func TestEvalGlobalNoSuccessWindow(t *testing.T) {
	now := time.Now()
	e := &Eval{
		StartTime:       now.Add(-2 * time.Minute),
		HardLimit:       25 * time.Minute,
		NoSuccessWindow: 30 * time.Second,
	}
	rows := []Row{
		{TSMS: now.Add(-40 * time.Second).UnixMilli(), HTTPCode: 429},
		{TSMS: now.Add(-30 * time.Second).UnixMilli(), HTTPCode: 429},
		{TSMS: now.Add(-20 * time.Second).UnixMilli(), HTTPCode: 429},
		{TSMS: now.Add(-10 * time.Second).UnixMilli(), HTTPCode: 429},
		{TSMS: now.Add(-1 * time.Second).UnixMilli(), HTTPCode: 429},
	}
	if got := e.EvalGlobal(now, rows); got != StopNoSuccess {
		t.Errorf("got %q, want %q", got, StopNoSuccess)
	}

	rows[len(rows)-1].HTTPCode = 200
	if got := e.EvalGlobal(now, rows); got != "" {
		t.Errorf("with one success got %q, want ''", got)
	}
}
