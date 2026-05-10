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
