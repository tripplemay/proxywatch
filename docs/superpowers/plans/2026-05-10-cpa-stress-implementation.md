# cpa-stress Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `cpa-stress` — a Go CLI tool that runs a stair-step ramp stress test against `https://api.vpanel.cc`, captures every request's prompt/response/tokens/latency/exit-IP into JSONL, and produces a markdown summary report.

**Architecture:** Single Go binary, separate Go module under `tools/cpa-stress/`. Worker-pool design with one goroutine per concurrency slot. Side-channel ipify-through-SOCKS5 sampler tags each request with the most recent exit IP. JSONL written line-by-line during the run; reporter scans the file at the end.

**Tech Stack:** Go 1.25, stdlib only (no third-party deps if avoidable; `golang.org/x/net/proxy` for SOCKS5 dialing). Tests use stdlib `testing`, `httptest`, `t.TempDir`.

**Reference spec:** [`docs/superpowers/specs/2026-05-10-cpa-stress-design.md`](../specs/2026-05-10-cpa-stress-design.md)

**Repo:** `tripplemay/proxywatch`, branch `main`. Output committed under `tools/cpa-stress/`.

---

## Conventions

- Go module path: `github.com/tripplemay/proxywatch/tools/cpa-stress`
- Each task ends with one or more commits, conventional-commit style (`feat:`, `chore:`, `test:`, `docs:`)
- `git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" commit ...` (do not modify global git config)
- Tests run with `go test ./...` (no `-race` locally — gcc not installed; CI runs with race)
- Run `gofmt -l .` before each commit; if it lists any of YOUR new files, run `gofmt -w` on them

---

# Phase 0 — Skeleton

## Task 0.1: Initialize tools/cpa-stress/ Go module

**Files:**
- Create: `tools/cpa-stress/go.mod`
- Create: `tools/cpa-stress/main.go`

- [ ] **Step 1: Create directory and init module**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
mkdir -p tools/cpa-stress
cd tools/cpa-stress
go mod init github.com/tripplemay/proxywatch/tools/cpa-stress
```

- [ ] **Step 2: Pin Go version**

Edit `tools/cpa-stress/go.mod` so the `go` line reads `go 1.25` (match top-level proxywatch).

- [ ] **Step 3: Create main.go (CLI skeleton)**

`tools/cpa-stress/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.1.0-dev"

func main() {
	var (
		apiKey    string
		baseURL   string
		socksURL  string
		outputDir string
		dryRun    bool
		showVer   bool
	)
	flag.StringVar(&apiKey, "api-key", "", "CPA client API key (required)")
	flag.StringVar(&baseURL, "base-url", "https://api.vpanel.cc", "CPA base URL")
	flag.StringVar(&socksURL, "socks-url", "", "SOCKS5 URL for exit-IP sampling, e.g. socks5h://user:pass@host:port (required)")
	flag.StringVar(&outputDir, "output-dir", ".", "where to write run-<ts>.jsonl and report dir")
	flag.BoolVar(&dryRun, "dry-run", false, "short test (each step 30s, max C=4)")
	flag.BoolVar(&showVer, "version", false, "print version and exit")
	flag.Parse()

	if showVer {
		fmt.Println("cpa-stress", version)
		return
	}
	if apiKey == "" || socksURL == "" {
		fmt.Fprintln(os.Stderr, "error: -api-key and -socks-url are required")
		fmt.Fprintln(os.Stderr, "       -socks-url example: socks5h://user:pass@us.miyaip.online:1111")
		os.Exit(2)
	}

	fmt.Println("cpa-stress", version, "(skeleton — orchestration added in Task 6.1)")
	_ = baseURL
	_ = outputDir
	_ = dryRun
}
```

- [ ] **Step 4: Build and verify**

```bash
cd tools/cpa-stress
go build -o /tmp/cpa-stress ./
/tmp/cpa-stress -version
```

Expected: `cpa-stress 0.1.0-dev`.

```bash
/tmp/cpa-stress
```

Expected: exits with `error: -api-key and -socks-url are required`, exit code 2.

- [ ] **Step 5: Commit**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  add tools/cpa-stress/go.mod tools/cpa-stress/main.go
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  commit -m "feat(cpa-stress): scaffold tools/cpa-stress module with CLI flags"
```

---

# Phase 1 — Core types

## Task 1.1: prompts.go — task pool + model rotation

**Files:**
- Create: `tools/cpa-stress/prompts.go`
- Create: `tools/cpa-stress/prompts_test.go`

- [ ] **Step 1: Write failing tests**

`tools/cpa-stress/prompts_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestModelsList(t *testing.T) {
	want := []string{"gpt-5.2", "gpt-5.4", "gpt-5.4-mini", "gpt-5.5", "gpt-5.3-codex"}
	if len(Models) != len(want) {
		t.Fatalf("len(Models)=%d, want %d", len(Models), len(want))
	}
	for i, m := range want {
		if Models[i] != m {
			t.Errorf("Models[%d]=%q, want %q", i, Models[i], m)
		}
	}
}

func TestModelForRequestRoundRobin(t *testing.T) {
	for i, want := range []string{
		"gpt-5.2", "gpt-5.4", "gpt-5.4-mini", "gpt-5.5", "gpt-5.3-codex",
		"gpt-5.2", "gpt-5.4",
	} {
		got := ModelForRequest(int64(i))
		if got != want {
			t.Errorf("ModelForRequest(%d)=%q, want %q", i, got, want)
		}
	}
}

func TestTaskPoolNonEmpty(t *testing.T) {
	if len(Tasks) < 20 {
		t.Errorf("len(Tasks)=%d, want >=20", len(Tasks))
	}
	for i, task := range Tasks {
		if strings.TrimSpace(task) == "" {
			t.Errorf("Tasks[%d] is empty", i)
		}
	}
}

func TestBuildPrompt(t *testing.T) {
	p := BuildPrompt("reverses a string")
	if !strings.Contains(p, "reverses a string") {
		t.Errorf("expected task substring, got: %q", p)
	}
	if !strings.Contains(p, "Python function") {
		t.Errorf("expected 'Python function' in prompt, got: %q", p)
	}
}
```

- [ ] **Step 2: Run, expect failure**

```bash
cd tools/cpa-stress
go test ./...
```

Expected: compile error — `Models`, `ModelForRequest`, `Tasks`, `BuildPrompt` undefined.

- [ ] **Step 3: Implement**

`tools/cpa-stress/prompts.go`:

