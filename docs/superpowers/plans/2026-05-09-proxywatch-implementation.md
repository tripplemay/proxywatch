# proxywatch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build proxywatch — a Go sidecar that monitors a CLIProxyAPI deployment's SOCKS5 upstream proxy, alerts via Telegram when the exit IP is rate-limited, and verifies recovery after the operator manually rotates the IP at the proxy provider.

**Architecture:** Single Go binary, embedded React+TS frontend, SQLite for persistence. Two probes (active OpenAI call through SOCKS5; passive tail of CPA log files) feed a state machine (`HEALTHY → SUSPECT → ROTATING → VERIFYING → COOLDOWN`, with `ALERT_ONLY` fallback) that triggers Telegram alerts and observes recovery.

**Tech Stack:** Go 1.22+, `modernc.org/sqlite` (pure-Go SQLite), `golang.org/x/net/proxy` (SOCKS5), `fsnotify` (log tail), React 18 + TypeScript + Vite, Docker.

**Reference spec:** `docs/design/proxywatch-design.md`

**Repo:** `tripplemay/proxywatch`, branch `main`. All work is committed there.

---

## Conventions

- Go module: `github.com/tripplemay/proxywatch`
- Go version: 1.22 (matches CPA's stack)
- Test framework: stdlib `testing`; no external assertion libs in MVP
- Import order: stdlib → third-party → internal, separated by blank lines
- Errors: returned, not logged-and-swallowed; use `fmt.Errorf("context: %w", err)` for wrapping
- Time: `time.Time` everywhere except SQLite columns, which store `unix epoch ms` as `INTEGER`
- Logging: stdlib `log/slog` to stderr in JSON format
- Commit cadence: one commit per task (or per logical sub-step in long tasks); messages follow `feat: …` / `test: …` / `chore: …` / `fix: …` prefixes

---

# Phase 0 — Project Skeleton

## Task 0.1: Initialize Go module and base layout

**Files:**
- Create: `go.mod`
- Create: `cmd/proxywatch/main.go`
- Create: `Makefile`

- [ ] **Step 1: Initialize go module**

```bash
cd /mnt/c/Users/tripplezhou/project/apisub
go mod init github.com/tripplemay/proxywatch
```

Expected: creates `go.mod` with module declaration and `go 1.22` (or whatever's installed; we accept 1.22+).

- [ ] **Step 2: Pin Go version explicitly in go.mod**

Edit `go.mod` so the `go` directive reads `go 1.22`. If a higher toolchain was auto-selected, add `toolchain go1.22.5` or similar — keep the floor at 1.22.

- [ ] **Step 3: Create entry point that prints version**

`cmd/proxywatch/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

const version = "0.0.0-dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	fmt.Println("proxywatch", version)
}
```

- [ ] **Step 4: Build and run**

```bash
go build -o /tmp/proxywatch ./cmd/proxywatch
/tmp/proxywatch version
```

Expected output: `0.0.0-dev`

- [ ] **Step 5: Add Makefile**

`Makefile`:

```makefile
.PHONY: build test lint clean run

BINARY := proxywatch
GOFLAGS :=

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/proxywatch

test:
	go test -race ./...

lint:
	go vet ./...
	gofmt -l . | tee /dev/stderr | (! read)

run: build
	./bin/$(BINARY)

clean:
	rm -rf bin/ dist/ web/dist/
```

- [ ] **Step 6: Commit**

```bash
git add go.mod cmd/proxywatch/main.go Makefile
git commit -m "chore: initialize go module and base layout"
```

---

## Task 0.2: Add `.editorconfig` and confirm formatting

**Files:**
- Create: `.editorconfig`

- [ ] **Step 1: Create .editorconfig**

```editorconfig
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true

[*.go]
indent_style = tab

[{*.ts,*.tsx,*.js,*.jsx,*.json,*.yaml,*.yml,*.md}]
indent_style = space
indent_size = 2
```

- [ ] **Step 2: Commit**

```bash
git add .editorconfig
git commit -m "chore: add .editorconfig"
```

---

## Task 0.3: Add minimal Dockerfile that builds the current binary

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

- [ ] **Step 1: Create Dockerfile (multi-stage, Go-only for now)**

```dockerfile
# syntax=docker/dockerfile:1.6
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/proxywatch ./cmd/proxywatch

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/proxywatch /app/proxywatch
ENTRYPOINT ["/app/proxywatch"]
```

- [ ] **Step 2: Create .dockerignore**

```
.git/
.github/
.claude/
docs/
web/node_modules/
web/dist/
bin/
*.md
*.local.yaml
```

- [ ] **Step 3: Build the image and run**

```bash
docker build -t proxywatch:dev .
docker run --rm proxywatch:dev version
```

Expected: prints `0.0.0-dev`.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "chore: add Dockerfile (Go-only stage)"
```

---

## Task 0.4: GitHub Actions CI — build + test on push

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create workflow**

```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:

jobs:
  go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Vet
        run: go vet ./...
      - name: gofmt check
        run: |
          out=$(gofmt -l .)
          if [ -n "$out" ]; then
            echo "::error::Files need gofmt:"
            echo "$out"
            exit 1
          fi
      - name: Test
        run: go test -race -count=1 ./...
      - name: Build
        run: go build ./...
```

- [ ] **Step 2: Commit and push to verify CI passes**

```bash
git add .github/workflows/ci.yml
git commit -m "chore: add GitHub Actions CI"
git push
```

Expected: CI run completes green at https://github.com/tripplemay/proxywatch/actions

---

# Phase 1 — Storage Layer (SQLite)

## Task 1.1: Add SQLite dependency and create open/migrate function

**Files:**
- Modify: `go.mod`
- Create: `internal/store/store.go`
- Create: `internal/store/migrations.sql`
- Create: `internal/store/store_test.go`

- [ ] **Step 1: Add modernc.org/sqlite (pure-Go, no CGO)**

```bash
go get modernc.org/sqlite@latest
```

- [ ] **Step 2: Write the failing test for `Open`**

`internal/store/store_test.go`:

```go
package store

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	want := []string{"probes", "incidents", "rotations", "notifications", "config_kv"}
	for _, name := range want {
		var count int
		err := s.DB().QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`,
			name,
		).Scan(&count)
		if err != nil {
			t.Fatalf("query for %s: %v", name, err)
		}
		if count != 1 {
			t.Errorf("table %s: got %d rows, want 1", name, count)
		}
	}
}
```

- [ ] **Step 3: Run the test, expect failure**

```bash
go test ./internal/store/ -run TestOpenCreatesAllTables -v
```

Expected: compile error — `Open` undefined.

- [ ] **Step 4: Write `migrations.sql` with the spec's schema**

`internal/store/migrations.sql`:

```sql
CREATE TABLE IF NOT EXISTS probes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          INTEGER NOT NULL,
    kind        TEXT NOT NULL,
    target      TEXT,
    http_code   INTEGER,
    latency_ms  INTEGER,
    exit_ip     TEXT,
    ok          INTEGER NOT NULL,
    raw_error   TEXT
);
CREATE INDEX IF NOT EXISTS idx_probes_ts ON probes(ts);

CREATE TABLE IF NOT EXISTS incidents (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at      INTEGER NOT NULL,
    ended_at        INTEGER,
    trigger_reason  TEXT NOT NULL,
    initial_state   TEXT,
    terminal_state  TEXT,
    rotation_count  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS rotations (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    incident_id      INTEGER NOT NULL,
    started_at       INTEGER NOT NULL,
    ended_at         INTEGER,
    old_ip           TEXT,
    new_ip           TEXT,
    detection_method TEXT,
    ok               INTEGER,
    error            TEXT,
    FOREIGN KEY (incident_id) REFERENCES incidents(id)
);

CREATE TABLE IF NOT EXISTS notifications (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          INTEGER NOT NULL,
    incident_id INTEGER,
    level       TEXT NOT NULL,
    text        TEXT NOT NULL,
    sent_at     INTEGER,
    error       TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS config_kv (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
```

- [ ] **Step 5: Implement `Open` and embed migrations**

`internal/store/store.go`:

```go
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed migrations.sql
var migrationsSQL string

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := db.Exec(migrationsSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Close() error { return s.db.Close() }
```

- [ ] **Step 6: Run the test, expect pass**

```bash
go test ./internal/store/ -run TestOpenCreatesAllTables -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/store/
git commit -m "feat(store): SQLite open + schema migrations"
```

---

## Task 1.2: Probes table CRUD

**Files:**
- Create: `internal/store/probes.go`
- Create: `internal/store/probes_test.go`

- [ ] **Step 1: Write failing tests**

`internal/store/probes_test.go`:

```go
package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertProbeAndRecent(t *testing.T) {
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0)

	p := Probe{
		TS:        now,
		Kind:      "active",
		Target:    "https://api.openai.com/v1/models",
		HTTPCode:  200,
		LatencyMS: 312,
		ExitIP:    "172.58.213.36",
		OK:        true,
	}
	id, err := s.InsertProbe(p)
	if err != nil {
		t.Fatalf("InsertProbe: %v", err)
	}
	if id == 0 {
		t.Errorf("expected non-zero id, got %d", id)
	}

	got, err := s.RecentProbes(10, "")
	if err != nil {
		t.Fatalf("RecentProbes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	if got[0].HTTPCode != 200 || got[0].ExitIP != "172.58.213.36" {
		t.Errorf("round-trip mismatch: %+v", got[0])
	}
}

func TestRecentProbesFilteredByKind(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	for i, k := range []string{"active", "passive", "active"} {
		_, err := s.InsertProbe(Probe{
			TS:   now.Add(time.Duration(i) * time.Second),
			Kind: k,
			OK:   true,
		})
		if err != nil {
			t.Fatalf("InsertProbe[%d]: %v", i, err)
		}
	}
	got, err := s.RecentProbes(10, "active")
	if err != nil {
		t.Fatalf("RecentProbes: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("active filter: got %d, want 2", len(got))
	}
}
```

- [ ] **Step 2: Run, expect failure**

```bash
go test ./internal/store/ -run TestInsertProbe -v
```

Expected: compile errors — `Probe`, `InsertProbe`, `RecentProbes` undefined.

- [ ] **Step 3: Implement**

`internal/store/probes.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Probe struct {
	ID        int64
	TS        time.Time
	Kind      string // "active" | "passive"
	Target    string
	HTTPCode  int
	LatencyMS int
	ExitIP    string
	OK        bool
	RawError  string
}

func (s *Store) InsertProbe(p Probe) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO probes (ts, kind, target, http_code, latency_ms, exit_ip, ok, raw_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.TS.UnixMilli(), p.Kind, p.Target, p.HTTPCode, p.LatencyMS, p.ExitIP, boolToInt(p.OK), p.RawError,
	)
	if err != nil {
		return 0, fmt.Errorf("insert probe: %w", err)
	}
	return res.LastInsertId()
}

// RecentProbes returns up to limit rows, newest first.
// kind filter is optional ("" = no filter).
func (s *Store) RecentProbes(limit int, kind string) ([]Probe, error) {
	q := `SELECT id, ts, kind, target, http_code, latency_ms, exit_ip, ok, raw_error
	      FROM probes`
	args := []any{}
	if kind != "" {
		q += ` WHERE kind = ?`
		args = append(args, kind)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query probes: %w", err)
	}
	defer rows.Close()

	var out []Probe
	for rows.Next() {
		var (
			p      Probe
			tsMS   int64
			okInt  int
			tgt    sql.NullString
			ip     sql.NullString
			rawErr sql.NullString
		)
		if err := rows.Scan(&p.ID, &tsMS, &p.Kind, &tgt, &p.HTTPCode, &p.LatencyMS, &ip, &okInt, &rawErr); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		p.TS = time.UnixMilli(tsMS)
		p.Target = tgt.String
		p.ExitIP = ip.String
		p.OK = okInt == 1
		p.RawError = rawErr.String
		out = append(out, p)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Run tests, expect pass**

```bash
go test ./internal/store/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/probes.go internal/store/probes_test.go
git commit -m "feat(store): probes table CRUD with kind filter"
```

---

## Task 1.3: Incidents and rotations CRUD

**Files:**
- Create: `internal/store/incidents.go`
- Create: `internal/store/incidents_test.go`
- Create: `internal/store/rotations.go`
- Create: `internal/store/rotations_test.go`

- [ ] **Step 1: Write failing tests for incidents**

`internal/store/incidents_test.go`:

```go
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
```

- [ ] **Step 2: Run, expect failure**

```bash
go test ./internal/store/ -run TestIncidentLifecycle -v
```

Expected: undefined symbols.

- [ ] **Step 3: Implement incidents**

`internal/store/incidents.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Incident struct {
	ID             int64
	StartedAt      time.Time
	EndedAt        time.Time // zero if open
	TriggerReason  string    // "passive_4xx" | "active_failure" | "manual"
	InitialState   string
	TerminalState  string
	RotationCount  int
}

func (s *Store) OpenIncident(in Incident) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO incidents (started_at, trigger_reason, initial_state)
		 VALUES (?, ?, ?)`,
		in.StartedAt.UnixMilli(), in.TriggerReason, in.InitialState,
	)
	if err != nil {
		return 0, fmt.Errorf("open incident: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) CloseIncident(id int64, endedAt time.Time, terminalState string) error {
	_, err := s.db.Exec(
		`UPDATE incidents SET ended_at = ?, terminal_state = ? WHERE id = ?`,
		endedAt.UnixMilli(), terminalState, id,
	)
	return err
}

func (s *Store) IncrementRotationCount(id int64) error {
	_, err := s.db.Exec(`UPDATE incidents SET rotation_count = rotation_count + 1 WHERE id = ?`, id)
	return err
}

func (s *Store) OpenIncidents() ([]Incident, error) {
	return s.queryIncidents(`SELECT id, started_at, ended_at, trigger_reason, initial_state, terminal_state, rotation_count
	                          FROM incidents WHERE ended_at IS NULL ORDER BY id DESC`)
}

func (s *Store) RecentIncidents(limit int) ([]Incident, error) {
	return s.queryIncidents(
		`SELECT id, started_at, ended_at, trigger_reason, initial_state, terminal_state, rotation_count
		 FROM incidents ORDER BY id DESC LIMIT ?`,
		limit,
	)
}

func (s *Store) queryIncidents(q string, args ...any) ([]Incident, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query incidents: %w", err)
	}
	defer rows.Close()

	var out []Incident
	for rows.Next() {
		var (
			in        Incident
			startedMS int64
			endedMS   sql.NullInt64
			term      sql.NullString
			initial   sql.NullString
		)
		if err := rows.Scan(&in.ID, &startedMS, &endedMS, &in.TriggerReason, &initial, &term, &in.RotationCount); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		in.StartedAt = time.UnixMilli(startedMS)
		if endedMS.Valid {
			in.EndedAt = time.UnixMilli(endedMS.Int64)
		}
		in.InitialState = initial.String
		in.TerminalState = term.String
		out = append(out, in)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run incident tests, expect pass**

```bash
go test ./internal/store/ -run TestIncident -v
```

Expected: PASS.

- [ ] **Step 5: Write failing tests for rotations**

`internal/store/rotations_test.go`:

```go
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
```

- [ ] **Step 6: Implement rotations**

`internal/store/rotations.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Rotation struct {
	ID              int64
	IncidentID      int64
	StartedAt       time.Time
	EndedAt         time.Time // zero if still in flight
	OldIP           string
	NewIP           string
	DetectionMethod string // "auto" | "manual_button"
	OK              bool
	Error           string
}

func (s *Store) InsertRotation(r Rotation) (int64, error) {
	var endedMS sql.NullInt64
	if !r.EndedAt.IsZero() {
		endedMS.Valid = true
		endedMS.Int64 = r.EndedAt.UnixMilli()
	}
	res, err := s.db.Exec(
		`INSERT INTO rotations (incident_id, started_at, ended_at, old_ip, new_ip, detection_method, ok, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.IncidentID, r.StartedAt.UnixMilli(), endedMS,
		r.OldIP, r.NewIP, r.DetectionMethod, boolToInt(r.OK), r.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("insert rotation: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) RecentRotations(limit int) ([]Rotation, error) {
	rows, err := s.db.Query(
		`SELECT id, incident_id, started_at, ended_at, old_ip, new_ip, detection_method, ok, error
		 FROM rotations ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query rotations: %w", err)
	}
	defer rows.Close()

	var out []Rotation
	for rows.Next() {
		var (
			r        Rotation
			startMS  int64
			endMS    sql.NullInt64
			oldIP    sql.NullString
			newIP    sql.NullString
			method   sql.NullString
			okInt    sql.NullInt64
			errStr   sql.NullString
		)
		if err := rows.Scan(&r.ID, &r.IncidentID, &startMS, &endMS, &oldIP, &newIP, &method, &okInt, &errStr); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		r.StartedAt = time.UnixMilli(startMS)
		if endMS.Valid {
			r.EndedAt = time.UnixMilli(endMS.Int64)
		}
		r.OldIP = oldIP.String
		r.NewIP = newIP.String
		r.DetectionMethod = method.String
		r.OK = okInt.Valid && okInt.Int64 == 1
		r.Error = errStr.String
		out = append(out, r)
	}
	return out, rows.Err()
}
```

- [ ] **Step 7: Run all store tests, expect pass**

```bash
go test ./internal/store/ -v -race
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/store/incidents.go internal/store/incidents_test.go internal/store/rotations.go internal/store/rotations_test.go
git commit -m "feat(store): incidents + rotations CRUD"
```

---

## Task 1.4: Notifications queue + config_kv

**Files:**
- Create: `internal/store/notifications.go`
- Create: `internal/store/notifications_test.go`
- Create: `internal/store/kv.go`
- Create: `internal/store/kv_test.go`

- [ ] **Step 1: Write notifications test**

`internal/store/notifications_test.go`:

```go
package store

import (
	"testing"
	"time"
)

func TestEnqueueAndPending(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	id, err := s.EnqueueNotification(Notification{
		TS:    now,
		Level: "warning",
		Text:  "test alert",
	})
	if err != nil {
		t.Fatalf("EnqueueNotification: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	pending, err := s.PendingNotifications(100)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("len=%d, want 1", len(pending))
	}

	if err := s.MarkNotificationSent(id, now); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	pending, err = s.PendingNotifications(100)
	if err != nil {
		t.Fatalf("PendingNotifications 2: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("after MarkSent, pending=%d, want 0", len(pending))
	}
}

func TestRecordNotificationFailureIncrements(t *testing.T) {
	s := newStore(t)
	id, _ := s.EnqueueNotification(Notification{TS: time.Now(), Level: "info", Text: "x"})
	if err := s.RecordNotificationFailure(id, "503"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if err := s.RecordNotificationFailure(id, "503"); err != nil {
		t.Fatalf("RecordFailure 2: %v", err)
	}
	pending, _ := s.PendingNotifications(100)
	if pending[0].RetryCount != 2 {
		t.Errorf("retry_count=%d, want 2", pending[0].RetryCount)
	}
}
```

- [ ] **Step 2: Implement notifications**

`internal/store/notifications.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Notification struct {
	ID         int64
	TS         time.Time
	IncidentID int64 // 0 if not associated
	Level      string
	Text       string
	SentAt     time.Time // zero if pending
	Error      string
	RetryCount int
}

func (s *Store) EnqueueNotification(n Notification) (int64, error) {
	var incID sql.NullInt64
	if n.IncidentID > 0 {
		incID.Valid = true
		incID.Int64 = n.IncidentID
	}
	res, err := s.db.Exec(
		`INSERT INTO notifications (ts, incident_id, level, text) VALUES (?, ?, ?, ?)`,
		n.TS.UnixMilli(), incID, n.Level, n.Text,
	)
	if err != nil {
		return 0, fmt.Errorf("enqueue: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) PendingNotifications(limit int) ([]Notification, error) {
	rows, err := s.db.Query(
		`SELECT id, ts, incident_id, level, text, error, retry_count
		 FROM notifications WHERE sent_at IS NULL ORDER BY id ASC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("pending: %w", err)
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var (
			n      Notification
			tsMS   int64
			incID  sql.NullInt64
			errStr sql.NullString
		)
		if err := rows.Scan(&n.ID, &tsMS, &incID, &n.Level, &n.Text, &errStr, &n.RetryCount); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		n.TS = time.UnixMilli(tsMS)
		if incID.Valid {
			n.IncidentID = incID.Int64
		}
		n.Error = errStr.String
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) MarkNotificationSent(id int64, at time.Time) error {
	_, err := s.db.Exec(`UPDATE notifications SET sent_at = ?, error = NULL WHERE id = ?`, at.UnixMilli(), id)
	return err
}

func (s *Store) RecordNotificationFailure(id int64, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE notifications SET retry_count = retry_count + 1, error = ? WHERE id = ?`,
		errMsg, id,
	)
	return err
}
```

- [ ] **Step 3: Write KV test**

`internal/store/kv_test.go`:

```go
package store

import "testing"

func TestKVRoundTrip(t *testing.T) {
	s := newStore(t)
	if err := s.SetKV("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	v, ok, err := s.GetKV("foo")
	if err != nil || !ok || v != "bar" {
		t.Errorf("GetKV foo = %q, %v, %v; want bar, true, nil", v, ok, err)
	}

	if err := s.SetKV("foo", "baz"); err != nil {
		t.Fatal(err)
	}
	v, _, _ = s.GetKV("foo")
	if v != "baz" {
		t.Errorf("after update, foo = %q, want baz", v)
	}

	_, ok, _ = s.GetKV("missing")
	if ok {
		t.Error("missing key should return ok=false")
	}
}
```

- [ ] **Step 4: Implement KV**

`internal/store/kv.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) SetKV(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO config_kv (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("set kv: %w", err)
	}
	return nil
}

func (s *Store) GetKV(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM config_kv WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get kv: %w", err)
	}
	return v, true, nil
}
```

- [ ] **Step 5: Run all store tests, expect pass**

```bash
go test ./internal/store/ -v -race
```

- [ ] **Step 6: Commit**

```bash
git add internal/store/notifications.go internal/store/notifications_test.go internal/store/kv.go internal/store/kv_test.go
git commit -m "feat(store): notifications queue + config_kv"
```

---

# Phase 2 — Active Probe

## Task 2.1: Config struct (env + yaml)

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `proxywatch.example.yaml`

- [ ] **Step 1: Add yaml dependency**

```bash
go get gopkg.in/yaml.v3@latest
```

- [ ] **Step 2: Write failing test**

`internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.yaml")
	yaml := `
listen: ":18318"
data_dir: "/data"
cpa_proxy_url: "socks5h://u:p@host:1111"
cpa_log_dir: "/cpa-logs"
active_probe:
  target: "https://api.openai.com/v1/models"
  interval_seconds: 60
  timeout_seconds: 15
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROXYWATCH_KEY", "secret")

	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Listen != ":18318" {
		t.Errorf("Listen=%q, want :18318", c.Listen)
	}
	if c.AuthKey != "secret" {
		t.Errorf("AuthKey=%q, want secret", c.AuthKey)
	}
	if c.ActiveProbe.IntervalSeconds != 60 {
		t.Errorf("IntervalSeconds=%d, want 60", c.ActiveProbe.IntervalSeconds)
	}
}

func TestLoadRejectsMissingAuthKey(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.yaml")
	os.WriteFile(p, []byte("listen: \":18318\"\ndata_dir: \"/d\""), 0o644)
	os.Unsetenv("PROXYWATCH_KEY")

	_, err := Load(p)
	if err == nil {
		t.Error("expected error for missing PROXYWATCH_KEY, got nil")
	}
}
```

- [ ] **Step 3: Implement**

`internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen      string `yaml:"listen"`
	DataDir     string `yaml:"data_dir"`
	CPAProxyURL string `yaml:"cpa_proxy_url"`
	CPALogDir   string `yaml:"cpa_log_dir"`

	ActiveProbe ActiveProbeConfig `yaml:"active_probe"`

	// AuthKey comes from env, not yaml
	AuthKey string `yaml:"-"`
}

type ActiveProbeConfig struct {
	Target          string `yaml:"target"`
	IntervalSeconds int    `yaml:"interval_seconds"`
	TimeoutSeconds  int    `yaml:"timeout_seconds"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	c := &Config{
		Listen:  ":18318",
		DataDir: "/data",
		ActiveProbe: ActiveProbeConfig{
			Target:          "https://api.openai.com/v1/models",
			IntervalSeconds: 60,
			TimeoutSeconds:  15,
		},
	}
	if err := yaml.Unmarshal(raw, c); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	c.AuthKey = os.Getenv("PROXYWATCH_KEY")
	if c.AuthKey == "" {
		return nil, fmt.Errorf("PROXYWATCH_KEY environment variable is required")
	}
	return c, nil
}
```

- [ ] **Step 4: Run tests, expect pass**

```bash
go test ./internal/config/ -v
```

- [ ] **Step 5: Add proxywatch.example.yaml**

```yaml
# proxywatch.example.yaml
listen: ":18318"
data_dir: "/data"

# CPA's SOCKS5 upstream proxy URL (same value as in CPA's config.yaml)
cpa_proxy_url: "socks5h://USERNAME:PASSWORD@us.miyaip.online:1111"

# Path to CPA's log directory, mounted read-only into proxywatch
cpa_log_dir: "/cpa-logs"

active_probe:
  target: "https://api.openai.com/v1/models"
  interval_seconds: 60
  timeout_seconds: 15

# PROXYWATCH_KEY must be set as an environment variable, not in this file.
# Generate with: openssl rand -hex 32
```

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config/ proxywatch.example.yaml
git commit -m "feat(config): yaml + env loader with PROXYWATCH_KEY guard"
```

---

## Task 2.2: Active probe — SOCKS5 HTTP client

**Files:**
- Create: `internal/prober/active.go`
- Create: `internal/prober/active_test.go`

- [ ] **Step 1: Add SOCKS5 dependency**

```bash
go get golang.org/x/net/proxy
```

- [ ] **Step 2: Write failing test using a mock SOCKS5 server**

For the test we'll bypass SOCKS5 by passing a custom `http.Client` (the production code accepts a Client to make this testable). The test verifies the result-shaping logic (200 → ok, 4xx → not ok, latency captured, error captured on timeout).

`internal/prober/active_test.go`:

```go
package prober

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestActiveProbeSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	p := &ActiveProber{
		Target:  srv.URL,
		Timeout: 2 * time.Second,
		Client:  srv.Client(),
		IPLookup: func() (string, error) { return "1.2.3.4", nil },
	}
	r := p.Run()
	if !r.OK || r.HTTPCode != 200 {
		t.Errorf("expected ok+200, got %+v", r)
	}
	if r.ExitIP != "1.2.3.4" {
		t.Errorf("ExitIP=%q, want 1.2.3.4", r.ExitIP)
	}
	if r.LatencyMS == 0 {
		t.Error("LatencyMS should be > 0")
	}
}

func TestActiveProbe403IsNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer srv.Close()

	p := &ActiveProber{
		Target:   srv.URL,
		Timeout:  2 * time.Second,
		Client:   srv.Client(),
		IPLookup: func() (string, error) { return "", nil },
	}
	r := p.Run()
	if r.OK {
		t.Error("403 should not be OK")
	}
	if r.HTTPCode != 403 {
		t.Errorf("HTTPCode=%d, want 403", r.HTTPCode)
	}
}

func TestActiveProbeNetworkError(t *testing.T) {
	p := &ActiveProber{
		Target:   "http://127.0.0.1:1", // refused
		Timeout:  500 * time.Millisecond,
		Client:   &http.Client{Timeout: 500 * time.Millisecond},
		IPLookup: func() (string, error) { return "", nil },
	}
	r := p.Run()
	if r.OK {
		t.Error("connection refused should not be OK")
	}
	if r.RawError == "" {
		t.Error("RawError should be populated on network failure")
	}
}
```

- [ ] **Step 3: Implement ActiveProber**

`internal/prober/active.go`:

```go
package prober

import (
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// Result is the output of a single probe run.
type Result struct {
	TS        time.Time
	Target    string
	HTTPCode  int
	LatencyMS int
	ExitIP    string
	OK        bool // 200 ≤ code < 400 AND no transport error
	RawError  string
}

type ActiveProber struct {
	Target   string
	Timeout  time.Duration
	Client   *http.Client
	IPLookup func() (string, error)
}

func (p *ActiveProber) Run() Result {
	start := time.Now()
	r := Result{TS: start, Target: p.Target}

	req, err := http.NewRequest("GET", p.Target, nil)
	if err != nil {
		r.RawError = err.Error()
		return r
	}
	resp, err := p.Client.Do(req)
	r.LatencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		r.RawError = err.Error()
	} else {
		resp.Body.Close()
		r.HTTPCode = resp.StatusCode
		r.OK = resp.StatusCode >= 200 && resp.StatusCode < 400
	}

	if p.IPLookup != nil {
		ip, ipErr := p.IPLookup()
		if ipErr == nil {
			r.ExitIP = ip
		}
	}
	return r
}

// NewSOCKS5Client builds an http.Client whose transport routes through the SOCKS5 URL.
func NewSOCKS5Client(socksURL string, timeout time.Duration) (*http.Client, error) {
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
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Dial: dialer.Dial,
		},
	}, nil
}
```

- [ ] **Step 4: Run tests, expect pass**

```bash
go test ./internal/prober/ -v -race
```

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/prober/active.go internal/prober/active_test.go
git commit -m "feat(prober): active probe with SOCKS5 client + result shaping"
```

---

## Task 2.3: ipify lookup with fallbacks

**Files:**
- Create: `internal/prober/iplookup.go`
- Create: `internal/prober/iplookup_test.go`

- [ ] **Step 1: Write failing test**

`internal/prober/iplookup_test.go`:

```go
package prober

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIPLookupTriesAllInOrder(t *testing.T) {
	calls := []string{}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "bad")
		w.WriteHeader(500)
	}))
	defer bad.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "good")
		w.Write([]byte("8.8.8.8"))
	}))
	defer good.Close()

	lookup := &IPLookup{
		Endpoints: []string{bad.URL, good.URL},
		Client:    bad.Client(),
		Timeout:   2 * time.Second,
	}
	ip, err := lookup.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if strings.TrimSpace(ip) != "8.8.8.8" {
		t.Errorf("ip=%q, want 8.8.8.8", ip)
	}
	if len(calls) != 2 || calls[0] != "bad" || calls[1] != "good" {
		t.Errorf("call order = %v, want [bad good]", calls)
	}
}

func TestIPLookupAllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	lookup := &IPLookup{
		Endpoints: []string{srv.URL, srv.URL},
		Client:    srv.Client(),
		Timeout:   2 * time.Second,
	}
	if _, err := lookup.Get(); err == nil {
		t.Error("expected error when all endpoints fail")
	}
}
```

- [ ] **Step 2: Implement**

`internal/prober/iplookup.go`:

```go
package prober

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type IPLookup struct {
	Endpoints []string
	Client    *http.Client
	Timeout   time.Duration
}

// DefaultIPLookupEndpoints — used when none are configured.
// All return the IP as plain text.
var DefaultIPLookupEndpoints = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://api.myip.com",
}

func (l *IPLookup) Get() (string, error) {
	var lastErr error
	for _, ep := range l.Endpoints {
		req, err := http.NewRequest("GET", ep, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := l.Client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("%s: HTTP %d", ep, resp.StatusCode)
			continue
		}
		ip := strings.TrimSpace(string(body))
		// api.myip.com returns JSON {"ip":"...",...}; handle that
		if strings.HasPrefix(ip, "{") {
			// crude extract — look for "ip":"..."
			if idx := strings.Index(ip, `"ip":"`); idx >= 0 {
				rest := ip[idx+6:]
				if end := strings.Index(rest, `"`); end > 0 {
					ip = rest[:end]
				}
			}
		}
		if ip == "" {
			lastErr = fmt.Errorf("%s: empty body", ep)
			continue
		}
		return ip, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no endpoints configured")
	}
	return "", fmt.Errorf("all ip lookups failed: %w", lastErr)
}
```

- [ ] **Step 3: Run tests, expect pass**

```bash
go test ./internal/prober/ -v -race
```

- [ ] **Step 4: Commit**

```bash
git add internal/prober/iplookup.go internal/prober/iplookup_test.go
git commit -m "feat(prober): IP lookup with fallback endpoints"
```

---

## Task 2.4: Wire active prober into main loop and persist results

**Files:**
- Modify: `cmd/proxywatch/main.go`
- Create: `internal/prober/loop.go`
- Create: `internal/prober/loop_test.go`

- [ ] **Step 1: Write failing test for `RunOnce` recording to store**

`internal/prober/loop_test.go`:

```go
package prober

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tripplemay/proxywatch/internal/store"
)

