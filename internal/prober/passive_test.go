package prober

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestPassiveTailEmitsStatusCodes(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "main.log")
	if err := os.WriteFile(logPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	var emitted int32
	pt := &PassiveTail{
		Path: logPath,
		// Per Task 6.1 documented regex (matches CPA gin_logger lines):
		Pattern: `\[gin_logger\.go:[0-9]+\]\s+(\d{3})`,
		Emit: func(ts time.Time, code int) {
			if code == 403 {
				atomic.AddInt32(&emitted, 1)
			}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go pt.Run(ctx)

	time.Sleep(100 * time.Millisecond)
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("[2026-05-09 17:00:00] [a] [info ] [gin_logger.go:100] 200 |  184ms | 1.2.3.4 | GET /\n")
	f.WriteString("[2026-05-09 17:00:01] [a] [info ] [gin_logger.go:100] 403 |   42ms | 1.2.3.4 | POST /v1/x\n")
	f.WriteString("[2026-05-09 17:00:02] [a] [info ] [gin_logger.go:100] 403 |   31ms | 1.2.3.4 | POST /v1/x\n")
	f.Close()

	// Wait up to 1s for emissions
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&emitted) < 2 {
		time.Sleep(50 * time.Millisecond)
	}
	if atomic.LoadInt32(&emitted) != 2 {
		t.Errorf("emitted=%d, want 2", emitted)
	}
}