```go
package main

import "fmt"

// Models is the round-robin pool of gpt-* models to test.
var Models = []string{
	"gpt-5.2",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.5",
	"gpt-5.3-codex",
}

// ModelForRequest returns the model for the given request sequence number.
func ModelForRequest(seq int64) string {
	return Models[seq%int64(len(Models))]
}

// Tasks is the pool of task variants. BuildPrompt picks one and wraps it.
var Tasks = []string{
	"reverses a string",
	"checks if a number is prime",
	"parses an ISO 8601 date string",
	"merges two sorted lists into one sorted list",
	"counts word frequency in a text",
	"flattens a nested list of integers",
	"computes the nth Fibonacci number iteratively",
	"converts a hex color string to an RGB tuple",
	"validates an email address using a regex",
	"removes duplicates from a list while preserving order",
	"computes the longest common prefix of a list of strings",
	"capitalizes the first letter of each word in a sentence",
	"finds the second largest unique value in a list of integers",
	"transposes a 2D matrix represented as a list of lists",
	"converts a Roman numeral string to an integer",
	"computes the GCD of two positive integers",
	"checks if two strings are anagrams of each other",
	"flattens a deeply nested dictionary using dot notation",
	"computes the moving average of a list with given window size",
	"groups items in a list by a key function",
}

// BuildPrompt wraps a task into the canonical user message.
func BuildPrompt(task string) string {
	return fmt.Sprintf("Write a Python function that %s. Include a brief docstring.", task)
}
```

- [ ] **Step 4: Run, expect pass**

```bash
cd tools/cpa-stress
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  add tools/cpa-stress/prompts.go tools/cpa-stress/prompts_test.go
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  commit -m "feat(cpa-stress): prompt + model pool with round-robin"
```

---

## Task 1.2: jsonl.go — Row struct + concurrent-safe writer

**Files:**
- Create: `tools/cpa-stress/jsonl.go`
- Create: `tools/cpa-stress/jsonl_test.go`

- [ ] **Step 1: Write failing tests**

`tools/cpa-stress/jsonl_test.go`:

```go
package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "run.jsonl")
	w, err := NewWriter(p)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	row := Row{
		TSMS:        1700000000000,
		Step:        2,
		Concurrency: 4,
		WorkerID:    1,
		Model:       "gpt-5.4-mini",
		Prompt:      "Write a Python function that reverses a string.",
		Response: &RespBody{
			ID:           "resp_xxx",
			Content:      "ok",
			FinishReason: "stop",
		},
		HTTPCode:    200,
		LatencyMS:   1234,
		InTokens:    41,
		OutTokens:   163,
		TotalTokens: 204,
		ExitIP:      "1.2.3.4",
		ExitIPAgeMS: 320,
	}
	if err := w.Write(row); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, _ := os.Open(p)
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("expected one line")
	}
	var got Row
	if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.HTTPCode != 200 || got.Model != "gpt-5.4-mini" || got.Response == nil || got.Response.Content != "ok" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestWriterConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "run.jsonl")
	w, err := NewWriter(p)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = w.Write(Row{TSMS: int64(i), Step: i, HTTPCode: 200})
		}(i)
	}
	wg.Wait()

	f, _ := os.Open(p)
	defer f.Close()
	count := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r Row
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("malformed line %d: %v", count, err)
		}
		count++
	}
	if count != 50 {
		t.Errorf("got %d rows, want 50", count)
	}
}
```

- [ ] **Step 2: Run, expect failure**

```bash
cd tools/cpa-stress
go test ./...
```

- [ ] **Step 3: Implement**

`tools/cpa-stress/jsonl.go`:

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Row is one logged request.
type Row struct {
	TSMS         int64     `json:"ts_ms"`
	Step         int       `json:"step"`
	Concurrency  int       `json:"concurrency"`
	WorkerID     int       `json:"worker_id"`
	Model        string    `json:"model"`
	Prompt       string    `json:"prompt"`
	Response     *RespBody `json:"response,omitempty"`
	HTTPCode     int       `json:"http_code"`
	LatencyMS    int       `json:"latency_ms"`
	InTokens     int       `json:"in_tokens"`
	OutTokens    int       `json:"out_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	ExitIP       string    `json:"exit_ip"`
	ExitIPAgeMS  int       `json:"exit_ip_age_ms"`
	Error        string    `json:"error"`
}

// RespBody is the part of the upstream response we keep.
type RespBody struct {
	ID           string `json:"id"`
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason"`
}

// Writer serializes Rows to a JSONL file. Safe for concurrent use.
type Writer struct {
	mu   sync.Mutex
	f    *os.File
	buf  *bufio.Writer
	enc  *json.Encoder
}

// NewWriter creates and opens (truncating) the output file.
func NewWriter(path string) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	buf := bufio.NewWriterSize(f, 64*1024)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	return &Writer{f: f, buf: buf, enc: enc}, nil
}

// Write serializes one row and flushes (so kills don't lose data).
func (w *Writer) Write(r Row) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.enc.Encode(r); err != nil {
		return err
	}
	return w.buf.Flush()
}

// Close flushes and closes the underlying file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.buf.Flush(); err != nil {
		_ = w.f.Close()
		return err
	}
	return w.f.Close()
}
```

- [ ] **Step 4: Run, expect pass**

```bash
cd tools/cpa-stress
go test ./...
```

- [ ] **Step 5: Commit**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  add tools/cpa-stress/jsonl.go tools/cpa-stress/jsonl_test.go
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  commit -m "feat(cpa-stress): JSONL Row schema + thread-safe writer"
```

---

## Task 1.3: stopcond.go — stop condition evaluator

**Files:**
- Create: `tools/cpa-stress/stopcond.go`
- Create: `tools/cpa-stress/stopcond_test.go`

- [ ] **Step 1: Write failing tests**

`tools/cpa-stress/stopcond_test.go`:

```go
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
	// All recent rows are failures, time spans > 30s
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

	// One recent success in the window
	rows[len(rows)-1].HTTPCode = 200
	if got := e.EvalGlobal(now, rows); got != "" {
		t.Errorf("with one success got %q, want ''", got)
	}
}
```

- [ ] **Step 2: Run, expect failure**

```bash
cd tools/cpa-stress
go test ./...
```

- [ ] **Step 3: Implement**

`tools/cpa-stress/stopcond.go`:

