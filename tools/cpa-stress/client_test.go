package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req ChatRequest
		_ = json.Unmarshal(body, &req)
		if r.Header.Get("Authorization") != "Bearer testkey" {
			t.Errorf("missing or wrong auth header")
		}
		if req.Model != "gpt-5.4-mini" {
			t.Errorf("model=%q", req.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":"resp_x",
			"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}
		}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "testkey", HTTP: srv.Client(), Timeout: 5 * time.Second}
	res := c.Chat(context.Background(), ChatRequest{
		Model:       "gpt-5.4-mini",
		Messages:    []Message{{Role: "user", Content: "hi"}},
		MaxTokens:   100,
		Temperature: 0.7,
	})
	if res.HTTPCode != 200 {
		t.Errorf("HTTPCode=%d", res.HTTPCode)
	}
	if res.Content != "hello" {
		t.Errorf("Content=%q", res.Content)
	}
	if res.ID != "resp_x" {
		t.Errorf("ID=%q", res.ID)
	}
	if res.InTokens != 5 || res.OutTokens != 2 || res.TotalTokens != 7 {
		t.Errorf("tokens: %+v", res)
	}
	if res.FinishReason != "stop" {
		t.Errorf("FinishReason=%q", res.FinishReason)
	}
	if res.Error != "" {
		t.Errorf("unexpected error: %s", res.Error)
	}
	if res.LatencyMS <= 0 {
		t.Errorf("LatencyMS=%d", res.LatencyMS)
	}
}

func TestChat4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, APIKey: "k", HTTP: srv.Client(), Timeout: 5 * time.Second}
	res := c.Chat(context.Background(), ChatRequest{Model: "x", Messages: []Message{{Role: "user", Content: "hi"}}})
	if res.HTTPCode != 429 {
		t.Errorf("HTTPCode=%d", res.HTTPCode)
	}
	if !strings.Contains(res.Content, "rate limited") {
		t.Errorf("content should include error body, got %q", res.Content)
	}
	if res.Error != "" {
		t.Errorf("Error should be empty for HTTP 4xx, got %q", res.Error)
	}
}

func TestChatTransportError(t *testing.T) {
	c := &Client{
		BaseURL: "http://127.0.0.1:1",
		APIKey:  "k",
		HTTP:    &http.Client{Timeout: 500 * time.Millisecond},
		Timeout: 500 * time.Millisecond,
	}
	res := c.Chat(context.Background(), ChatRequest{Model: "x", Messages: []Message{{Role: "user", Content: "hi"}}})
	if res.HTTPCode != 0 {
		t.Errorf("HTTPCode=%d, want 0 on transport error", res.HTTPCode)
	}
	if res.Error == "" {
		t.Error("Error should be populated on transport error")
	}
}