func newStoreT(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRunOnceWritesProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := newStoreT(t)
	p := &ActiveProber{
		Target:   srv.URL,
		Timeout:  2 * time.Second,
		Client:   srv.Client(),
		IPLookup: func() (string, error) { return "5.6.7.8", nil },
	}

	if err := RunOnce(s, p); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	rows, err := s.RecentProbes(10, "active")
	if err != nil {
		t.Fatalf("RecentProbes: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len=%d, want 1", len(rows))
	}
	if rows[0].HTTPCode != 200 || rows[0].ExitIP != "5.6.7.8" {
		t.Errorf("got %+v", rows[0])
	}
}
```

- [ ] **Step 2: Implement `RunOnce` and `Loop`**

`internal/prober/loop.go`:

```go
package prober

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tripplemay/proxywatch/internal/store"
)

// RunOnce executes a single active probe and persists the result.
func RunOnce(s *store.Store, p *ActiveProber) error {
	r := p.Run()
	_, err := s.InsertProbe(store.Probe{
		TS:        r.TS,
		Kind:      "active",
		Target:    r.Target,
		HTTPCode:  r.HTTPCode,
		LatencyMS: r.LatencyMS,
		ExitIP:    r.ExitIP,
		OK:        r.OK,
		RawError:  r.RawError,
	})
	if err != nil {
		return fmt.Errorf("persist probe: %w", err)
	}
	return nil
}

// Loop runs RunOnce on a ticker until ctx is cancelled.
// Interval is read fresh from getInterval each tick to allow live config changes.
func Loop(ctx context.Context, s *store.Store, p *ActiveProber, getInterval func() time.Duration, log *slog.Logger) {
	for {
		if err := RunOnce(s, p); err != nil {
			log.Error("active probe failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(getInterval()):
		}
	}
}
```

- [ ] **Step 3: Run test, expect pass**

```bash
go test ./internal/prober/ -v -race
```

- [ ] **Step 4: Update main to start the loop**

`cmd/proxywatch/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tripplemay/proxywatch/internal/config"
	"github.com/tripplemay/proxywatch/internal/prober"
	"github.com/tripplemay/proxywatch/internal/store"
)

const version = "0.1.0-dev"

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "/etc/proxywatch.yaml", "config file path")
	flag.Parse()

	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(cfg.DataDir, "proxywatch.sqlite")
	s, err := store.Open(dbPath)
	if err != nil {
		log.Error("open store", "err", err)
		os.Exit(1)
	}
	defer s.Close()

	socksClient, err := prober.NewSOCKS5Client(cfg.CPAProxyURL, time.Duration(cfg.ActiveProbe.TimeoutSeconds)*time.Second)
	if err != nil {
		log.Error("build socks5 client", "err", err)
		os.Exit(1)
	}

	ipLookup := &prober.IPLookup{
		Endpoints: prober.DefaultIPLookupEndpoints,
		Client:    &http.Client{Timeout: 5 * time.Second},
		Timeout:   5 * time.Second,
	}
	probe := &prober.ActiveProber{
		Target:   cfg.ActiveProbe.Target,
		Timeout:  time.Duration(cfg.ActiveProbe.TimeoutSeconds) * time.Second,
		Client:   socksClient,
		IPLookup: func() (string, error) { return ipLookup.Get() },
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	getInterval := func() time.Duration {
		return time.Duration(cfg.ActiveProbe.IntervalSeconds) * time.Second
	}

	log.Info("proxywatch starting", "version", version, "listen", cfg.Listen)
	go prober.Loop(ctx, s, probe, getInterval, log)

	// HTTP server stub — real handlers added in Phase 3.
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})
	srv := &http.Server{Addr: cfg.Listen, Handler: mux}
	go func() { _ = srv.ListenAndServe() }()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	srv.Shutdown(shutdownCtx)
}
```

- [ ] **Step 5: Build and smoke-test**

```bash
go build -o /tmp/proxywatch ./cmd/proxywatch