```go
package main

import "time"

// StopReason describes why the test ended.
type StopReason string

const (
	StopComplete  StopReason = "complete"
	StopErrorRate StopReason = "error_rate_exceeded"
	StopNoSuccess StopReason = "no_success_30s"
	StopTimeLimit StopReason = "time_limit"
	StopSignal    StopReason = "signal"
)

// Eval holds parameters for evaluating stop conditions.
type Eval struct {
	StartTime          time.Time
	HardLimit          time.Duration // total wall-clock cap
	ErrorRateThreshold float64       // e.g. 0.5 = 50%
	NoSuccessWindow    time.Duration // e.g. 30s
}

// isFailure returns true if the row counts as a failure for stop-condition purposes.
func isFailure(r Row) bool {
	if r.Error != "" {
		return true
	}
	if r.HTTPCode == 0 {
		return true
	}
	if r.HTTPCode >= 400 {
		return true
	}
	return false
}

// EvalStep returns a stop reason if the step's rows breach the error-rate threshold,
// else "". Empty input returns "".
func (e *Eval) EvalStep(rows []Row) StopReason {
	if len(rows) == 0 {
		return ""
	}
	fails := 0
	for _, r := range rows {
		if isFailure(r) {
			fails++
		}
	}
	rate := float64(fails) / float64(len(rows))
	if rate >= e.ErrorRateThreshold {
		return StopErrorRate
	}
	return ""
}

// EvalGlobal checks time-limit and no-success-in-window conditions.
// `recent` should be all rows whose ts falls in the last NoSuccessWindow seconds.
func (e *Eval) EvalGlobal(now time.Time, recent []Row) StopReason {
	if e.HardLimit > 0 && now.Sub(e.StartTime) >= e.HardLimit {
		return StopTimeLimit
	}
	if e.NoSuccessWindow == 0 {
		return ""
	}
	cutoff := now.Add(-e.NoSuccessWindow).UnixMilli()
	hadSuccess := false
	hadAny := false
	for _, r := range recent {
		if r.TSMS < cutoff {
			continue
		}
		hadAny = true
		if !isFailure(r) {
			hadSuccess = true
			break
		}
	}
	if hadAny && !hadSuccess {
		return StopNoSuccess
	}
	return ""
}
```

- [ ] **Step 4: Run, expect pass**

```bash
cd tools/cpa-stress
go test ./...
```

- [ ] **Step 5: Commit**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  add tools/cpa-stress/stopcond.go tools/cpa-stress/stopcond_test.go
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  commit -m "feat(cpa-stress): stop-condition evaluator (errors/time/no-success)"
```

---

# Phase 2 — HTTP client

## Task 2.1: client.go — OpenAI-compatible chat client

**Files:**
- Create: `tools/cpa-stress/client.go`
- Create: `tools/cpa-stress/client_test.go`

- [ ] **Step 1: Write failing tests**

`tools/cpa-stress/client_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo the request to verify body
		body, _ := io.ReadAll(r.Body)
		var req ChatRequest
		_ = json.Unmarshal(body, &req)
		if r.Header.Get("Authorization") != "Bearer testkey" {
			t.Errorf("missing or wrong auth header")
		}
		if req.Model != "gpt-5.4-mini" {
			t.Errorf("model=%q", req.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":"resp_x",
			"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}
		}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "testkey", HTTP: srv.Client(), Timeout: 5 * time.Second}
	res := c.Chat(context.Background(), ChatRequest{
		Model:       "gpt-5.4-mini",
		Messages:    []Message{{Role: "user", Content: "hi"}},
		MaxTokens:   100,
		Temperature: 0.7,
	})
	if res.HTTPCode != 200 {
		t.Errorf("HTTPCode=%d", res.HTTPCode)
	}
	if res.Content != "hello" {
		t.Errorf("Content=%q", res.Content)
	}
	if res.ID != "resp_x" {
		t.Errorf("ID=%q", res.ID)
	}
	if res.InTokens != 5 || res.OutTokens != 2 || res.TotalTokens != 7 {
		t.Errorf("tokens: %+v", res)
	}
	if res.FinishReason != "stop" {
		t.Errorf("FinishReason=%q", res.FinishReason)
	}
	if res.Error != "" {
		t.Errorf("unexpected error: %s", res.Error)
	}
	if res.LatencyMS <= 0 {
		t.Errorf("LatencyMS=%d", res.LatencyMS)
	}
}

func TestChat4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, APIKey: "k", HTTP: srv.Client(), Timeout: 5 * time.Second}
	res := c.Chat(context.Background(), ChatRequest{Model: "x", Messages: []Message{{Role: "user", Content: "hi"}}})
	if res.HTTPCode != 429 {
		t.Errorf("HTTPCode=%d", res.HTTPCode)
	}
	if !strings.Contains(res.Content, "rate limited") {
		t.Errorf("content should include error body, got %q", res.Content)
	}
	// Error string is empty for HTTP errors (only for transport errors).
	if res.Error != "" {
		t.Errorf("Error should be empty for HTTP 4xx, got %q", res.Error)
	}
}

func TestChatTransportError(t *testing.T) {
	c := &Client{
		BaseURL: "http://127.0.0.1:1", // refused
		APIKey:  "k",
		HTTP:    &http.Client{Timeout: 500 * time.Millisecond},
		Timeout: 500 * time.Millisecond,
	}
	res := c.Chat(context.Background(), ChatRequest{Model: "x", Messages: []Message{{Role: "user", Content: "hi"}}})
	if res.HTTPCode != 0 {
		t.Errorf("HTTPCode=%d, want 0 on transport error", res.HTTPCode)
	}
	if res.Error == "" {
		t.Error("Error should be populated on transport error")
	}
}
```

- [ ] **Step 2: Run, expect failure**

- [ ] **Step 3: Implement**

`tools/cpa-stress/client.go`:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Message is one chat-completion message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is sent to /v1/chat/completions.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

// ChatResult bundles everything we want to log per-request.
type ChatResult struct {
	HTTPCode     int
	ID           string
	Content      string
	FinishReason string
	InTokens     int
	OutTokens    int
	TotalTokens  int
	LatencyMS    int
	Error        string // populated only on transport / decode failure
}

// Client is an OpenAI-compatible chat-completions client.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	Timeout time.Duration
}

// chatRespEnvelope mirrors the OpenAI response shape we care about.
type chatRespEnvelope struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

const maxContentBytes = 4096

// Chat issues one chat-completion request and returns the result. Never panics
// on HTTP errors; returns a populated ChatResult with HTTPCode and Error fields.
func (c *Client) Chat(ctx context.Context, req ChatRequest) ChatResult {
	start := time.Now()
	res := ChatResult{}

	rctx := ctx
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		rctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	body, err := json.Marshal(req)
	if err != nil {
		res.Error = fmt.Sprintf("marshal: %v", err)
		res.LatencyMS = int(time.Since(start).Milliseconds())
		return res
	}

	httpReq, err := http.NewRequestWithContext(rctx, "POST", c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		res.Error = fmt.Sprintf("build req: %v", err)
		res.LatencyMS = int(time.Since(start).Milliseconds())
		return res
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	res.LatencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	res.HTTPCode = resp.StatusCode

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		res.Error = fmt.Sprintf("read: %v", err)
		return res
	}

	// On non-2xx, return the body as Content (truncated) for diagnostics.
	if resp.StatusCode >= 400 {
		s := string(rawBody)
		if len(s) > maxContentBytes {
			s = s[:maxContentBytes]
		}
		res.Content = s
		return res
	}

	var env chatRespEnvelope
	if err := json.Unmarshal(rawBody, &env); err != nil {
		res.Error = fmt.Sprintf("decode: %v", err)
		return res
	}
	res.ID = env.ID
	if len(env.Choices) > 0 {
		c := env.Choices[0]
		s := c.Message.Content
		if len(s) > maxContentBytes {
			s = s[:maxContentBytes]
		}
		res.Content = s
		res.FinishReason = c.FinishReason
	}
	res.InTokens = env.Usage.PromptTokens
	res.OutTokens = env.Usage.CompletionTokens
	res.TotalTokens = env.Usage.TotalTokens
	return res
}
```

