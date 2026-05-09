# CPA log format (observed v6.10.9)

This document records the format of CPA's log lines as observed on a
production deployment. The passive prober tails `main.log` and extracts
HTTP status codes from request-completion lines so the decision engine
can count 4xx events.

## Line format

Every request-completion line has the structure:

```
[YYYY-MM-DD HH:MM:SS] [REQID] [LEVEL] [gin_logger.go:NNN] <STATUS> | <LATENCY> | <CLIENT_IP> | <METHOD> <PATH>
```

Fields:

- **Timestamp** — bracketed `[2026-05-09 11:56:29]` in server local time (UTC on this server).
- **Request ID** — 8-character hex like `[1be5be62]`, OR `[--------]` for events without a per-request context (e.g. startup, periodic background tasks).
- **Level** — one of `[info ]`, `[warn ]`, `[error]` (note the space padding to length 5).
- **Source location** — `[gin_logger.go:NNN]` for request lifecycle events. Other source files appear for non-request events (e.g. `[selector.go:...]`, `[main.go:...]`); these do NOT carry HTTP status codes and should be ignored by the passive prober.
- **Status code** — three digits, immediately after the closing bracket of the source location, separated by a space. Followed by a ` | ` separator.
- **Latency** — right-aligned numeric with unit suffix, e.g. `184ms`, `1.234s`. Ignored.
- **Client IP** — right-aligned. Ignored.
- **Method + Path** — e.g. `POST /v1/chat/completions`. Ignored for now (Phase 5 only consumes the status code; future phases may use path for per-endpoint stats).

## Sample lines (sanitized prefix only)

```
[2026-05-09 11:56:29] [--------] [info ] [gin_logger.go:100] 200 |         184ms | ...
[2026-05-09 11:56:30] [1be5be62] [info ] [gin_logger.go:100] 200 |         188ms | ...
[2026-05-09 11:57:01] [88b6df60] [info ] [gin_logger.go:100] 403 |         421ms | ...
```

## Grep / regex pattern

For the passive prober (`internal/prober/passive.go`), use this regex to
extract the status code from each line:

```
\[gin_logger\.go:[0-9]+\]\s+(\d{3})
```

This explicitly anchors on `[gin_logger.go:` so it ignores non-request
log lines from other source files. The single capture group is the
3-digit HTTP status code.

In Go's `regexp` syntax this is the same string. Use
`regexp.MustCompile` at startup; on each new line, run `FindSubmatch`
and parse the integer if a match was found.

## Edge cases

- **Lines with `[--------]` request id** — these CAN still be from
  `gin_logger.go` (e.g. when the request didn't reach a real handler,
  the request-id middleware may not have populated the slot). The regex
  above matches them correctly. They count as real status events.
- **Multi-line responses** — gin_logger emits one line per request, so
  there's no multi-line concern. Don't try to handle continuation.
- **Streaming / partial responses** — when a streaming response is
  cancelled mid-stream, gin still logs the final status (often 200
  even if the client gave up). This is OK; we only care about
  initial-response status codes which gin captures correctly.
- **Log rotation** — CPA writes to a single `main.log`. Past versions
  rotated; current version (v6.10.9) keeps appending. The passive
  prober uses fsnotify + fseek-to-end on startup, so it tolerates both
  growing files and rotation (via the file replacement signal).

## Verification command (manual)

To sanity-check the format on a live server:

```bash
awk 'NR<=5 && /gin_logger/{print substr($0, 50, 50)}' /opt/cliproxyapi/logs/main.log
```

Expected output: lines starting with `ger.go:NNN] <code> | <latency> |` —
i.e. the closing bracket of the source location, the status code, and
the separator. Any deviation indicates a CPA version with a different
log format; the regex would need updating.

## Implementation notes for Task 6.2

- The `PassiveTail.Pattern` field in the prober struct should be set to
  the regex above when wiring in `main.go`.
- Filter to ONLY count status codes 403 and 429 (the `Window` in
  `internal/decision/window.go` already does this via `IsTriggerCode`).
  401 should NOT count (auth issue, not IP-related).
- Codes 200/204/3xx count as "healthy traffic" — the passive prober
  may emit them but the decision engine ignores them.