# Create a minimal config + run for 90s with low interval
cat > /tmp/p.yaml <<'EOF'
listen: ":18318"
data_dir: "/tmp"
cpa_proxy_url: "socks5h://noauth@127.0.0.1:1"
cpa_log_dir: "/tmp"
active_probe:
  target: "https://example.com"
  interval_seconds: 5
  timeout_seconds: 5
EOF

PROXYWATCH_KEY=test /tmp/proxywatch -config /tmp/p.yaml &
PID=$!
sleep 12
kill $PID
sqlite3 /tmp/proxywatch.sqlite "SELECT count(*), kind FROM probes GROUP BY kind"
```

Expected: at least 2 probe rows in `active` kind. (They will likely all be `ok=0` because the SOCKS5 dial to 127.0.0.1:1 fails — that's fine, we're testing the loop wiring, not the proxy.)

- [ ] **Step 6: Commit**

```bash
git add cmd/proxywatch/main.go internal/prober/loop.go internal/prober/loop_test.go
git commit -m "feat(prober): periodic active probe loop wired into main"
```

---

# Phase 3 — HTTP API + Minimal Frontend

## Task 3.1: Bearer auth middleware

**Files:**
- Create: `internal/api/auth.go`
- Create: `internal/api/auth_test.go`

- [ ] **Step 1: Write failing test**

`internal/api/auth_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuthRejectsMissingHeader(t *testing.T) {
	h := BearerAuth("k", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 401 {
		t.Errorf("code=%d, want 401", rec.Code)
	}
}

func TestBearerAuthRejectsWrongKey(t *testing.T) {
	h := BearerAuth("right", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	h.ServeHTTP(rec, r)
	if rec.Code != 401 {
		t.Errorf("code=%d, want 401", rec.Code)
	}
}

func TestBearerAuthAcceptsCorrectKey(t *testing.T) {
	h := BearerAuth("k", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer k")
	h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Errorf("code=%d, want 200", rec.Code)
	}
}
```

- [ ] **Step 2: Implement**

`internal/api/auth.go`:

```go
package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func BearerAuth(key string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, "missing Bearer token", http.StatusUnauthorized)
			return
		}
		got := strings.TrimPrefix(h, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 3: Run tests, expect pass**

```bash
go test ./internal/api/ -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go
git commit -m "feat(api): bearer auth middleware with constant-time compare"
```

---

## Task 3.2: /api/status endpoint

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/handlers.go`
- Create: `internal/api/handlers_test.go`

- [ ] **Step 1: Write failing test for /api/status**

`internal/api/handlers_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tripplemay/proxywatch/internal/store"
)

func newStoreT(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStatusHandlerEmptyStore(t *testing.T) {
	s := newStoreT(t)
	srv := NewServer(s, "k", "0.1.0")
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/status", nil)
	r.Header.Set("Authorization", "Bearer k")
	srv.Handler().ServeHTTP(rec, r)

	if rec.Code != 200 {
		t.Fatalf("code=%d, want 200", rec.Code)
	}
	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["state"] != "HEALTHY" {
		t.Errorf("state=%v, want HEALTHY", got["state"])
	}
	if got["last_active_probe"] != nil {
		t.Errorf("expected nil last_active_probe, got %v", got["last_active_probe"])
	}
}

func TestStatusHandlerWithProbe(t *testing.T) {
	s := newStoreT(t)
	s.InsertProbe(store.Probe{
		TS: time.Now(), Kind: "active", HTTPCode: 200, OK: true, ExitIP: "1.1.1.1",
	})

	srv := NewServer(s, "k", "0.1.0")
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/status", nil)
	r.Header.Set("Authorization", "Bearer k")
	srv.Handler().ServeHTTP(rec, r)

	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var got map[string]any
	json.NewDecoder(rec.Body).Decode(&got)
	if got["exit_ip"] != "1.1.1.1" {
		t.Errorf("exit_ip=%v, want 1.1.1.1", got["exit_ip"])
	}
}
```

- [ ] **Step 2: Implement Server + handler**

`internal/api/server.go`:

```go
package api

import (
	"net/http"

	"github.com/tripplemay/proxywatch/internal/store"
)

type Server struct {
	store   *store.Store
	authKey string
	version string
}

func NewServer(s *store.Store, authKey, version string) *Server {
	return &Server{store: s, authKey: authKey, version: version}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)

	api := http.NewServeMux()
	api.HandleFunc("/api/status", s.handleStatus)

	mux.Handle("/api/", BearerAuth(s.authKey, api))
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("ok"))
}
```

`internal/api/handlers.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
)

type statusResponse struct {
	Version         string  `json:"version"`
	State           string  `json:"state"`
	ExitIP          string  `json:"exit_ip,omitempty"`
	LastActiveProbe *probeJSON `json:"last_active_probe,omitempty"`
}

type probeJSON struct {
	TSMS      int64  `json:"ts_ms"`
	HTTPCode  int    `json:"http_code"`
	LatencyMS int    `json:"latency_ms"`
	ExitIP    string `json:"exit_ip,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := statusResponse{Version: s.version, State: "HEALTHY"}

	rows, err := s.store.RecentProbes(1, "active")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if len(rows) > 0 {
		p := rows[0]
		resp.LastActiveProbe = &probeJSON{
			TSMS:      p.TS.UnixMilli(),
			HTTPCode:  p.HTTPCode,
			LatencyMS: p.LatencyMS,
			ExitIP:    p.ExitIP,
			OK:        p.OK,
			Error:     p.RawError,
		}
		resp.ExitIP = p.ExitIP
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 3: Run tests, expect pass**

```bash
go test ./internal/api/ -v -race
```

- [ ] **Step 4: Wire server into main**

Modify `cmd/proxywatch/main.go` — replace the inline `mux` block with:

```go
apiSrv := api.NewServer(s, cfg.AuthKey, version)
srv := &http.Server{Addr: cfg.Listen, Handler: apiSrv.Handler()}
```

Add the import: `"github.com/tripplemay/proxywatch/internal/api"`.

- [ ] **Step 5: Build and smoke-test**

```bash
go build -o /tmp/proxywatch ./cmd/proxywatch
PROXYWATCH_KEY=test /tmp/proxywatch -config /tmp/p.yaml &
PID=$!
sleep 8
curl -s -H "Authorization: Bearer test" http://127.0.0.1:18318/api/status
echo
kill $PID
```

Expected: JSON with `"state":"HEALTHY"` and a `last_active_probe` field.

- [ ] **Step 6: Commit**

```bash
git add internal/api/server.go internal/api/handlers.go internal/api/handlers_test.go cmd/proxywatch/main.go
git commit -m "feat(api): /api/status endpoint with auth"
```

---

## Task 3.3: Set up frontend (React + Vite + TS)

**Files:**
- Create: `web/package.json`, `web/tsconfig.json`, `web/vite.config.ts`, `web/index.html`
- Create: `web/src/main.tsx`, `web/src/App.tsx`, `web/src/api.ts`, `web/src/styles.css`

- [ ] **Step 1: Initialize**

```bash
mkdir -p web && cd web
npm create vite@latest . -- --template react-ts
# answer "Ignore files and continue" if prompted (since the dir has package.json template etc)
npm install
cd ..
```

- [ ] **Step 2: Replace `web/vite.config.ts` to emit to `dist`**

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: { outDir: 'dist', emptyOutDir: true },
})
```

- [ ] **Step 3: Replace `web/src/App.tsx` with minimal status UI**

```tsx
import { useEffect, useState } from 'react'
import { fetchStatus, getKey, setKey, Status } from './api'
import './styles.css'

export default function App() {
  const [keyInput, setKeyInput] = useState(getKey())
  const [status, setStatus] = useState<Status | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!getKey()) return
    const refresh = async () => {
      try {
        setStatus(await fetchStatus())
        setError(null)
      } catch (e) {
        setError(String(e))
      }
    }
    refresh()
    const t = setInterval(refresh, 5000)
    return () => clearInterval(t)
  }, [keyInput])

  if (!getKey()) {
    return (
      <div className="login">
        <h1>proxywatch</h1>
        <p>Enter your PROXYWATCH_KEY to continue.</p>
        <input value={keyInput} onChange={(e) => setKeyInput(e.target.value)} type="password" />
        <button onClick={() => { setKey(keyInput); setKeyInput(keyInput) }}>Save</button>
      </div>
    )
  }

  return (
    <div className="app">
      <header>
        <h1>proxywatch</h1>
        <span className="version">v{status?.version || '?'}</span>
      </header>
      {error && <div className="error">{error}</div>}
      <section className="status-card">
        <h2>State</h2>
        <div className={`state state-${status?.state || 'unknown'}`}>{status?.state || '...'}</div>
        <div className="exit-ip">Exit IP: {status?.exit_ip || '(unknown)'}</div>
      </section>
      <section className="probe-card">
        <h2>Last active probe</h2>
        {status?.last_active_probe ? (
          <ul>
            <li>HTTP: {status.last_active_probe.http_code} ({status.last_active_probe.ok ? 'OK' : 'FAIL'})</li>
            <li>Latency: {status.last_active_probe.latency_ms} ms</li>
            <li>Time: {new Date(status.last_active_probe.ts_ms).toLocaleTimeString()}</li>
          </ul>
        ) : <p>No probes yet</p>}
      </section>
      <footer>
        <button onClick={() => { localStorage.removeItem('proxywatch.key'); location.reload() }}>Logout</button>
      </footer>
    </div>
  )
}
```

`web/src/api.ts`:

```ts
const KEY_STORAGE = 'proxywatch.key'

export type Status = {
  version: string
  state: string
  exit_ip?: string
  last_active_probe?: {
    ts_ms: number
    http_code: number
    latency_ms: number
    exit_ip?: string
    ok: boolean
    error?: string
  }
}

export const getKey = () => localStorage.getItem(KEY_STORAGE) || ''
export const setKey = (k: string) => localStorage.setItem(KEY_STORAGE, k)

async function authedFetch(path: string, init: RequestInit = {}) {
  const r = await fetch(path, {
    ...init,
    headers: { ...(init.headers || {}), Authorization: `Bearer ${getKey()}` },
  })
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`)
  return r.json()
}