- [ ] **Step 4: Run, expect pass**

```bash
cd tools/cpa-stress
go test ./...
```

- [ ] **Step 5: Commit**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  add tools/cpa-stress/client.go tools/cpa-stress/client_test.go
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  commit -m "feat(cpa-stress): OpenAI-compatible chat client with full result capture"
```

---

# Phase 3 — IP sampler

## Task 3.1: ipsampler.go — SOCKS5 ipify sampler

**Files:**
- Modify: `tools/cpa-stress/go.mod` (add `golang.org/x/net`)
- Create: `tools/cpa-stress/ipsampler.go`
- Create: `tools/cpa-stress/ipsampler_test.go`

- [ ] **Step 1: Add SOCKS5 dependency**

```bash
cd tools/cpa-stress
go get golang.org/x/net/proxy
```

- [ ] **Step 2: Write failing tests (using a mock HTTP server, bypass SOCKS5 in tests)**

`tools/cpa-stress/ipsampler_test.go`:

```go
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSamplerLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("9.9.9.9"))
	}))
	defer srv.Close()

	s := &Sampler{
		Endpoints: []string{srv.URL},
		HTTP:      srv.Client(),
		Timeout:   2 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go s.Run(ctx, 50*time.Millisecond)

	// Wait until at least one sample is recorded
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if s.Latest().IP != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := s.Latest()
	if got.IP != "9.9.9.9" {
		t.Errorf("Latest().IP=%q, want 9.9.9.9", got.IP)
	}
	if got.TSMS == 0 {
		t.Error("Latest().TSMS should be set")
	}
}

func TestSamplerFallbackOnError(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("4.4.4.4"))
	}))
	defer good.Close()

	s := &Sampler{
		Endpoints: []string{bad.URL, good.URL},
		HTTP:      bad.Client(),
		Timeout:   2 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go s.Run(ctx, 50*time.Millisecond)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if s.Latest().IP != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s.Latest().IP != "4.4.4.4" {
		t.Errorf("expected fallback to second endpoint, got %q", s.Latest().IP)
	}
}
```

- [ ] **Step 3: Run, expect failure**

- [ ] **Step 4: Implement**

`tools/cpa-stress/ipsampler.go`:

```go
package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// Sample is one captured exit IP at a moment in time.
type Sample struct {
	TSMS int64
	IP   string
}

// Sampler periodically queries an "ip lookup" endpoint and stores the latest result.
// Use NewSamplerOverSOCKS5 in production to route lookups through the proxy.
type Sampler struct {
	Endpoints []string
	HTTP      *http.Client
	Timeout   time.Duration

	mu     sync.RWMutex
	latest Sample
}

// DefaultIPLookupEndpoints — lookups are tried in order until one succeeds.
var DefaultIPLookupEndpoints = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
}

// NewSamplerOverSOCKS5 builds a Sampler whose HTTP client routes through the SOCKS5 URL.
func NewSamplerOverSOCKS5(socksURL string, timeout time.Duration) (*Sampler, error) {
	u, err := url.Parse(socksURL)
	if err != nil {
		return nil, err
	}
	var auth *proxy.Auth
	if u.User != nil {
		pw, _ := u.User.Password()
		auth = &proxy.Auth{User: u.User.Username(), Password: pw}
	}
	dialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{Dial: dialer.Dial},
	}
	return &Sampler{
		Endpoints: DefaultIPLookupEndpoints,
		HTTP:      hc,
		Timeout:   timeout,
	}, nil
}

