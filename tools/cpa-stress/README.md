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
