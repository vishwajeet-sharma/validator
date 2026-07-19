// Package llm provides a minimal OpenAI-compatible chat-completions client used
// for the scout prompt-mutation evaluation. It is intentionally provider
// agnostic: point LLM_API_BASE at OpenAI, Groq, Together, OpenRouter, or a local
// Ollama/vLLM OpenAI endpoint. This keeps the eval OFF the Yutori Research API,
// which is expensive and reserved for Day 0 research + recurring scouts.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotConfigured is returned when the client lacks the minimum settings to
// make a call. Callers should treat it as "skip the eval" rather than a failure.
var ErrNotConfigured = errors.New("llm: not configured (LLM_API_KEY, LLM_API_BASE and LLM_MODEL required)")

// Client is an OpenAI-compatible chat completions client.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient injects a custom *http.Client (useful for tests / tuning).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// New builds a client. It always returns a non-nil client so callers can
// construct it unconditionally; Complete returns ErrNotConfigured when the
// required settings are missing.
func New(baseURL, apiKey, model string, timeout time.Duration, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Configured reports whether the client has the minimum settings to make a call.
func (c *Client) Configured() bool {
	return c.apiKey != "" && c.baseURL != "" && c.model != ""
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Complete sends a chat-completions request and returns the assistant's text.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	if !c.Configured() {
		return "", ErrNotConfigured
	}
	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.2,
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("llm: status %d: %s", resp.StatusCode, truncate(string(body), 512))
	}
	var out chatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("llm: decode response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("llm: api error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm: empty choices")
	}
	return out.Choices[0].Message.Content, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