// Run polls every interval until ctx is cancelled. On each tick, the first
// reachable endpoint that returns a non-empty body wins.
func (s *Sampler) Run(ctx context.Context, interval time.Duration) {
	for {
		s.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (s *Sampler) tick(ctx context.Context) {
	for _, ep := range s.Endpoints {
		req, err := http.NewRequestWithContext(ctx, "GET", ep, nil)
		if err != nil {
			continue
		}
		resp, err := s.HTTP.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if ip == "" {
			continue
		}
		s.mu.Lock()
		s.latest = Sample{TSMS: time.Now().UnixMilli(), IP: ip}
		s.mu.Unlock()
		return
	}
}

// Latest returns the most recent successful sample (zero Sample if none yet).
func (s *Sampler) Latest() Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest
}
```

- [ ] **Step 5: Run, expect pass**

```bash
cd tools/cpa-stress
go test ./...
```

- [ ] **Step 6: Commit**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  add tools/cpa-stress/go.mod tools/cpa-stress/go.sum \
      tools/cpa-stress/ipsampler.go tools/cpa-stress/ipsampler_test.go
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  commit -m "feat(cpa-stress): SOCKS5-routed ipify sampler with fallback endpoints"
```

---

# Phase 4 — Runner

## Task 4.1: runner.go — worker pool + step driver

**Files:**
- Create: `tools/cpa-stress/runner.go`
- Create: `tools/cpa-stress/runner_test.go`

- [ ] **Step 1: Write failing tests**

`tools/cpa-stress/runner_test.go`:

```go
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
		Models:   []string{"m1"},
		Tasks:    []string{"t1"},
		Writer:   w,
		MaxToks:  10,
		Temp:     0.7,
		Eval:     &Eval{ErrorRateThreshold: 0.5, NoSuccessWindow: 30 * time.Second, HardLimit: 10 * time.Minute},
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

	// Verify step 1 never ran (we stop after step 0 fails the threshold)
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
```

- [ ] **Step 2: Run, expect failure**

- [ ] **Step 3: Implement**

`tools/cpa-stress/runner.go`:

```go
package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// StepConfig is one ramp step.
type StepConfig struct {
	Step        int
	Concurrency int
	Duration    time.Duration
}

// Runner drives the stair-step ramp test. All upstream interactions are pluggable
// via function fields so tests can mock them.
type Runner struct {
	Steps     []StepConfig
	Models    []string
	Tasks     []string
	Writer    *Writer
	MaxToks   int
	Temp      float64
	Eval      *Eval
	DoChat    func(ctx context.Context, req ChatRequest) ChatResult
	GetSample func() Sample

	seq atomic.Int64 // global request counter for round-robin model selection
}

// Run executes the configured steps until completion or stop. Returns the stop reason.
// `start` is when the test began (for hard-limit calculations).
func (r *Runner) Run(ctx context.Context, start time.Time) StopReason {
	r.Eval.StartTime = start

	for _, step := range r.Steps {
		// Check global stop (time, no-success) before starting next step
		if reason := r.Eval.EvalGlobal(time.Now(), nil); reason != "" {
			return reason
		}

		stepRows := r.runStep(ctx, step)

		// Did this step trip the error rate?
		if reason := r.Eval.EvalStep(stepRows); reason != "" {
			return reason
		}
		// Time check after each step
		if reason := r.Eval.EvalGlobal(time.Now(), stepRows); reason != "" {
			return reason
		}
		if ctx.Err() != nil {
			return StopSignal
		}
	}
	return StopComplete
}

// runStep launches `step.Concurrency` workers for `step.Duration`, returns all rows produced.
func (r *Runner) runStep(ctx context.Context, step StepConfig) []Row {
	stepCtx, cancel := context.WithTimeout(ctx, step.Duration)
	defer cancel()

	var (
		mu   sync.Mutex
		rows []Row
		wg   sync.WaitGroup
	)
	collect := func(row Row) {
		mu.Lock()
		rows = append(rows, row)
		mu.Unlock()
	}

	for w := 0; w < step.Concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for stepCtx.Err() == nil {
				row := r.fireOne(stepCtx, step, workerID)
				_ = r.Writer.Write(row)
				collect(row)

				// During no-success-window check (per-tick global eval) we want to
				// catch deteriorating conditions mid-step too.
				if reason := r.Eval.EvalGlobal(time.Now(), rows); reason != "" {
					cancel()
					return
				}
			}
		}(w)
	}
	wg.Wait()
	return rows
}

func (r *Runner) fireOne(ctx context.Context, step StepConfig, workerID int) Row {
	seq := r.seq.Add(1) - 1
	model := r.Models[seq%int64(len(r.Models))]

	// random-ish task pick: use seq mod len(tasks) for determinism in tests
	task := r.Tasks[int(seq)%len(r.Tasks)]
	prompt := BuildPrompt(task)

	sample := r.GetSample()
	now := time.Now()

	res := r.DoChat(ctx, ChatRequest{
		Model:       model,
		Messages:    []Message{{Role: "user", Content: prompt}},
		MaxTokens:   r.MaxToks,
		Temperature: r.Temp,
	})

	row := Row{
		TSMS:        now.UnixMilli(),
		Step:        step.Step,
		Concurrency: step.Concurrency,
		WorkerID:    workerID,
		Model:       model,
		Prompt:      prompt,
		HTTPCode:    res.HTTPCode,
		LatencyMS:   res.LatencyMS,
		InTokens:    res.InTokens,
		OutTokens:   res.OutTokens,
		TotalTokens: res.TotalTokens,
		ExitIP:      sample.IP,
		Error:       res.Error,
	}
	if sample.TSMS != 0 {
		row.ExitIPAgeMS = int(now.UnixMilli() - sample.TSMS)
	}
	if res.HTTPCode == 200 || res.Content != "" {
		row.Response = &RespBody{
			ID:           res.ID,
			Content:      res.Content,
			FinishReason: res.FinishReason,
		}
	}
	return row
}
```

- [ ] **Step 4: Run, expect pass**

```bash
cd tools/cpa-stress
go test ./...
```

- [ ] **Step 5: Commit**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  add tools/cpa-stress/runner.go tools/cpa-stress/runner_test.go
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  commit -m "feat(cpa-stress): worker-pool runner with stair-step ramp + stop checks"
```

---

# Phase 5 — Reporter

## Task 5.1: reporter.go — JSONL → markdown report

**Files:**
- Create: `tools/cpa-stress/reporter.go`
- Create: `tools/cpa-stress/reporter_test.go`

- [ ] **Step 1: Write failing tests**

`tools/cpa-stress/reporter_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReport(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "run.jsonl")
	f, _ := os.Create(jsonlPath)
	enc := json.NewEncoder(f)
	rows := []Row{
		{TSMS: 1, Step: 0, Concurrency: 1, Model: "gpt-5.2", HTTPCode: 200, LatencyMS: 100, InTokens: 5, OutTokens: 10, TotalTokens: 15, ExitIP: "1.1.1.1"},
		{TSMS: 2, Step: 0, Concurrency: 1, Model: "gpt-5.2", HTTPCode: 200, LatencyMS: 110, InTokens: 5, OutTokens: 10, TotalTokens: 15, ExitIP: "1.1.1.1"},
		{TSMS: 3, Step: 1, Concurrency: 2, Model: "gpt-5.4", HTTPCode: 429, LatencyMS: 50, ExitIP: "2.2.2.2"},
	}
	for _, r := range rows {
		_ = enc.Encode(r)
	}
	f.Close()

	rep, err := LoadReport(jsonlPath)
	if err != nil {
		t.Fatalf("LoadReport: %v", err)
	}
	if len(rep.Rows) != 3 {
		t.Errorf("Rows count=%d", len(rep.Rows))
	}
	if rep.Rows[0].HTTPCode != 200 {
		t.Errorf("first row HTTPCode=%d", rep.Rows[0].HTTPCode)
	}
}

