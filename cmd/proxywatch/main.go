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

	apiSrv := api.NewServer(s, cfg.AuthKey, version)
	srv := &http.Server{Addr: cfg.Listen, Handler: apiSrv.Handler()}
	go func() { _ = srv.ListenAndServe() }()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	srv.Shutdown(shutdownCtx)
}
