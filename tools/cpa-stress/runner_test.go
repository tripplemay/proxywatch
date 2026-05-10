package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerCompletesAllSteps(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewWriter(filepath.Join(dir, "run.jsonl"))
	defer w.Close()

	var calls int32
	r := &Runner{
		Steps: []StepConfig{
			{Step: 0, Concurrency: 2, Duration: 200 * time.Millisecond},
			{Step: 1, Concurrency: 2, Duration: 200 * time.Millisecond},
		},
		Models:  []string{"m1"},
		Tasks:   []string{"t1"},
		Writer:  w,
		MaxToks: 10,
		Temp:    0.7,
		Eval:    &Eval{ErrorRateThreshold: 0.5, NoSuccessWindow: 30 * time.Second, HardLimit: 10 * time.Minute},
		DoChat: func(ctx context.Context, req ChatRequest) ChatResult {
			atomic.AddInt32(&calls, 1)
			time.Sleep(20 * time.Millisecond)
			return ChatResult{HTTPCode: 200, Content: "ok", ID: "x", InTokens: 5, OutTokens: 1, TotalTokens: 6, LatencyMS: 20}
		},
		GetSample: func() Sample { return Sample{IP: "1.1.1.1", TSMS: time.Now().UnixMilli()} },
	}

	reason := r.Run(context.Background(), time.Now())
	if reason != StopComplete {
		t.Errorf("got %q, want %q", reason, StopComplete)
	}
	if atomic.LoadInt32(&calls) < 4 {
		t.Errorf("expected at least 4 chat calls, got %d", calls)
	}
}

func TestRunnerStopsOnHighErrorRate(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewWriter(filepath.Join(dir, "run.jsonl"))
	defer w.Close()

	r := &Runner{
		Steps: []StepConfig{
			{Step: 0, Concurrency: 2, Duration: 300 * time.Millisecond},
			{Step: 1, Concurrency: 2, Duration: 300 * time.Millisecond},
		},
		Models:  []string{"m1"},
		Tasks:   []string{"t1"},
		Writer:  w,
		MaxToks: 10,
		Eval:    &Eval{ErrorRateThreshold: 0.5, NoSuccessWindow: 60 * time.Second, HardLimit: 10 * time.Minute},
		DoChat: func(ctx context.Context, req ChatRequest) ChatResult {
			time.Sleep(20 * time.Millisecond)
			return ChatResult{HTTPCode: 429, Content: "rl", LatencyMS: 20}
		},
		GetSample: func() Sample { return Sample{IP: "1.1.1.1", TSMS: time.Now().UnixMilli()} },
	}
	reason := r.Run(context.Background(), time.Now())
	if reason != StopErrorRate {
		t.Errorf("got %q, want %q", reason, StopErrorRate)
	}

	f, _ := os.Open(filepath.Join(dir, "run.jsonl"))
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var row Row
		_ = json.Unmarshal(sc.Bytes(), &row)
		if row.Step != 0 {
			t.Errorf("expected only step=0 rows, got step=%d", row.Step)
		}
	}
}