func TestWriteMarkdownContents(t *testing.T) {
	rep := &Report{
		StoppedReason: StopErrorRate,
		Rows: []Row{
			{Step: 0, Concurrency: 1, Model: "gpt-5.2", HTTPCode: 200, LatencyMS: 100, InTokens: 5, OutTokens: 10, TotalTokens: 15, ExitIP: "1.1.1.1"},
			{Step: 0, Concurrency: 1, Model: "gpt-5.2", HTTPCode: 200, LatencyMS: 110, InTokens: 5, OutTokens: 10, TotalTokens: 15, ExitIP: "1.1.1.1"},
			{Step: 1, Concurrency: 2, Model: "gpt-5.4", HTTPCode: 429, LatencyMS: 50, ExitIP: "2.2.2.2"},
		},
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "report.md")
	if err := rep.WriteMarkdown(out); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	b, _ := os.ReadFile(out)
	s := string(b)

	for _, want := range []string{
		"# CPA Stress Test Report",
		"Stopped reason",
		"error_rate_exceeded",
		"## Per-step",
		"## Per-model",
		"## Exit IP histogram",
		"## Errors detail",
		"gpt-5.2",
		"gpt-5.4",
		"1.1.1.1",
		"2.2.2.2",
		"429",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in report", want)
		}
	}
}
```

- [ ] **Step 2: Run, expect failure**

- [ ] **Step 3: Implement**

`tools/cpa-stress/reporter.go`:

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Report holds rows + metadata produced by a run.
type Report struct {
	StartTime     time.Time
	EndTime       time.Time
	StoppedReason StopReason
	Rows          []Row
}

// LoadReport reads a JSONL file written by Writer.
func LoadReport(path string) (*Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := &Report{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		var row Row
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			continue // skip malformed lines
		}
		r.Rows = append(r.Rows, row)
	}
	return r, sc.Err()
}

type stepStat struct {
	step        int
	concurrency int
	count       int
	ok          int
	c4xx        int
	c5xx        int
	terr        int // transport errors (HTTPCode==0 + Error!="")
	durationMS  int64
	startMS     int64
	endMS       int64
	latencies   []int
	tokIn       int
	tokOut      int
}

type modelStat struct {
	model      string
	count      int
	ok         int
	c4xx       int
	totLatency int64
	tokIn      int
	tokOut     int
}

type ipStat struct {
	ip       string
	count    int
	ok       int
	c4xx     int
	firstStep int
	lastStep  int
}

func percentile(latencies []int, p float64) int {
	if len(latencies) == 0 {
		return 0
	}
	cp := append([]int(nil), latencies...)
	sort.Ints(cp)
	idx := int(float64(len(cp)) * p)
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

// WriteMarkdown serializes the report to a markdown file.
func (rep *Report) WriteMarkdown(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	// --- Summary ---
	now := time.Now()
	if rep.EndTime.IsZero() {
		rep.EndTime = now
	}
	dur := rep.EndTime.Sub(rep.StartTime)
	totalIn, totalOut := 0, 0
	for _, r := range rep.Rows {
		totalIn += r.InTokens
		totalOut += r.OutTokens
	}

	fmt.Fprintf(w, "# CPA Stress Test Report — %s\n\n", rep.EndTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "## Summary\n")
	fmt.Fprintf(w, "- Total duration: %s\n", dur.Round(time.Second))
	fmt.Fprintf(w, "- Total requests: %d\n", len(rep.Rows))
	fmt.Fprintf(w, "- Stopped reason: `%s`\n", rep.StoppedReason)
	fmt.Fprintf(w, "- Total input tokens: %d\n", totalIn)
	fmt.Fprintf(w, "- Total output tokens: %d\n\n", totalOut)

	// --- Per-step ---
	stepIdx := map[int]*stepStat{}
	for _, row := range rep.Rows {
		s, ok := stepIdx[row.Step]
		if !ok {
			s = &stepStat{step: row.Step, concurrency: row.Concurrency, startMS: row.TSMS}
			stepIdx[row.Step] = s
		}
		s.count++
		if row.TSMS > s.endMS {
			s.endMS = row.TSMS
		}
		if row.TSMS < s.startMS || s.startMS == 0 {
			s.startMS = row.TSMS
		}
		switch {
		case row.Error != "" && row.HTTPCode == 0:
			s.terr++
		case row.HTTPCode >= 500:
			s.c5xx++
		case row.HTTPCode >= 400:
			s.c4xx++
		case row.HTTPCode >= 200 && row.HTTPCode < 400:
			s.ok++
		}
		s.latencies = append(s.latencies, row.LatencyMS)
		s.tokIn += row.InTokens
		s.tokOut += row.OutTokens
	}
	steps := make([]int, 0, len(stepIdx))
	for k := range stepIdx {
		steps = append(steps, k)
	}
	sort.Ints(steps)

	fmt.Fprintf(w, "## Per-step\n\n")
	fmt.Fprintf(w, "| Step | C | Duration | Reqs | OK | 4xx | 5xx | err | RPS | p50 ms | p95 ms | tok in/out avg |\n")
	fmt.Fprintf(w, "|------|---|----------|------|----|-----|-----|-----|-----|--------|--------|----------------|\n")
	for _, k := range steps {
		s := stepIdx[k]
		var rps float64
		if s.endMS > s.startMS {
			rps = float64(s.count) * 1000.0 / float64(s.endMS-s.startMS)
		}
		dms := s.endMS - s.startMS
		var tokInAvg, tokOutAvg int
		if s.count > 0 {
			tokInAvg = s.tokIn / s.count
			tokOutAvg = s.tokOut / s.count
		}
		fmt.Fprintf(w, "| %d | %d | %ds | %d | %d | %d | %d | %d | %.2f | %d | %d | %d / %d |\n",
			s.step, s.concurrency, dms/1000, s.count, s.ok, s.c4xx, s.c5xx, s.terr,
			rps, percentile(s.latencies, 0.5), percentile(s.latencies, 0.95),
			tokInAvg, tokOutAvg)
	}

	// --- Per-model ---
	mIdx := map[string]*modelStat{}
	for _, row := range rep.Rows {
		m, ok := mIdx[row.Model]
		if !ok {
			m = &modelStat{model: row.Model}
			mIdx[row.Model] = m
		}
		m.count++
		if row.HTTPCode >= 200 && row.HTTPCode < 400 {
			m.ok++
		} else if row.HTTPCode >= 400 && row.HTTPCode < 500 {
			m.c4xx++
		}
		m.totLatency += int64(row.LatencyMS)
		m.tokIn += row.InTokens
		m.tokOut += row.OutTokens
	}
	models := make([]string, 0, len(mIdx))
	for k := range mIdx {
		models = append(models, k)
	}
	sort.Strings(models)

	fmt.Fprintf(w, "\n## Per-model\n\n")
	fmt.Fprintf(w, "| Model | Reqs | OK | 4xx | Avg latency ms | tok in/out avg |\n")
	fmt.Fprintf(w, "|-------|------|----|----|----------------|----------------|\n")
	for _, k := range models {
		m := mIdx[k]
		var avgLat int64
		var tokInAvg, tokOutAvg int
		if m.count > 0 {
			avgLat = m.totLatency / int64(m.count)
			tokInAvg = m.tokIn / m.count
			tokOutAvg = m.tokOut / m.count
		}
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d / %d |\n",
			m.model, m.count, m.ok, m.c4xx, avgLat, tokInAvg, tokOutAvg)
	}

	// --- Exit IP histogram ---
	ipIdx := map[string]*ipStat{}
	for _, row := range rep.Rows {
		if row.ExitIP == "" {
			continue
		}
		ip, ok := ipIdx[row.ExitIP]
		if !ok {
			ip = &ipStat{ip: row.ExitIP, firstStep: row.Step, lastStep: row.Step}
			ipIdx[row.ExitIP] = ip
		}
		ip.count++
		if row.HTTPCode >= 200 && row.HTTPCode < 400 {
			ip.ok++
		} else if row.HTTPCode >= 400 && row.HTTPCode < 500 {
			ip.c4xx++
		}
		if row.Step < ip.firstStep {
			ip.firstStep = row.Step
		}
		if row.Step > ip.lastStep {
			ip.lastStep = row.Step
		}
	}
	ips := make([]string, 0, len(ipIdx))
	for k := range ipIdx {
		ips = append(ips, k)
	}
	sort.Slice(ips, func(i, j int) bool { return ipIdx[ips[i]].count > ipIdx[ips[j]].count })

	fmt.Fprintf(w, "\n## Exit IP histogram\n\n")
	fmt.Fprintf(w, "| Exit IP | Reqs | OK | 4xx | First step | Last step |\n")
	fmt.Fprintf(w, "|---------|------|----|----|-----------|-----------|\n")
	for _, k := range ips {
		ip := ipIdx[k]
		fmt.Fprintf(w, "| `%s` | %d | %d | %d | %d | %d |\n", ip.ip, ip.count, ip.ok, ip.c4xx, ip.firstStep, ip.lastStep)
	}

	// --- Errors detail ---
	type errKey struct {
		code int
		msg  string
	}
	errIdx := map[errKey]int{}
	for _, row := range rep.Rows {
		if row.Error == "" && row.HTTPCode < 400 {
			continue
		}
		msg := row.Error
		if msg == "" {
			msg = firstLine(row.Response)
		}
		errIdx[errKey{code: row.HTTPCode, msg: truncate(msg, 80)}]++
	}
	type errRow struct {
		code  int
		msg   string
		count int
	}
	errRows := make([]errRow, 0, len(errIdx))
	for k, v := range errIdx {
		errRows = append(errRows, errRow{code: k.code, msg: k.msg, count: v})
	}
	sort.Slice(errRows, func(i, j int) bool { return errRows[i].count > errRows[j].count })

	fmt.Fprintf(w, "\n## Errors detail\n\n")
	if len(errRows) == 0 {
		fmt.Fprintf(w, "_No errors recorded._\n")
	} else {
		fmt.Fprintf(w, "| Code | Count | Sample message |\n")
		fmt.Fprintf(w, "|------|-------|----------------|\n")
		for _, er := range errRows {
			fmt.Fprintf(w, "| %d | %d | %s |\n", er.code, er.count, er.msg)
		}
	}

	// --- Caveats ---
	fmt.Fprintf(w, "\n## Caveats\n")
	fmt.Fprintf(w, "- `exit_ip` precision is ~1 second (sidecar ipify-via-SOCKS5 sampler).\n")
	fmt.Fprintf(w, "  At high concurrency, multiple requests in the same second may share an IP tag while the real exit IP rotated within that second.\n")
	fmt.Fprintf(w, "- Test consumed real ChatGPT subscription resources. Account-level rate limits may persist for hours after this run.\n")
	return nil
}

func firstLine(r *RespBody) string {
	if r == nil {
		return ""
	}
	for i, c := range r.Content {
		if c == '\n' {
			return r.Content[:i]
		}
	}
	return r.Content
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
```

