package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Row is one logged request.
type Row struct {
	TSMS        int64     `json:"ts_ms"`
	Step        int       `json:"step"`
	Concurrency int       `json:"concurrency"`
	WorkerID    int       `json:"worker_id"`
	Model       string    `json:"model"`
	Prompt      string    `json:"prompt"`
	Response    *RespBody `json:"response,omitempty"`
	HTTPCode    int       `json:"http_code"`
	LatencyMS   int       `json:"latency_ms"`
	InTokens    int       `json:"in_tokens"`
	OutTokens   int       `json:"out_tokens"`
	TotalTokens int       `json:"total_tokens"`
	ExitIP      string    `json:"exit_ip"`
	ExitIPAgeMS int       `json:"exit_ip_age_ms"`
	Error       string    `json:"error"`
}

// RespBody is the part of the upstream response we keep.
type RespBody struct {
	ID           string `json:"id"`
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason"`
}

// Writer serializes Rows to a JSONL file. Safe for concurrent use.
type Writer struct {
	mu  sync.Mutex
	f   *os.File
	buf *bufio.Writer
	enc *json.Encoder
}

// NewWriter creates and opens (truncating) the output file.
func NewWriter(path string) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	buf := bufio.NewWriterSize(f, 64*1024)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	return &Writer{f: f, buf: buf, enc: enc}, nil
}

// Write serializes one row and flushes (so kills don't lose data).
func (w *Writer) Write(r Row) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.enc.Encode(r); err != nil {
		return err
	}
	return w.buf.Flush()
}

// Close flushes and closes the underlying file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.buf.Flush(); err != nil {
		_ = w.f.Close()
		return err
	}
	return w.f.Close()
}
