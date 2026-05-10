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

	sampler, err := NewSamplerOverSOCKS5(socksURL, 5*time.Second)
	if err != nil {
		log.Fatalf("build socks5 sampler: %v", err)
	}

	steps := buildSteps(dryRun, stepDuration)
	hardLimit := 25 * time.Minute
	if dryRun {
		hardLimit = 5 * time.Minute
	}

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

	go sampler.Run(ctx, 1*time.Second)

	log.Printf("cpa-stress %s starting", version)
	log.Printf("  jsonl  -> %s", jsonlPath)
	log.Printf("  report -> %s", reportPath)
	log.Printf("  steps  -> %d (dryRun=%v)", len(steps), dryRun)

	reason := r.Run(ctx, startedAt)
	if ctx.Err() != nil && reason == StopComplete {
		reason = StopSignal
	}
	endedAt := time.Now()

	log.Printf("cpa-stress finished: reason=%s, duration=%s", reason, endedAt.Sub(startedAt).Round(time.Second))

	if err := w.Close(); err != nil {
		log.Printf("warn: close writer: %v", err)
	}

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