- [ ] **Step 4: Run, expect pass**

- [ ] **Step 5: Commit**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  add tools/cpa-stress/reporter.go tools/cpa-stress/reporter_test.go
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  commit -m "feat(cpa-stress): JSONL → markdown reporter (per-step/model/IP/errors)"
```

---

# Phase 6 — Wire main + dry-run

## Task 6.1: main.go — full CLI orchestration

**Files:**
- Modify: `tools/cpa-stress/main.go` (replace skeleton)

- [ ] **Step 1: Replace main.go with full orchestration**

`tools/cpa-stress/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const version = "0.1.0-dev"

func main() {
	var (
		apiKey       string
		baseURL      string
		socksURL     string
		outputDir    string
		dryRun       bool
		stepDuration time.Duration
		showVer      bool
	)
	flag.StringVar(&apiKey, "api-key", "", "CPA client API key (required)")
	flag.StringVar(&baseURL, "base-url", "https://api.vpanel.cc", "CPA base URL")
	flag.StringVar(&socksURL, "socks-url", "", "SOCKS5 URL for exit-IP sampling, e.g. socks5h://user:pass@host:port (required)")
	flag.StringVar(&outputDir, "output-dir", ".", "where to write run-<ts>.jsonl and report dir")
	flag.BoolVar(&dryRun, "dry-run", false, "short test (each step 30s, max C=4)")
	flag.DurationVar(&stepDuration, "step-duration", 3*time.Minute, "per-step duration (overridden by -dry-run)")
	flag.BoolVar(&showVer, "version", false, "print version and exit")
	flag.Parse()

	if showVer {
		fmt.Println("cpa-stress", version)
		return
	}
	if apiKey == "" || socksURL == "" {
		fmt.Fprintln(os.Stderr, "error: -api-key and -socks-url are required")
		os.Exit(2)
	}

	startedAt := time.Now()
	ts := startedAt.Format("20060102-150405")
	jsonlPath := filepath.Join(outputDir, fmt.Sprintf("run-%s.jsonl", ts))
	reportDir := filepath.Join(outputDir, fmt.Sprintf("cpa-stress-report-%s", ts))
	reportPath := filepath.Join(reportDir, "report.md")

	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		log.Fatalf("mkdir reportDir: %v", err)
	}

	w, err := NewWriter(jsonlPath)
	if err != nil {
		log.Fatalf("open jsonl: %v", err)
	}
	defer w.Close()

	// SOCKS5-routed sampler
	sampler, err := NewSamplerOverSOCKS5(socksURL, 5*time.Second)
	if err != nil {
		log.Fatalf("build socks5 sampler: %v", err)
	}

	// Steps
	steps := buildSteps(dryRun, stepDuration)
	hardLimit := 25 * time.Minute
	if dryRun {
		hardLimit = 5 * time.Minute
	}

	// Real chat client
	chatClient := &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		Timeout: 60 * time.Second,
	}

	r := &Runner{
		Steps:   steps,
		Models:  Models,
		Tasks:   Tasks,
		Writer:  w,
		MaxToks: 200,
		Temp:    0.7,
		Eval: &Eval{
			HardLimit:          hardLimit,
			ErrorRateThreshold: 0.5,
			NoSuccessWindow:    30 * time.Second,
		},
		DoChat:    chatClient.Chat,
		GetSample: sampler.Latest,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start sampler
	go sampler.Run(ctx, 1*time.Second)

	log.Printf("cpa-stress %s starting", version)
	log.Printf("  jsonl  -> %s", jsonlPath)
	log.Printf("  report -> %s", reportPath)
	log.Printf("  steps  -> %d (dryRun=%v)", len(steps), dryRun)

	reason := r.Run(ctx, startedAt)
	if ctx.Err() != nil && reason == StopComplete {
		// signal arrived during the (rare) post-loop window
		reason = StopSignal
	}
	endedAt := time.Now()

	log.Printf("cpa-stress finished: reason=%s, duration=%s", reason, endedAt.Sub(startedAt).Round(time.Second))

	// Close writer before reporter reads it
	if err := w.Close(); err != nil {
		log.Printf("warn: close writer: %v", err)
	}

	// Generate report
	rep, err := LoadReport(jsonlPath)
	if err != nil {
		log.Fatalf("load report: %v", err)
	}
	rep.StartTime = startedAt
	rep.EndTime = endedAt
	rep.StoppedReason = reason
	if err := rep.WriteMarkdown(reportPath); err != nil {
		log.Fatalf("write markdown: %v", err)
	}

	log.Printf("report ready: %s", reportPath)
}

