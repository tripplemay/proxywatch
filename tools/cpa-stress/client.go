package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Message is one chat-completion message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is sent to /v1/chat/completions.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

// ChatResult bundles everything we want to log per-request.
type ChatResult struct {
	HTTPCode     int
	ID           string
	Content      string
	FinishReason string
	InTokens     int
	OutTokens    int
	TotalTokens  int
	LatencyMS    int
	Error        string
}

// Client is an OpenAI-compatible chat-completions client.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	Timeout time.Duration
}

type chatRespEnvelope struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

const maxContentBytes = 4096

// Chat issues one chat-completion request and returns the result.
func (c *Client) Chat(ctx context.Context, req ChatRequest) ChatResult {
	start := time.Now()
	res := ChatResult{}

	rctx := ctx
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		rctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	body, err := json.Marshal(req)
	if err != nil {
		res.Error = fmt.Sprintf("marshal: %v", err)
		res.LatencyMS = int(time.Since(start).Milliseconds())
		return res
	}

	httpReq, err := http.NewRequestWithContext(rctx, "POST", c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		res.Error = fmt.Sprintf("build req: %v", err)
		res.LatencyMS = int(time.Since(start).Milliseconds())
		return res
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	res.LatencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	res.HTTPCode = resp.StatusCode

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		res.Error = fmt.Sprintf("read: %v", err)
		return res
	}

	if resp.StatusCode >= 400 {
		s := string(rawBody)
		if len(s) > maxContentBytes {
			s = s[:maxContentBytes]
		}
		res.Content = s
		return res
	}

	var env chatRespEnvelope
	if err := json.Unmarshal(rawBody, &env); err != nil {
		res.Error = fmt.Sprintf("decode: %v", err)
		return res
	}
	res.ID = env.ID
	if len(env.Choices) > 0 {
		c := env.Choices[0]
		s := c.Message.Content
		if len(s) > maxContentBytes {
			s = s[:maxContentBytes]
		}
		res.Content = s
		res.FinishReason = c.FinishReason
	}
	res.InTokens = env.Usage.PromptTokens
	res.OutTokens = env.Usage.CompletionTokens
	res.TotalTokens = env.Usage.TotalTokens
	return res
}
