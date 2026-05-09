# proxywatch

A health-monitoring sidecar for [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI) deployments that route through a manually-rotated SOCKS5 upstream proxy (e.g. miyaIP).

`proxywatch` watches both passive request traffic (4xx rates from upstream AI providers) and active probes (periodic calls through the proxy), decides when the exit IP has likely been blocked, alerts the operator via Telegram, and verifies recovery once the operator manually rotates the IP at their proxy provider.

## Status

**Working — feature-complete for v0.1.** All planned phases (0–11) of the design are landed:

- Periodic active probes through SOCKS5
- Passive log tail of CPA's main.log (counts 403/429)
- Decision state machine (HEALTHY → SUSPECT → ROTATING → VERIFYING → COOLDOWN, plus ALERT_ONLY)
- Executor that opens incidents, alerts on transitions, auto-detects IP changes
- Telegram notifier with retry queue
- Web UI (single page, embedded in binary): live status, exit IP, sparkline, incidents, rotations, settings
- Manual override endpoints (confirm-rotation, resume-automation)
- proxy-down classification (TCP failure vs upstream 4xx)
- `proxywatch drill` CLI subcommand for alert-path validation

The design is at [`docs/design/proxywatch-design.md`](docs/design/proxywatch-design.md).

## Why this exists

CPA's built-in features and CPA-Manager already cover account management, model routing, and per-request usage analytics. What they do not cover: the upstream SOCKS5 proxy (the path between CPA and the AI providers). When that proxy's exit IP is rate-limited or blocked by an upstream provider, CPA cannot fix itself — it needs an out-of-band signal. proxywatch is that signal, plus a state machine that turns the signal into a usable workflow (alert → operator action → automatic verification).

## Tech stack

- Backend: Go 1.25 (single static binary; minimal memory footprint)
- Frontend: React 18 + TypeScript, Vite-built, embedded into the Go binary via `embed.FS`
- Storage: SQLite (`modernc.org/sqlite`, pure-Go, no CGO)
- Deployment: Docker, alongside CPA in the same `docker-compose.yml`

## Deployment

These steps assume you already have CPA running on a Linux host with Docker, nginx, and certbot installed.

### 1. Add a DNS A record

Point a subdomain (e.g. `proxywatch.example.com`) at the host's IP. Wait for it to resolve (a few minutes).

### 2. Generate a panel access key

```bash
openssl rand -hex 32
```

Save this — you'll paste it into the panel on first visit. Drop it in `/opt/cliproxyapi/proxywatch.env` as:

```
PROXYWATCH_KEY=<the-key>
```

```bash
chmod 600 /opt/cliproxyapi/proxywatch.env
```

### 3. Write the proxywatch config

Create `/opt/cliproxyapi/proxywatch.yaml`:

```yaml
listen: ":18318"
data_dir: "/data"
cpa_proxy_url: "socks5h://USER:PASS@your-proxy-host:1111"
cpa_log_dir: "/cpa-logs"
active_probe:
  target: "https://www.google.com/generate_204"
  interval_seconds: 60
  timeout_seconds: 15
telegram:
  bot_token: ""
  chat_id: ""
```

The `telegram` block can be empty initially; you'll fill it in via the panel after first login.

### 4. Build the image and add the service

```bash
git clone https://github.com/tripplemay/proxywatch /opt/proxywatch-src
cd /opt/proxywatch-src
docker build -t proxywatch:0.1.0 .
```

Then add the proxywatch service block from [`docker-compose.example.yml`](docker-compose.example.yml) to your existing `/opt/cliproxyapi/docker-compose.yml`.

### 5. Configure nginx + TLS

Copy [`deploy/nginx-site.conf`](deploy/nginx-site.conf) to `/etc/nginx/sites-available/<your-domain>` and replace `<YOUR_DOMAIN>` with your actual subdomain. Then:

```bash
ln -s /etc/nginx/sites-available/<your-domain> /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx
certbot --nginx -d <your-domain>
```

### 6. Bring up the service

```bash
cd /opt/cliproxyapi
docker compose up -d proxywatch
```

### 7. First-time setup in the panel

Visit `https://<your-domain>` in your browser. Paste the `PROXYWATCH_KEY` from step 2. Click Save.

You should see the dashboard: state HEALTHY, current exit IP, last probe status.

### 8. Validate the Telegram alert path

If you've configured Telegram (`bot_token` + `chat_id` in panel Settings or yaml):

```bash
docker exec proxywatch /app/proxywatch drill
```

Expected: a "🧪 proxywatch drill — alert path is working" message in your Telegram chat within a few seconds.

## License

Not yet decided. Will be set before any production-grade releases.