func buildSteps(dryRun bool, stepDur time.Duration) []StepConfig {
	if dryRun {
		return []StepConfig{
			{Step: 0, Concurrency: 1, Duration: 30 * time.Second},
			{Step: 1, Concurrency: 2, Duration: 30 * time.Second},
			{Step: 2, Concurrency: 4, Duration: 30 * time.Second},
		}
	}
	cs := []int{1, 2, 4, 8, 16, 32, 64}
	out := make([]StepConfig, len(cs))
	for i, c := range cs {
		out[i] = StepConfig{Step: i, Concurrency: c, Duration: stepDur}
	}
	return out
}
```

- [ ] **Step 2: Build + smoke**

```bash
cd tools/cpa-stress
go test ./...
go build -o /tmp/cpa-stress ./
/tmp/cpa-stress -version
```

Expected: `cpa-stress 0.1.0-dev`. All tests pass.

- [ ] **Step 3: gofmt clean**

```bash
gofmt -l .
```

Expected: empty output.

- [ ] **Step 4: Commit**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  add tools/cpa-stress/main.go
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  commit -m "feat(cpa-stress): wire CLI orchestration (steps, runner, sampler, reporter)"
```

---

## Task 6.2: README.md + dry-run checkpoint

**Files:**
- Create: `tools/cpa-stress/README.md`

- [ ] **Step 1: Write README**

`tools/cpa-stress/README.md`:

```markdown
# cpa-stress

A one-shot stress test for a CLIProxyAPI deployment. Stair-step ramp 1→2→4→8→16→32→64 concurrent workers (3 minutes each, 21 minutes total). Records every request to JSONL; produces a markdown report at the end.

**Spec:** [`docs/superpowers/specs/2026-05-10-cpa-stress-design.md`](../../docs/superpowers/specs/2026-05-10-cpa-stress-design.md)

## Build

```bash
cd tools/cpa-stress
go build -o cpa-stress ./
```

## Run (dry-run first)

```bash
./cpa-stress \
  -api-key <YOUR_CPA_API_KEY> \
  -socks-url 'socks5h://USER:PASS@us.miyaip.online:1111' \
  -base-url https://api.vpanel.cc \
  -output-dir /tmp/cpa-stress-out \
  -dry-run
```

Dry-run is short (~90 s, max C=4). Use this first to verify wiring; only switch off `-dry-run` for the real test.

## Run (real test)

```bash
./cpa-stress \
  -api-key <YOUR_CPA_API_KEY> \
  -socks-url 'socks5h://USER:PASS@us.miyaip.online:1111' \
  -base-url https://api.vpanel.cc \
  -output-dir /tmp/cpa-stress-out
```

Runs the full ramp. Up to 25 minutes wall-clock. Sends Ctrl+C → graceful shutdown + report.

## Stop conditions

The run terminates when ANY of these triggers:

- All 7 steps complete normally
- A single step has ≥50% failure rate
- 30 seconds with no successful response
- 25-minute hard cap
- Manual SIGINT (Ctrl+C)

## Outputs

```
/tmp/cpa-stress-out/
  run-20260510-153012.jsonl
  cpa-stress-report-20260510-153012/
    report.md
```

The JSONL file has one row per request (full schema in spec §4.1). The markdown report has per-step / per-model / exit-IP / error breakdowns.

## Risks

This test consumes real ChatGPT subscription resources via the proxy. It can:

- Trigger account-level rate limits (1-24h cooldowns)
- Get IPs in the SOCKS5 pool blacklisted by OpenAI
- In extreme cases, get accounts suspended

Only run when you've consciously accepted those risks.
```

- [ ] **Step 2: Commit**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  add tools/cpa-stress/README.md
git -c user.name="tripplemay" -c user.email="LloydRoberta668@zohomail.com" \
  commit -m "docs(cpa-stress): README with build/run/stop-conditions/risks"
```

- [ ] **Step 3: Push and verify CI**

```bash
git push
```

Wait for CI to complete. Expected: green. If CI fails, fix and push.

- [ ] **Step 4: HUMAN-IN-LOOP CHECKPOINT — dry-run on server**

Before merging or running real test, the controller (you) should ask the user to:

1. Confirm they want to proceed to a dry-run
2. Run on the production server:

   ```bash
   ssh root@<server-ip>
   cd /opt/proxywatch-src
   git pull
   cd tools/cpa-stress
   go build -o /opt/cpa-stress ./
   /opt/cpa-stress \
     -api-key 5d378b3ca96097d0e0a31b76965fc04bed3da0bc0de66d58 \
     -socks-url 'socks5h://1o7j3xahhcmiyaip:1g7t3gYWq9xIUHnosession@us.miyaip.online:1111' \
     -base-url https://api.vpanel.cc \
     -output-dir /tmp/cpa-stress-out \
     -dry-run
   ```

3. Inspect the dry-run report (~90 s) and confirm it looks reasonable
4. Decide: proceed to real test, or stop here

Do NOT proceed to a real test without explicit user confirmation after dry-run.

---

# Self-Review Checklist (run after plan is written)

The author of this plan should verify:

- [ ] Each spec section maps to at least one task:
  - §1 background/goal → README + main.go (CLI)
  - §2 architecture → tasks 0.1–6.1
  - §3 parameters → main.go + prompts.go + runner.go + stopcond.go
  - §3.5 exit-IP sampler → ipsampler.go
  - §4.1 JSONL schema → jsonl.go
  - §4.2 markdown report → reporter.go
  - §5 errors → client.go (error capture) + stopcond.go (failure classification)
  - §6 strategy → runner_test, dry-run checkpoint
  - §7 risks → README, dry-run checkpoint
  - §8 deployment → README

- [ ] No TBD/TODO placeholders. All steps include exact code or commands.

- [ ] Type/method consistency:
  - `Row` field names consistent (`HTTPCode`, `LatencyMS`, `ExitIPAgeMS` etc.)
  - `Sample{TSMS, IP}` consistent across `ipsampler.go` and `runner.go`
  - `ChatResult{HTTPCode, Content, ...}` consistent across `client.go` and `runner.go`
  - `StopReason` constants consistent across `stopcond.go`, `runner.go`, `main.go`
  - `Runner` struct fields (`MaxToks` vs `MaxTokens`) — used `MaxToks` consistently inside Runner
  - `Eval` field names (`ErrorRateThreshold`, `NoSuccessWindow`, `HardLimit`, `StartTime`) consistent

- [ ] `GetSample` is `func() Sample` consistently in both runner and main wiring.

- [ ] Tests cover happy path + at least one failure path for each component.
