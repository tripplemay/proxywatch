package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "run.jsonl")
	w, err := NewWriter(p)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	row := Row{
		TSMS:        1700000000000,
		Step:        2,
		Concurrency: 4,
		WorkerID:    1,
		Model:       "gpt-5.4-mini",
		Prompt:      "Write a Python function that reverses a string.",
		Response: &RespBody{
			ID:           "resp_xxx",
			Content:      "ok",
			FinishReason: "stop",
		},
		HTTPCode:    200,
		LatencyMS:   1234,
		InTokens:    41,
		OutTokens:   163,
		TotalTokens: 204,
		ExitIP:      "1.2.3.4",
		ExitIPAgeMS: 320,
	}
	if err := w.Write(row); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, _ := os.Open(p)
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("expected one line")
	}
	var got Row
	if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.HTTPCode != 200 || got.Model != "gpt-5.4-mini" || got.Response == nil || got.Response.Content != "ok" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestWriterConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "run.jsonl")
	w, err := NewWriter(p)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = w.Write(Row{TSMS: int64(i), Step: i, HTTPCode: 200})
		}(i)
	}
	wg.Wait()

	f, _ := os.Open(p)
	defer f.Close()
	count := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r Row
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("malformed line %d: %v", count, err)
		}
		count++
	}
	if count != 50 {
		t.Errorf("got %d rows, want 50", count)
	}
}
