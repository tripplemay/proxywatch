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

	if cfg.Telegram.BotToken != "" && cfg.Telegram.ChatID != "" {
		tg := notifier.NewTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, &http.Client{Timeout: 10 * time.Second})
		q := &notifier.Queue{Store: s, Telegram: tg}
		go q.Loop(ctx, 10*time.Second, log)
	} else {
		log.Warn("telegram not configured; notifications will queue but not be sent")
	}

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

	apiSrv := api.NewServer(s, cfg.AuthKey, version).WithStatic(web.FS()).WithMachine(m)
	srv := &http.Server{Addr: cfg.Listen, Handler: apiSrv.Handler()}
	go func() { _ = srv.ListenAndServe() }()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	srv.Shutdown(shutdownCtx)
}