export const fetchStatus = (): Promise<Status> => authedFetch('/api/status')
```

`web/src/styles.css`:

```css
* { box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 0; background: #0e1116; color: #e5e7eb; }
.login, .app { max-width: 720px; margin: 4rem auto; padding: 2rem; }
.login input { padding: 0.5rem; width: 100%; margin: 1rem 0; background: #1f2937; color: white; border: 1px solid #374151; border-radius: 4px; }
.login button, footer button { padding: 0.5rem 1rem; background: #374151; color: white; border: 1px solid #4b5563; border-radius: 4px; cursor: pointer; }
header { display: flex; align-items: baseline; gap: 1rem; margin-bottom: 2rem; }
.version { color: #9ca3af; font-size: 0.875rem; }
section { background: #1f2937; padding: 1.5rem; border-radius: 8px; margin-bottom: 1rem; }
.state { display: inline-block; padding: 0.25rem 0.75rem; border-radius: 4px; font-weight: 600; }
.state-HEALTHY { background: #065f46; color: #d1fae5; }
.state-SUSPECT, .state-VERIFYING { background: #92400e; color: #fef3c7; }
.state-ROTATING, .state-ALERT_ONLY { background: #991b1b; color: #fee2e2; }
.exit-ip { margin-top: 0.5rem; color: #9ca3af; font-family: monospace; }
.error { background: #991b1b; color: white; padding: 0.5rem 1rem; border-radius: 4px; margin-bottom: 1rem; }
ul { margin: 0; padding-left: 1.5rem; }
footer { margin-top: 2rem; }
```

- [ ] **Step 4: Build the frontend**

```bash
cd web
npm run build
ls dist/
cd ..
```

Expected: `web/dist/index.html` and `web/dist/assets/*.js,*.css` present.

- [ ] **Step 5: Update .gitignore for web/**

Append to `.gitignore`:

```
web/node_modules/
web/dist/
```

(Already present per Phase 0; verify.)

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/package-lock.json web/tsconfig.json web/tsconfig.node.json web/vite.config.ts web/index.html web/src/ .gitignore
git commit -m "feat(web): initial React+TS frontend with /api/status fetch"
```

---

## Task 3.4: Embed the frontend into the Go binary

**Files:**
- Create: `internal/web/embed.go`
- Modify: `internal/api/server.go`
- Modify: `Dockerfile`

- [ ] **Step 1: Add embed file**

`internal/web/embed.go`:

```go
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded built frontend rooted at dist/.
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // should never happen — embed is build-time
	}
	return sub
}
```

- [ ] **Step 2: Symlink or copy `web/dist` into the embed dir**

Approach: a Makefile target builds the frontend and copies its output into `internal/web/dist`. The committed source lives in `web/`; the build artifact is regenerated in `internal/web/dist/`.

Append to `Makefile`:

```makefile
web-build:
	cd web && npm install && npm run build
	rm -rf internal/web/dist
	cp -r web/dist internal/web/dist

build-all: web-build build
```

Add to `.gitignore`:

```
internal/web/dist/
```

Create a placeholder so `go:embed` doesn't fail when the dist isn't present at test time:

```bash
mkdir -p internal/web/dist
touch internal/web/dist/.gitkeep
```

Modify `.gitignore` to NOT ignore the placeholder:

```
internal/web/dist/*
!internal/web/dist/.gitkeep
```

- [ ] **Step 3: Mount the embedded FS in the API server**

Replace `Handler()` in `internal/api/server.go`:

```go
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)

	api := http.NewServeMux()
	api.HandleFunc("/api/status", s.handleStatus)
	mux.Handle("/api/", BearerAuth(s.authKey, api))

	mux.Handle("/", http.FileServer(http.FS(s.staticFS)))
	return mux
}
```

Add to the struct:

```go
import "io/fs"
// ...
type Server struct {
	store    *store.Store
	authKey  string
	version  string
	staticFS fs.FS
}
```

Add a setter:

```go
func (s *Server) WithStatic(fsys fs.FS) *Server {
	s.staticFS = fsys
	return s
}
```

- [ ] **Step 4: Wire embed in main**

In `cmd/proxywatch/main.go`, add:

```go
import "github.com/tripplemay/proxywatch/internal/web"
// ...
apiSrv := api.NewServer(s, cfg.AuthKey, version).WithStatic(web.FS())
```

- [ ] **Step 5: Build and smoke-test in browser**

```bash
make web-build
make build
PROXYWATCH_KEY=test ./bin/proxywatch -config /tmp/p.yaml &
sleep 5
curl -s http://127.0.0.1:18318/ | head -5    # should return index.html
kill %1
```

Expected: HTML output starting with `<!DOCTYPE html>` containing the React mount div.

Open `http://127.0.0.1:18318/` in a browser (after copying out of WSL or via SSH tunnel) — should see the proxywatch UI.

- [ ] **Step 6: Update Dockerfile to build the frontend**

Replace Dockerfile with:

```dockerfile
# syntax=docker/dockerfile:1.6
FROM node:20-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /web/dist /src/internal/web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/proxywatch ./cmd/proxywatch

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/proxywatch /app/proxywatch
ENTRYPOINT ["/app/proxywatch"]
```

- [ ] **Step 7: Verify Docker build**

```bash
docker build -t proxywatch:dev .
docker run --rm proxywatch:dev version
```

- [ ] **Step 8: Commit**

```bash
git add internal/web/ internal/api/server.go cmd/proxywatch/main.go Makefile Dockerfile .gitignore
git commit -m "feat: embed built frontend into Go binary"
```

---

# Phase 4 — Telegram Notifier

## Task 4.1: Telegram client

**Files:**
- Create: `internal/notifier/telegram.go`
- Create: `internal/notifier/telegram_test.go`

- [ ] **Step 1: Write failing test using a mock telegram server**

`internal/notifier/telegram_test.go`:

```go
package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegramSendSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/bot") {
			t.Errorf("path=%s, want /bot...", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		if payload["chat_id"] != "12345" {
			t.Errorf("chat_id=%v, want 12345", payload["chat_id"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Telegram{
		Token:   "tok",
		ChatID:  "12345",
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
	}
	if err := c.Send("hello"); err != nil {
		t.Errorf("Send: %v", err)
	}
}

func TestTelegramSendApiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
	}))
	defer srv.Close()

	c := &Telegram{Token: "tok", ChatID: "x", BaseURL: srv.URL, HTTP: srv.Client()}
	if err := c.Send("x"); err == nil {
		t.Error("expected error from API ok=false response")
	}
}
```

- [ ] **Step 2: Implement**

`internal/notifier/telegram.go`:

```go
package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Telegram struct {
	Token   string
	ChatID  string
	BaseURL string // e.g. https://api.telegram.org; used for testability
	HTTP    *http.Client
}

func NewTelegram(token, chatID string, http *http.Client) *Telegram {
	return &Telegram{
		Token:   token,
		ChatID:  chatID,
		BaseURL: "https://api.telegram.org",
		HTTP:    http,
	}
}

type sendMessageBody struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type sendMessageResp struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func (t *Telegram) Send(text string) error {
	if t.Token == "" || t.ChatID == "" {
		return fmt.Errorf("telegram: token or chat_id not configured")
	}
	body, _ := json.Marshal(sendMessageBody{ChatID: t.ChatID, Text: text})
	url := fmt.Sprintf("%s/bot%s/sendMessage", t.BaseURL, t.Token)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("telegram http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var r sendMessageResp
	if err := json.Unmarshal(respBody, &r); err != nil {
		return fmt.Errorf("telegram parse: %w", err)
	}
	if !r.OK {
		return fmt.Errorf("telegram api: %s", r.Description)
	}
	return nil
}
```

- [ ] **Step 3: Run tests, expect pass**

```bash
go test ./internal/notifier/ -v -race
```

- [ ] **Step 4: Commit**

```bash
git add internal/notifier/telegram.go internal/notifier/telegram_test.go
git commit -m "feat(notifier): telegram client with mock-friendly BaseURL"
```

---

## Task 4.2: Notifier queue + retry loop

**Files:**
- Create: `internal/notifier/queue.go`
- Create: `internal/notifier/queue_test.go`

- [ ] **Step 1: Write failing test**

`internal/notifier/queue_test.go`:

```go
package notifier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tripplemay/proxywatch/internal/store"
)

func newStoreT(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestQueueDrainSendsAndMarksSent(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s := newStoreT(t)
	tg := &Telegram{Token: "t", ChatID: "c", BaseURL: srv.URL, HTTP: srv.Client()}
	q := &Queue{Store: s, Telegram: tg}

	id, _ := s.EnqueueNotification(store.Notification{TS: time.Now(), Level: "info", Text: "hi"})
	if err := q.DrainOnce(context.Background()); err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("hits=%d, want 1", hits)
	}
	pending, _ := s.PendingNotifications(10)
	if len(pending) != 0 {
		t.Errorf("pending len=%d, want 0; id=%d still listed", len(pending), id)
	}
}

func TestQueueDrainRecordsFailureAndContinues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	s := newStoreT(t)
	tg := &Telegram{Token: "t", ChatID: "c", BaseURL: srv.URL, HTTP: srv.Client()}
	q := &Queue{Store: s, Telegram: tg, MaxRetries: 5}

	s.EnqueueNotification(store.Notification{TS: time.Now(), Level: "info", Text: "1"})
	if err := q.DrainOnce(context.Background()); err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
	pending, _ := s.PendingNotifications(10)
	if len(pending) != 1 || pending[0].RetryCount != 1 {
		t.Errorf("after failure, retry_count=%d, len=%d", pending[0].RetryCount, len(pending))
	}
}
```

- [ ] **Step 2: Implement**

`internal/notifier/queue.go`:

```go
package notifier

import (
	"context"
	"log/slog"
	"time"

	"github.com/tripplemay/proxywatch/internal/store"
)

type Queue struct {
	Store      *store.Store
	Telegram   *Telegram
	MaxRetries int // default 10; after this, the entry stops being retried
}

func (q *Queue) maxRetries() int {
	if q.MaxRetries <= 0 {
		return 10
	}
	return q.MaxRetries
}

// DrainOnce drains the pending queue once. Each entry is attempted; failures are recorded.
func (q *Queue) DrainOnce(ctx context.Context) error {
	pending, err := q.Store.PendingNotifications(50)
	if err != nil {
		return err
	}
	for _, n := range pending {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if n.RetryCount >= q.maxRetries() {
			continue
		}
		if err := q.Telegram.Send(n.Text); err != nil {
			_ = q.Store.RecordNotificationFailure(n.ID, err.Error())
			continue
		}
		_ = q.Store.MarkNotificationSent(n.ID, time.Now())
	}
	return nil
}

// Loop drains the queue at interval until ctx is cancelled.
func (q *Queue) Loop(ctx context.Context, interval time.Duration, log *slog.Logger) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if err := q.DrainOnce(ctx); err != nil && ctx.Err() == nil {
			log.Error("notifier drain", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
```

- [ ] **Step 3: Run tests, expect pass**

```bash
go test ./internal/notifier/ -v -race
```

- [ ] **Step 4: Commit**

```bash
git add internal/notifier/queue.go internal/notifier/queue_test.go
git commit -m "feat(notifier): queue draining with retry-count tracking"
```

---

## Task 4.3: /api/test-notify + wire notifier into main

**Files:**
- Modify: `internal/api/handlers.go`
- Modify: `internal/api/server.go`
- Modify: `cmd/proxywatch/main.go`
- Modify: `internal/config/config.go`
- Modify: `internal/api/handlers_test.go`

- [ ] **Step 1: Add Telegram fields to config**

In `internal/config/config.go`, add to `Config`:

```go
Telegram TelegramConfig `yaml:"telegram"`
```

And:

```go
type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}
```

Update `proxywatch.example.yaml`:

```yaml
telegram:
  bot_token: ""
  chat_id: ""
```

- [ ] **Step 2: Add a test for /api/test-notify**

Append to `internal/api/handlers_test.go`:

```go
func TestTestNotifyEnqueuesMessage(t *testing.T) {
	s := newStoreT(t)
	srv := NewServer(s, "k", "0.1.0")
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/test-notify", nil)
	r.Header.Set("Authorization", "Bearer k")
	srv.Handler().ServeHTTP(rec, r)

	if rec.Code != 202 {
		t.Errorf("code=%d, want 202", rec.Code)
	}
	pending, _ := s.PendingNotifications(10)
	if len(pending) != 1 {
		t.Errorf("pending=%d, want 1", len(pending))
	}
}
```

- [ ] **Step 3: Add handler**

Append to `internal/api/handlers.go`:

```go
import "time"
// (already imported earlier; merge as needed)

func (s *Server) handleTestNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	_, err := s.store.EnqueueNotification(store.Notification{
		TS:    time.Now(),
		Level: "info",
		Text:  "proxywatch test notification — if you see this, telegram works",
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
```

Register it in `Handler()` next to status:

```go
api.HandleFunc("/api/test-notify", s.handleTestNotify)
```

Add the `store` import in `handlers.go`: `"github.com/tripplemay/proxywatch/internal/store"`.

- [ ] **Step 4: Run handler tests, expect pass**

```bash
go test ./internal/api/ -v -race
```

- [ ] **Step 5: Wire notifier in main**

In `cmd/proxywatch/main.go`, after the prober loop start:

```go
if cfg.Telegram.BotToken != "" && cfg.Telegram.ChatID != "" {
	tg := notifier.NewTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, &http.Client{Timeout: 10 * time.Second})
	q := &notifier.Queue{Store: s, Telegram: tg}
	go q.Loop(ctx, 10*time.Second, log)
} else {
	log.Warn("telegram not configured; notifications will queue but not be sent")
}
```

Import: `"github.com/tripplemay/proxywatch/internal/notifier"`.

- [ ] **Step 6: Smoke-test end-to-end**

Generate a real Telegram bot via @BotFather first (out of band), get bot_token and chat_id (use @userinfobot to find your chat_id).

```bash
# Update /tmp/p.yaml with telegram bot_token and chat_id
make web-build && make build
PROXYWATCH_KEY=test ./bin/proxywatch -config /tmp/p.yaml &
sleep 3
curl -X POST -H "Authorization: Bearer test" http://127.0.0.1:18318/api/test-notify
sleep 12
# Check phone for telegram message
kill %1
```

Expected: Telegram message arrives within ~12 seconds.

- [ ] **Step 7: Commit**

```bash
git add internal/config/ proxywatch.example.yaml internal/api/ cmd/proxywatch/main.go
git commit -m "feat: /api/test-notify + wire notifier loop in main"
```

---

# 🎉 MVP COMPLETE

At this point we have:
- Periodic active probe of OpenAI through SOCKS5
- Probes persisted to SQLite
- Web panel showing current state + last probe + exit IP
- Telegram alerts (test endpoint working)
- All deployable as a Docker image

**Recommended cut-point for first deployment.** Ship to staging, observe behavior with real CPA traffic, then continue to Phase 5+.

---

# Phase 5 — Decision Engine + State Machine

## Task 5.1: State enum + sliding window

**Files:**
- Create: `internal/decision/state.go`
- Create: `internal/decision/window.go`
- Create: `internal/decision/window_test.go`

- [ ] **Step 1: State enum**

`internal/decision/state.go`:

```go
package decision

type State string

const (
	StateHealthy   State = "HEALTHY"
	StateSuspect   State = "SUSPECT"
	StateRotating  State = "ROTATING"
	StateVerifying State = "VERIFYING"
	StateCooldown  State = "COOLDOWN"
	StateAlertOnly State = "ALERT_ONLY"
)
```

- [ ] **Step 2: Test for sliding window**

`internal/decision/window_test.go`:

```go
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
```

- [ ] **Step 3: Implement window**

`internal/decision/window.go`:

```go
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
```

- [ ] **Step 4: Run tests, expect pass**

```bash
go test ./internal/decision/ -v -race
```

- [ ] **Step 5: Commit**

```bash
git add internal/decision/state.go internal/decision/window.go internal/decision/window_test.go
git commit -m "feat(decision): state enum + sliding 4xx window"
```

---

## Task 5.2: State machine — transitions

**Files:**
- Create: `internal/decision/machine.go`
- Create: `internal/decision/machine_test.go`

- [ ] **Step 1: Test transitions**

`internal/decision/machine_test.go`:

```go
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
```

- [ ] **Step 2: Implement machine**

`internal/decision/machine.go`:

```go
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

	params  Params
	state   State
	since   time.Time
	window  *Window
	consecutiveActiveFails int

	// rotation state
	rotationCount int
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
```

- [ ] **Step 3: Run tests, expect pass**

```bash
go test ./internal/decision/ -v -race
```

- [ ] **Step 4: Commit**

```bash
git add internal/decision/machine.go internal/decision/machine_test.go
git commit -m "feat(decision): state machine with all spec'd transitions"
```

---

## Task 5.3: Wire machine into prober loop

**Files:**
- Modify: `internal/prober/loop.go`
- Modify: `cmd/proxywatch/main.go`
- Modify: `internal/api/server.go`, `handlers.go`

- [ ] **Step 1: Update prober.Loop signature to accept machine**

Modify `internal/prober/loop.go`:

```go
package prober

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tripplemay/proxywatch/internal/decision"
	"github.com/tripplemay/proxywatch/internal/store"
)

func RunOnce(s *store.Store, p *ActiveProber, m *decision.Machine) error {
	r := p.Run()
	_, err := s.InsertProbe(store.Probe{
		TS:        r.TS,
		Kind:      "active",
		Target:    r.Target,
		HTTPCode:  r.HTTPCode,
		LatencyMS: r.LatencyMS,
		ExitIP:    r.ExitIP,
		OK:        r.OK,
		RawError:  r.RawError,
	})
	if err != nil {
		return fmt.Errorf("persist probe: %w", err)
	}
	if m != nil {
		m.OnActive(r.TS, r.OK)
		m.Tick(r.TS)
	}
	return nil
}

func Loop(ctx context.Context, s *store.Store, p *ActiveProber, m *decision.Machine, getInterval func() time.Duration, log *slog.Logger) {
	for {
		if err := RunOnce(s, p, m); err != nil {
			log.Error("active probe", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(getInterval()):
		}
	}
}
```

- [ ] **Step 2: Update existing prober tests**

Edit `internal/prober/loop_test.go` to pass `nil` for the machine:

```go
if err := RunOnce(s, p, nil); err != nil { ... }
```

- [ ] **Step 3: Update server to expose machine state**

Add field to `Server` in `internal/api/server.go`:

```go
machine *decision.Machine
```

`func (s *Server) WithMachine(m *decision.Machine) *Server { s.machine = m; return s }`

Add the import.

- [ ] **Step 4: Update handleStatus to use machine state**

```go
state := "HEALTHY"
if s.machine != nil {
	state = string(s.machine.State())
}
resp := statusResponse{Version: s.version, State: state}
```

- [ ] **Step 5: Wire in main**

```go
m := decision.NewMachine(decision.Defaults())
go prober.Loop(ctx, s, probe, m, getInterval, log)
apiSrv := api.NewServer(s, cfg.AuthKey, version).WithStatic(web.FS()).WithMachine(m)
```

Add tick goroutine for time-driven transitions:

```go
go func() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Tick(time.Now())
		}
	}
}()
```

- [ ] **Step 6: Run all tests + build**

```bash
go test ./... -v -race
make build
```

- [ ] **Step 7: Commit**

```bash
git add internal/prober/loop.go internal/prober/loop_test.go internal/api/server.go internal/api/handlers.go cmd/proxywatch/main.go
git commit -m "feat: wire decision machine into prober loop and status API"
```

---

# Phase 6 — Passive Log Tail

## Task 6.1: Sample CPA logs to confirm grep pattern

**Files:**
- Create: `internal/prober/passive_format.md`

**Important:** Before coding, sample real CPA logs from the production server (`/opt/cliproxyapi/logs/main.log`) to confirm the exact format of request-completion lines. The implementation will pin to whatever pattern is observed.

- [ ] **Step 1: Pull a sample**

```bash
sshpass -p <root-pw> ssh root@<server-ip> '
  # Grab 200 lines that mention common HTTP codes; redact tokens
  grep -E "(200|400|401|403|429|500|502|503)" /opt/cliproxyapi/logs/main.log | head -200 \
    | sed -E "s/(Bearer )[A-Za-z0-9._-]+/\1<redacted>/g; s/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/<jwt>/g"' \
    > /tmp/cpa-sample.log
wc -l /tmp/cpa-sample.log
head -10 /tmp/cpa-sample.log
```

- [ ] **Step 2: Document the observed grep pattern**

Write findings to `internal/prober/passive_format.md`. Example skeleton (replace with actual observation):

```markdown
# CPA log format (v6.10.9)

Format: `[YYYY-MM-DD HH:MM:SS] [reqid] [level] [file:line] message`

Status code appears in request-completion lines like:

    [2026-05-08 17:42:13] [abc123] [info ] [server.go:230] request_complete method=POST path=/v1/chat/completions status=403 duration=312ms

Regex: `\[([0-9-]+ [0-9:]+)\].*status=(\d{3})`

Edge cases:
- Some lines have `[--------]` instead of req-id
- Streaming responses show interim status lines; we only count `request_complete`
```

- [ ] **Step 3: Commit**

```bash
git add internal/prober/passive_format.md
git commit -m "docs(prober): document observed CPA log line format"
```

---

## Task 6.2: Passive log tail

**Files:**
- Create: `internal/prober/passive.go`
- Create: `internal/prober/passive_test.go`

- [ ] **Step 1: Add fsnotify**

```bash
go get github.com/fsnotify/fsnotify@latest
```

- [ ] **Step 2: Failing test using a temp log file**

`internal/prober/passive_test.go`:

```go
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
		// Per Task 6.1's documented regex (replace if doc updated):
		Pattern: `status=(\d{3})`,
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
	f.WriteString("[2026-05-08 17:00:00] [a] [info ] [s.go:1] request_complete status=200\n")
	f.WriteString("[2026-05-08 17:00:01] [a] [info ] [s.go:1] request_complete status=403\n")
	f.WriteString("[2026-05-08 17:00:02] [a] [info ] [s.go:1] request_complete status=403\n")
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
```

- [ ] **Step 3: Implement PassiveTail**

`internal/prober/passive.go`:

```go
package prober

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/fsnotify/fsnotify"
)

// PassiveTail follows a log file, parses HTTP status codes, and emits them.
type PassiveTail struct {
	Path    string
	Pattern string // regex with one capture group for the status code
	Emit    func(ts time.Time, code int)
	Log     *slog.Logger

	rx *regexp.Regexp
}

func (p *PassiveTail) compile() error {
	rx, err := regexp.Compile(p.Pattern)
	if err != nil {
		return err
	}
	p.rx = rx
	return nil
}

func (p *PassiveTail) Run(ctx context.Context) error {
	if err := p.compile(); err != nil {
		return err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.Add(p.Path); err != nil {
		return err
	}

	f, err := os.Open(p.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	// seek to end
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	br := bufio.NewReader(f)

	readNew := func() {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return
				}
				if p.Log != nil {
					p.Log.Error("passive read", "err", err)
				}
				return
			}
			m := p.rx.FindStringSubmatch(line)
			if len(m) >= 2 {
				code, _ := strconv.Atoi(m[1])
				if p.Emit != nil {
					p.Emit(time.Now(), code)
				}
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&fsnotify.Write == fsnotify.Write {
				readNew()
			}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			if p.Log != nil {
				p.Log.Error("watcher", "err", err)
			}
		}
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/prober/ -v -race -run TestPassiveTail
```

- [ ] **Step 5: Wire into main**

In `cmd/proxywatch/main.go`, after the active-probe loop:

```go
if cfg.CPALogDir != "" {
	pt := &prober.PassiveTail{
		Path:    filepath.Join(cfg.CPALogDir, "main.log"),
		Pattern: `status=(\d{3})`, // confirmed in Task 6.1
		Emit: func(ts time.Time, code int) {
			_, _ = s.InsertProbe(store.Probe{
				TS:       ts,
				Kind:     "passive",
				HTTPCode: code,
				OK:       code < 400,
			})
			m.OnPassive(ts, code)
		},
		Log: log,
	}
	go func() {
		if err := pt.Run(ctx); err != nil {
			log.Error("passive tail", "err", err)
		}
	}()
}
```

Add: `import "path/filepath"` and `"github.com/tripplemay/proxywatch/internal/store"` if not present.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/prober/passive.go internal/prober/passive_test.go cmd/proxywatch/main.go
git commit -m "feat(prober): passive log tail with fsnotify"
```

---

# Phase 7 — Executor (Branch B)

## Task 7.1: Executor — alert + auto-detect IP change

**Files:**
- Create: `internal/executor/executor.go`
- Create: `internal/executor/executor_test.go`

- [ ] **Step 1: Test for state transition triggering alert**

`internal/executor/executor_test.go`:

```go
package executor

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tripplemay/proxywatch/internal/decision"
	"github.com/tripplemay/proxywatch/internal/store"
)

func newStoreT(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestExecutorAlertsOnEnterRotating(t *testing.T) {
	s := newStoreT(t)
	d := decision.Defaults()
	d.SuspectObservation = 50 * time.Millisecond
	m := decision.NewMachine(d)

	var alerts int32
	e := &Executor{
		Store:   s,
		Machine: m,
		Alert: func(text string, level string) {
			atomic.AddInt32(&alerts, 1)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go e.Run(ctx, 20*time.Millisecond)

	now := time.Now()
	for i := 0; i < 3; i++ {
		m.OnActive(now, false)
	}
	m.Tick(now) // → SUSPECT
	time.Sleep(100 * time.Millisecond)
	m.Tick(time.Now()) // → ROTATING (after observation)

	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&alerts) < 1 {
		t.Errorf("expected at least 1 alert, got %d", alerts)
	}
}
```

- [ ] **Step 2: Implement Executor**

`internal/executor/executor.go`:

```go
package executor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tripplemay/proxywatch/internal/decision"
	"github.com/tripplemay/proxywatch/internal/store"
)

// Executor watches the decision machine and enacts side effects on state transitions:
//   - Entering ROTATING: send alert, open incident
//   - IP change observed during ROTATING: notify machine
//   - Entering COOLDOWN: write rotation record + send "recovered" alert
//   - Entering ALERT_ONLY: send "automation paused" alert
type Executor struct {
	Store   *store.Store
	Machine *decision.Machine
	Alert   func(text string, level string)
	Log     *slog.Logger

	prevState     decision.State
	openIncident  int64
	rotationStart time.Time
	rotationOldIP string
}

func (e *Executor) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	e.prevState = decision.StateHealthy
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.tick(time.Now())
		}
	}
}

func (e *Executor) tick(now time.Time) {
	cur := e.Machine.State()
	if cur == e.prevState {
		// observe IP changes if in ROTATING
		if cur == decision.StateRotating {
			if newIP, ok := e.detectIPChange(); ok {
				e.Machine.OnIPChange()
				if e.Log != nil {
					e.Log.Info("auto-detected IP change", "old", e.rotationOldIP, "new", newIP)
				}
			}
		}
		return
	}
	// transition
	switch cur {
	case decision.StateRotating:
		// open incident, snapshot old IP, send alert
		e.openIncident, _ = e.Store.OpenIncident(store.Incident{
			StartedAt:     now,
			TriggerReason: "auto",
			InitialState:  string(cur),
		})
		e.rotationStart = now
		e.rotationOldIP = e.lastExitIP()
		if e.Alert != nil {
			e.Alert(fmt.Sprintf("⚠️ proxy unhealthy\ncurrent IP: %s\nplease rotate at miyaIP backend; proxywatch will auto-detect", e.rotationOldIP), "warning")
		}
	case decision.StateCooldown:
		newIP := e.lastExitIP()
		_, _ = e.Store.InsertRotation(store.Rotation{
			IncidentID:      e.openIncident,
			StartedAt:       e.rotationStart,
			EndedAt:         now,
			OldIP:           e.rotationOldIP,
			NewIP:           newIP,
			DetectionMethod: "auto",
			OK:              true,
		})
		_ = e.Store.CloseIncident(e.openIncident, now, "recovered")
		e.openIncident = 0
		if e.Alert != nil {
			e.Alert(fmt.Sprintf("✅ recovered\nold: %s\nnew: %s", e.rotationOldIP, newIP), "info")
		}
	case decision.StateAlertOnly:
		if e.Alert != nil {
			e.Alert("❌ automation paused — please investigate", "error")
		}
		if e.openIncident != 0 {
			_ = e.Store.CloseIncident(e.openIncident, now, "alert_only")
			e.openIncident = 0
		}
	}
	e.prevState = cur
}

func (e *Executor) lastExitIP() string {
	rows, err := e.Store.RecentProbes(1, "active")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return rows[0].ExitIP
}

func (e *Executor) detectIPChange() (string, bool) {
	cur := e.lastExitIP()
	return cur, cur != "" && cur != e.rotationOldIP
}
```

- [ ] **Step 3: Run tests, expect pass**

```bash
go test ./internal/executor/ -v -race
```

- [ ] **Step 4: Wire executor in main**

```go
exec := &executor.Executor{
	Store:   s,
	Machine: m,
	Alert: func(text, level string) {
		_, _ = s.EnqueueNotification(store.Notification{
			TS: time.Now(), Level: level, Text: text,
		})
	},
	Log: log,
}
go exec.Run(ctx, 5*time.Second)
```

Import `"github.com/tripplemay/proxywatch/internal/executor"`.

- [ ] **Step 5: Commit**

```bash
git add internal/executor/ cmd/proxywatch/main.go
git commit -m "feat(executor): branch-B alert+verify executor"
```

---

## Task 7.2: Manual confirm endpoint

**Files:**
- Modify: `internal/api/server.go`, `handlers.go`, `handlers_test.go`

- [ ] **Step 1: Test**

Append to `internal/api/handlers_test.go`:

```go
import "github.com/tripplemay/proxywatch/internal/decision"

func TestConfirmRotationAdvancesMachine(t *testing.T) {
	s := newStoreT(t)
	m := decision.NewMachine(decision.Defaults())
	srv := NewServer(s, "k", "0.1.0").WithMachine(m)

	// drive machine to SUSPECT
	for i := 0; i < 3; i++ {
		m.OnActive(time.Now(), false)
	}
	m.Tick(time.Now())

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/confirm-rotation", nil)
	r.Header.Set("Authorization", "Bearer k")
	srv.Handler().ServeHTTP(rec, r)

	if rec.Code != 200 {
		t.Errorf("code=%d", rec.Code)
	}
	if m.State() != decision.StateVerifying {
		t.Errorf("state=%s, want VERIFYING", m.State())
	}
}
```

- [ ] **Step 2: Implement**

In `internal/api/handlers.go`:

```go
func (s *Server) handleConfirmRotation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.machine == nil {
		http.Error(w, "machine not configured", 500)
		return
	}
	s.machine.Confirm()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleResumeAutomation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.machine == nil {
		http.Error(w, "machine not configured", 500)
		return
	}
	s.machine.ResumeAutomation()
	w.WriteHeader(http.StatusOK)
}
```

In `internal/api/server.go` Handler():

```go
api.HandleFunc("/api/confirm-rotation", s.handleConfirmRotation)
api.HandleFunc("/api/resume-automation", s.handleResumeAutomation)
```

- [ ] **Step 3: Run, expect pass**

```bash
go test ./internal/api/ -v -race
```

- [ ] **Step 4: Add UI button**

In `web/src/App.tsx`, after the status card, add:

```tsx
{(status?.state === 'ROTATING' || status?.state === 'SUSPECT') && (
  <button className="confirm-btn" onClick={async () => {
    await fetch('/api/confirm-rotation', { method: 'POST', headers: { Authorization: `Bearer ${getKey()}` } })
  }}>I rotated, re-verify</button>
)}

{status?.state === 'ALERT_ONLY' && (
  <button className="resume-btn" onClick={async () => {
    await fetch('/api/resume-automation', { method: 'POST', headers: { Authorization: `Bearer ${getKey()}` } })
  }}>Resume automation</button>
)}
```

In `web/src/styles.css`:

```css
.confirm-btn { background: #1d4ed8; color: white; padding: 0.5rem 1rem; border: 0; border-radius: 4px; cursor: pointer; margin: 1rem 0; }
.resume-btn { background: #b45309; color: white; padding: 0.5rem 1rem; border: 0; border-radius: 4px; cursor: pointer; margin: 1rem 0; }
```

- [ ] **Step 5: Commit**

```bash
make web-build
git add internal/api/ web/src/
git commit -m "feat: manual confirm-rotation + resume-automation endpoints + UI"
```

---

# Phase 8 — Settings panel

## Task 8.1: Settings GET/PUT

**Files:**
- Modify: `internal/api/handlers.go`, `handlers_test.go`

- [ ] **Step 1: Test for GET**

Append to `internal/api/handlers_test.go`:

```go
func TestGetSettingsReturnsDefaults(t *testing.T) {
	s := newStoreT(t)
	srv := NewServer(s, "k", "0.1.0").WithMachine(decision.NewMachine(decision.Defaults()))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/settings", nil)
	r.Header.Set("Authorization", "Bearer k")
	srv.Handler().ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Errorf("code=%d", rec.Code)
	}
	var got map[string]any
	json.NewDecoder(rec.Body).Decode(&got)
	if _, ok := got["passive_threshold"]; !ok {
		t.Error("expected passive_threshold key")
	}
}
```

- [ ] **Step 2: Implement get/put settings**

```go
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}
	keys := []string{
		"active_probe_interval_seconds", "passive_threshold",
		"active_failure_threshold", "suspect_observation_seconds",
		"cooldown_seconds", "telegram_bot_token", "telegram_chat_id",
	}
	for _, k := range keys {
		v, _, _ := s.store.GetKV(k)
		out[k] = v
	}
	// also expose machine defaults as fallback hints
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "PUT only", http.StatusMethodNotAllowed)
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	for k, v := range body {
		if err := s.store.SetKV(k, v); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	w.WriteHeader(200)
}
```

Register in `Handler()`:

```go
api.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSettings(w, r)
	case http.MethodPut:
		s.handlePutSettings(w, r)
	default:
		http.Error(w, "method not allowed", 405)
	}
})
```

- [ ] **Step 3: Run, expect pass**

```bash
go test ./internal/api/ -v
```

- [ ] **Step 4: UI Settings tab**

Add `web/src/components/Settings.tsx`:

```tsx
import { useEffect, useState } from 'react'

export function Settings({ apiKey }: { apiKey: string }) {
  const [vals, setVals] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)
  const fields = [
    'active_probe_interval_seconds',
    'passive_threshold',
    'active_failure_threshold',
    'suspect_observation_seconds',
    'cooldown_seconds',
    'telegram_bot_token',
    'telegram_chat_id',
  ]

  useEffect(() => {
    fetch('/api/settings', { headers: { Authorization: `Bearer ${apiKey}` } })
      .then((r) => r.json())
      .then(setVals)
  }, [apiKey])

  async function save() {
    setSaving(true)
    await fetch('/api/settings', {
      method: 'PUT',
      headers: { Authorization: `Bearer ${apiKey}`, 'Content-Type': 'application/json' },
      body: JSON.stringify(vals),
    })
    setSaving(false)
  }

  return (
    <section className="settings-card">
      <h2>Settings</h2>
      {fields.map((f) => (
        <div key={f} className="settings-row">
          <label>{f}</label>
          <input
            value={vals[f] || ''}
            onChange={(e) => setVals({ ...vals, [f]: e.target.value })}
            type={f.includes('token') ? 'password' : 'text'}
          />
        </div>
      ))}
      <button onClick={save} disabled={saving}>{saving ? 'saving…' : 'Save'}</button>
    </section>
  )
}
```

In `App.tsx`, add `<Settings apiKey={getKey()} />` near the bottom.

- [ ] **Step 5: Commit**

```bash
make web-build
git add internal/api/ web/src/
git commit -m "feat(api,web): /api/settings GET/PUT + UI tab"
```

---

## Task 8.2: Settings actually drive runtime params

**Files:**
- Modify: `cmd/proxywatch/main.go`
- Modify: `internal/store/kv.go`

For MVP we accept lazy refresh: settings are read fresh from KV on every probe loop iteration.

- [ ] **Step 1: Add helper**

`internal/store/kv.go`:

```go
import "strconv"

func (s *Store) GetKVInt(key string, dflt int) int {
	v, ok, err := s.GetKV(key)
	if err != nil || !ok {
		return dflt
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return dflt
	}
	return n
}
```

- [ ] **Step 2: Use in main**

Change `getInterval` in main:

```go
getInterval := func() time.Duration {
	n := s.GetKVInt("active_probe_interval_seconds", cfg.ActiveProbe.IntervalSeconds)
	return time.Duration(n) * time.Second
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/store/kv.go cmd/proxywatch/main.go
git commit -m "feat: settings drive active probe interval at runtime"
```

---

# Phase 9 — History views

## Task 9.1: /api/probes, /api/incidents, /api/rotations

**Files:**
- Modify: `internal/api/handlers.go`, `handlers_test.go`, `server.go`

- [ ] **Step 1: Tests**

Append:

```go
func TestProbesHistoryEndpoint(t *testing.T) {
	s := newStoreT(t)
	for i := 0; i < 3; i++ {
		s.InsertProbe(store.Probe{TS: time.Now(), Kind: "active", OK: true})
	}
	srv := NewServer(s, "k", "0.1.0")
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/probes?limit=10&kind=active", nil)
	r.Header.Set("Authorization", "Bearer k")
	srv.Handler().ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var got []map[string]any
	json.NewDecoder(rec.Body).Decode(&got)
	if len(got) != 3 {
		t.Errorf("len=%d, want 3", len(got))
	}
}
```

- [ ] **Step 2: Implement**

```go
func (s *Server) handleProbesHistory(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	kind := r.URL.Query().Get("kind")
	rows, err := s.store.RecentProbes(limit, kind)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	out := make([]probeJSON, 0, len(rows))
	for _, p := range rows {
		out = append(out, probeJSON{
			TSMS: p.TS.UnixMilli(), HTTPCode: p.HTTPCode, LatencyMS: p.LatencyMS,
			ExitIP: p.ExitIP, OK: p.OK, Error: p.RawError,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) handleIncidentsHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.RecentIncidents(50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rows)
}

func (s *Server) handleRotationsHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.RecentRotations(50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rows)
}
```

Register:

```go
api.HandleFunc("/api/probes", s.handleProbesHistory)
api.HandleFunc("/api/incidents", s.handleIncidentsHistory)
api.HandleFunc("/api/rotations", s.handleRotationsHistory)
```

Add `import "strconv"`.

- [ ] **Step 3: Frontend tables — minimal but concrete**

Add three components in `web/src/components/`:

`ProbeSparkline.tsx`:

```tsx
import { useEffect, useState } from 'react'

export function ProbeSparkline({ apiKey }: { apiKey: string }) {
  const [data, setData] = useState<{ ts_ms: number; ok: boolean }[]>([])
  useEffect(() => {
    const load = () => fetch('/api/probes?limit=60&kind=active', { headers: { Authorization: `Bearer ${apiKey}` } })
      .then((r) => r.json()).then((d) => setData(d.reverse()))
    load()
    const t = setInterval(load, 10000)
    return () => clearInterval(t)
  }, [apiKey])

  const W = 600, H = 60
  return (
    <section>
      <h2>Active probe (last 60)</h2>
      <svg width={W} height={H} style={{ background: '#111827', borderRadius: 4 }}>
        {data.map((p, i) => (
          <rect key={i} x={(i / data.length) * W} y={p.ok ? H * 0.4 : H * 0.1}
                width={W / data.length - 1} height={p.ok ? H * 0.5 : H * 0.8}
                fill={p.ok ? '#10b981' : '#ef4444'} />
        ))}
      </svg>
    </section>
  )
}
```

`IncidentTable.tsx` and `RotationTable.tsx` follow the same pattern: fetch `/api/incidents` / `/api/rotations` once on mount, render a `<table>` with the columns from the JSON shape (`started_at`, `ended_at`, `trigger_reason`, `terminal_state`, `rotation_count`; for rotations: `old_ip`, `new_ip`, `detection_method`, `ok`).

Sample table component (apply identically for both, swap path + columns):

```tsx
import { useEffect, useState } from 'react'

export function IncidentTable({ apiKey }: { apiKey: string }) {
  const [rows, setRows] = useState<any[]>([])
  useEffect(() => {
    fetch('/api/incidents', { headers: { Authorization: `Bearer ${apiKey}` } })
      .then((r) => r.json()).then(setRows)
  }, [apiKey])
  return (
    <section>
      <h2>Incidents</h2>
      <table className="data-table">
        <thead><tr>
          <th>Started</th><th>Ended</th><th>Trigger</th><th>Final</th><th>Rotations</th>
        </tr></thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.ID}>
              <td>{new Date(r.StartedAt).toLocaleString()}</td>
              <td>{r.EndedAt ? new Date(r.EndedAt).toLocaleString() : 'open'}</td>
              <td>{r.TriggerReason}</td>
              <td>{r.TerminalState}</td>
              <td>{r.RotationCount}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
```

Mount all three in `App.tsx` after the status card.

Add to `styles.css`:

```css
.data-table { width: 100%; border-collapse: collapse; }
.data-table th, .data-table td { padding: 0.5rem; text-align: left; border-bottom: 1px solid #374151; font-size: 0.875rem; }
.data-table th { color: #9ca3af; font-weight: 500; }
```

- [ ] **Step 4: Commit**

```bash
make web-build
git add internal/api/ web/src/
git commit -m "feat: history endpoints + minimal UI tables"
```

---

# Phase 10 — Drill mode

## Task 10.1: `proxywatch drill incident` subcommand

**Files:**
- Modify: `cmd/proxywatch/main.go`

- [ ] **Step 1: Add subcommand**

In `main`, before reading config:

```go
if len(os.Args) > 1 && os.Args[1] == "drill" {
	runDrill(os.Args[2:])
	return
}
```

Add:

```go
func runDrill(args []string) {
	cfg, err := config.Load("/etc/proxywatch.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cfg.Telegram.BotToken == "" {
		fmt.Fprintln(os.Stderr, "telegram not configured")
		os.Exit(1)
	}
	tg := notifier.NewTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, &http.Client{Timeout: 10 * time.Second})
	if err := tg.Send("🧪 proxywatch drill — alert path is working"); err != nil {
		fmt.Fprintln(os.Stderr, "drill failed:", err)
		os.Exit(1)
	}
	fmt.Println("drill alert sent successfully")
}
```

- [ ] **Step 2: Smoke**

```bash
docker run --rm -e PROXYWATCH_KEY=test -v /opt/cliproxyapi/proxywatch.yaml:/etc/proxywatch.yaml proxywatch:dev drill
```

Check Telegram for the drill message.

- [ ] **Step 3: Commit**

```bash
git add cmd/proxywatch/main.go
git commit -m "feat(cli): drill subcommand for end-to-end alert validation"
```

---

# Phase 11 — Production deploy

## Task 11.1: docker-compose example + nginx site

**Files:**
- Create: `docker-compose.example.yml`
- Create: `deploy/nginx-site.conf`

- [ ] **Step 1: docker-compose.example.yml**

```yaml
# Add this service to /opt/cliproxyapi/docker-compose.yml alongside cli-proxy-api and cpa-manager.
services:
  proxywatch:
    image: ghcr.io/tripplemay/proxywatch:latest  # or build locally
    container_name: proxywatch
    ports:
      - "127.0.0.1:18318:18318"
    volumes:
      - proxywatch-data:/data
      - ./logs:/cpa-logs:ro
      - ./proxywatch.yaml:/etc/proxywatch.yaml:ro
    environment:
      - PROXYWATCH_KEY=${PROXYWATCH_KEY}
    restart: unless-stopped
    depends_on:
      - cli-proxy-api

volumes:
  proxywatch-data:
```

- [ ] **Step 2: nginx site**

`deploy/nginx-site.conf`:

```nginx
server {
    listen 80;
    listen [::]:80;
    server_name proxywatch.example.com;

    client_max_body_size 4m;

    location / {
        proxy_pass http://127.0.0.1:18318;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_buffering off;
        proxy_read_timeout 60s;
    }
}
```

- [ ] **Step 3: Deploy steps in README**

Append a "Deployment" section to `README.md` covering:
1. DNS A record for `proxywatch.<your-domain>`
2. Generate `PROXYWATCH_KEY`: `openssl rand -hex 32`
3. Write `/opt/cliproxyapi/proxywatch.yaml`
4. Add the proxywatch service block to `docker-compose.yml`
5. Copy `deploy/nginx-site.conf` to `/etc/nginx/sites-available/proxywatch.<your-domain>` (replacing the server_name), symlink to sites-enabled, reload nginx
6. `certbot --nginx -d proxywatch.<your-domain>`
7. `docker compose up -d proxywatch`
8. Visit `https://proxywatch.<your-domain>`, paste PROXYWATCH_KEY, confirm dashboard loads
9. Run `/api/test-notify` to confirm Telegram path
10. Optionally `docker exec proxywatch /app/proxywatch drill` for the full drill

- [ ] **Step 4: Commit and push**

```bash
git add docker-compose.example.yml deploy/ README.md
git commit -m "docs: deployment recipe for production"
git push
```

---

## Task 11.2: GitHub Actions release workflow (optional MVP polish)

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Workflow that pushes a multi-arch image to ghcr.io on tag**

```yaml
name: Release
on:
  push:
    tags:
      - 'v*'
permissions:
  contents: read
  packages: write
jobs:
  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v6
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: |
            ghcr.io/${{ github.repository_owner }}/proxywatch:${{ github.ref_name }}
            ghcr.io/${{ github.repository_owner }}/proxywatch:latest
```

- [ ] **Step 2: Tag and push**

```bash
git tag v0.1.0
git push origin v0.1.0
```

- [ ] **Step 3: Verify the image exists**

```bash
docker pull ghcr.io/tripplemay/proxywatch:v0.1.0
docker run --rm ghcr.io/tripplemay/proxywatch:v0.1.0 version
```

---

# Self-Review Checklist (run after plan is written)

The author of this plan should verify:

- [ ] Every spec section in `docs/design/proxywatch-design.md` is covered by at least one task. Map:
  - §1 Goals → Phase 0 + README
  - §2 Architecture → Phase 0–4 + 11
  - §3.1 Watcher (active+passive) → Phase 2 + Phase 6
  - §3.2 Decision Engine → Phase 5
  - §3.3 Executor → Phase 7
  - §3.4 Notifier → Phase 4
  - §3.5 Storage → Phase 1
  - §3.6 API+UI → Phase 3 + Phase 9
  - §4 Error handling → covered across phases (401 split in window.go; ipify fallback in iplookup.go; proxy-down handling — **gap, see Task 7.3 below**)
  - §5 Testing → unit tests in each task; drill in Phase 10
  - §6 Deployment → Phase 11

- [ ] No TBDs, no "implement later", every code step has actual code

- [ ] Type/method names consistent across tasks (e.g. `decision.Machine.OnActive`, `store.Probe`, etc.)

## Identified gap → Task 7.3

The spec calls out distinct handling for "proxy gateway is itself down" (`us.miyaip.online:1111` TCP refused → `proxy_down` alert, do NOT enter ROTATING). The current Phase 7 executor doesn't make this distinction.

### Task 7.3: Distinguish proxy_down from rotation-trigger

**Files:**
- Modify: `internal/decision/machine.go`
- Modify: `internal/executor/executor.go`

- [ ] **Step 1: Add a proxy-down counter to the machine**

Add to `Machine`:

```go
proxyDownStreak int
```

Add a method that records connection-class failures separately:

```go
func (m *Machine) OnProxyDown(at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxyDownStreak++
}

func (m *Machine) OnProxyUp(at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxyDownStreak = 0
}

func (m *Machine) IsProxyDown() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.proxyDownStreak >= 3
}
```

- [ ] **Step 2: Classify in prober**

In `internal/prober/active.go`, add a helper that decides whether an error is a "proxy connection refused / network error" vs. an "upstream error after proxy succeeded":

The simplest heuristic: if `RawError` is non-empty and `HTTPCode == 0`, classify as proxy-down.

In `RunOnce`:

```go
if r.HTTPCode == 0 && r.RawError != "" {
	m.OnProxyDown(r.TS)
} else {
	m.OnProxyUp(r.TS)
	m.OnActive(r.TS, r.OK)
}
m.Tick(r.TS)
```

- [ ] **Step 3: Executor reads `IsProxyDown` and sends a different alert (no rotation)**

Modify `tick`:

```go
if e.Machine.IsProxyDown() && e.prevProxyDown == false {
	e.Alert("⚠️ proxy gateway unreachable (us.miyaip.online:1111). This is NOT a rotation trigger; check miyaIP service.", "warning")
	e.prevProxyDown = true
}
if !e.Machine.IsProxyDown() {
	e.prevProxyDown = false
}
```

Add `prevProxyDown bool` to Executor.

- [ ] **Step 4: Tests for both classifications**

`internal/decision/machine_test.go`:

```go
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
```

- [ ] **Step 5: Commit**

```bash
git add internal/decision/ internal/executor/ internal/prober/loop.go
git commit -m "feat: classify proxy-down distinctly from upstream-trigger 4xx"
```

---

# After-Implementation Checklist

- [ ] All `go test -race ./...` pass
- [ ] CI green on `main`
- [ ] Docker image built and tagged
- [ ] README has live deployment steps
- [ ] `proxywatch drill` succeeds against real Telegram
- [ ] First production deploy: low thresholds, observe 24h, no false positives
- [ ] Spec checked: any open §7 items resolved
