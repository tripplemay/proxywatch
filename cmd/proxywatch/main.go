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

	"github.com/tripplemay/proxywatch/internal/api"
	"github.com/tripplemay/proxywatch/internal/config"
	"github.com/tripplemay/proxywatch/internal/decision"
	"github.com/tripplemay/proxywatch/internal/executor"
	"github.com/tripplemay/proxywatch/internal/notifier"
	"github.com/tripplemay/proxywatch/internal/prober"
	"github.com/tripplemay/proxywatch/internal/store"
	"github.com/tripplemay/proxywatch/internal/web"
)

const version = "0.1.0-dev"

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "/etc/proxywatch.yaml", "config file path")
	flag.Parse()

	if len(os.Args) > 1 && os.Args[1] == "drill" {
		runDrill()
		return
	}

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

	// IP lookup must go through the SOCKS5 proxy so we capture the EXIT IP
	// (the proxy's egress, not this server's IP). Reuse socksClient.
	ipLookup := &prober.IPLookup{
		Endpoints: prober.DefaultIPLookupEndpoints,
		Client:    socksClient,
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
		// Read from KV on every call so panel-edited values take effect at the next tick.
		n := s.GetKVInt("active_probe_interval_seconds", cfg.ActiveProbe.IntervalSeconds)
		return time.Duration(n) * time.Second
	}

	log.Info("proxywatch starting", "version", version, "listen", cfg.Listen)
	m := decision.NewMachine(decision.Defaults())
	go prober.Loop(ctx, s, probe, m, getInterval, log)

	// Tick the machine periodically so time-driven transitions fire even
	// without a probe event (e.g. SUSPECT → ROTATING after observation).
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

	// Passive log tail — counts 4xx events from CPA's main.log (when available).
	if cfg.CPALogDir != "" {
		pt := &prober.PassiveTail{
			Path:    filepath.Join(cfg.CPALogDir, "main.log"),
			Pattern: `\[gin_logger\.go:[0-9]+\]\s+(\d{3})`, // confirmed in Task 6.1 (passive_format.md)
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

	var tg *notifier.Telegram // may be nil if not configured
	if cfg.Telegram.BotToken != "" && cfg.Telegram.ChatID != "" {
		tg = notifier.NewTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, &http.Client{Timeout: 10 * time.Second})
		q := &notifier.Queue{Store: s, Telegram: tg}
		go q.Loop(ctx, 10*time.Second, log)

		// Bot: long-poll for incoming commands and callback_query events.
		// Use a separate Telegram client with a longer timeout for long-polling.
		tgBot := notifier.NewTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, &http.Client{Timeout: 35 * time.Second})
		bot := &notifier.Bot{
			Telegram:   tgBot,
			AuthChatID: cfg.Telegram.ChatID,
			Log:        log,
		}
		bot.Commands = map[string]notifier.CommandHandler{
			"/start": helpCmd,
			"/help":  helpCmd,
			"/status": func(ctx context.Context, args string) string {
				return formatStatus(s, m)
			},
			"/probe": func(ctx context.Context, args string) string {
				if err := prober.RunOnce(s, probe, m); err != nil {
					return "probe failed: " + err.Error()
				}
				return "probe done\n\n" + formatStatus(s, m)
			},
		}
		bot.Callbacks = map[string]notifier.CallbackHandler{
			"confirm": func(ctx context.Context, data string) (string, bool) {
				m.Confirm()
				return "✓ confirmed — re-verifying", true
			},
			"resume": func(ctx context.Context, data string) (string, bool) {
				m.ResumeAutomation()
				return "✓ automation resumed", true
			},
			"refresh": func(ctx context.Context, data string) (string, bool) {
				// send full status as a new message; callback ack is brief
				go func() { _ = tg.Send(formatStatus(s, m)) }()
				return "↻ status sent", false
			},
		}
		go bot.Run(ctx)
	} else {
		log.Warn("telegram not configured; notifications will queue but not be sent")
	}

	exec := &executor.Executor{
		Store:   s,
		Machine: m,
		Alert: func(text, level string, buttons []notifier.InlineButton) {
			if len(buttons) > 0 && tg != nil {
				// Alerts with inline buttons go direct — the queue does not support reply_markup.
				// Known tradeoff: these alerts skip retry semantics (no re-queue on failure).
				if err := tg.SendWithButtons(text, buttons); err != nil {
					log.Error("alert send (with buttons)", "err", err)
				}
				return
			}
			// No buttons — go through queue for delivery retry semantics.
			_, _ = s.EnqueueNotification(store.Notification{
				TS: time.Now(), Level: level, Text: text,
			})
		},
		Log: log,
	}
	go exec.Run(ctx, 5*time.Second)

	apiSrv := api.NewServer(s, cfg.AuthKey, version).WithStatic(web.FS()).WithMachine(m)
	srv := &http.Server{Addr: cfg.Listen, Handler: apiSrv.Handler()}
	go func() { _ = srv.ListenAndServe() }()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	srv.Shutdown(shutdownCtx)
}

func helpCmd(_ context.Context, _ string) string {
	return "proxywatch bot commands:\n/status — show current state\n/probe — trigger active probe now\n/help — this message\n\nIn alerts you can tap [I rotated] / [Resume] / [Refresh] inline buttons."
}

func formatStatus(s *store.Store, m *decision.Machine) string {
	rows, _ := s.RecentProbes(1, "active")
	var lastLine string
	exitIP := "(unknown)"
	if len(rows) > 0 {
		p := rows[0]
		exitIP = p.ExitIP
		ageS := int(time.Since(p.TS).Seconds())
		lastLine = fmt.Sprintf("\nlast probe: HTTP %d, %dms, %ds ago", p.HTTPCode, p.LatencyMS, ageS)
	}
	return fmt.Sprintf("state: %s\nexit IP: %s%s", m.State(), exitIP, lastLine)
}

func runDrill() {
	cfg, err := config.Load("/etc/proxywatch.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, "drill: load config:", err)
		os.Exit(1)
	}
	if cfg.Telegram.BotToken == "" || cfg.Telegram.ChatID == "" {
		fmt.Fprintln(os.Stderr, "drill: telegram not configured (bot_token or chat_id missing)")
		os.Exit(1)
	}
	tg := notifier.NewTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, &http.Client{Timeout: 10 * time.Second})
	if err := tg.Send("🧪 proxywatch drill — alert path is working"); err != nil {
		fmt.Fprintln(os.Stderr, "drill: send failed:", err)
		os.Exit(1)
	}
	fmt.Println("drill alert sent successfully")
}
