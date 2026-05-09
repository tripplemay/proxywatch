# proxywatch

A health-monitoring sidecar for [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI) deployments that route through a manually-rotated SOCKS5 upstream proxy (e.g. miyaIP).

`proxywatch` watches both passive request traffic (4xx rates from upstream AI providers) and active probes (periodic calls through the proxy to OpenAI), decides when the exit IP has likely been blocked, alerts the operator via Telegram, and verifies recovery once the operator manually rotates the IP at their proxy provider.

## Status

**Design phase.** Implementation has not started.

The design document is at [`docs/design/proxywatch-design.md`](docs/design/proxywatch-design.md). It will be the source of truth for the implementation plan.

## Why this exists

CPA's built-in features and CPA-Manager already cover account management, model routing, and per-request usage analytics. What they do not cover: the upstream SOCKS5 proxy (the path between CPA and the AI providers). When that proxy's exit IP is rate-limited or blocked by an upstream provider, CPA cannot fix itself — it needs an out-of-band signal. proxywatch is that signal, plus a state machine that turns the signal into a usable workflow (alert → operator action → automatic verification).

## Scope (initial release)

- Active probe of `api.openai.com` through the configured SOCKS5 proxy
- Passive tail of CPA's log file for 403/429 patterns
- State machine: `HEALTHY → SUSPECT → ROTATING → VERIFYING → COOLDOWN`
- Telegram alerts (out-of-band; does not go through the SOCKS5 proxy)
- Lightweight web panel (single-page, embedded in the binary) for status, history, and tunable parameters
- SQLite for persistence

Explicit non-goals are listed in the design doc, §8.

## Tech stack (planned)

- Backend: Go (single static binary; matches CPA's stack; minimal memory footprint)
- Frontend: React + TypeScript, Vite-built, embedded into the Go binary
- Storage: SQLite
- Deployment: Docker, alongside CPA in the same `docker-compose.yml`

## License

Not yet decided. Will be set before code is published.
