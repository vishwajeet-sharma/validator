// Package yutori is a production HTTP client for the Yutori Research API.
//
// It translates a Scouting directive (idea + active watchlist of keywords +
// target platforms + desired polarity) into a one-time Yutori research task,
// polls it to completion, and decodes the structured result back into typed
// signals. Yutori owns the research (100+ MCP tools, multi-agent web search);
// Validator owns the cadence via the Restate durable workflow.
package yutori

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"validator-backend/internal/models"
)

const (
	// researchPollInterval is the delay between status checks while a task runs.
	researchPollInterval = 8 * time.Second
	// researchPollTimeout caps the total wait for a single research task. It is
	// kept under Restate's abort_timeout so a slow task can fail loudly rather
	// than get killed mid-flight.
	researchPollTimeout = 8 * time.Minute
)

// Client talks to the Yutori Research API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient injects a custom *http.Client (useful for tests / tuning).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// New builds a Yutori client from base URL, API key and per-request timeout.
func New(baseURL, apiKey string, timeout time.Duration, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ScoutRequest is the input to a single Pro- or Con-Scout research task.
type ScoutRequest struct {
	IdeaTitle       string
	IdeaDescription string
	Polarity        models.Polarity // "pro" or "con"
	Platforms       []string        // active target platforms (reddit, youtube, news, ...)
	Keywords        []string        // current watchlist / search radius
}

// Signal is one harvested finding returned by the scout.
type Signal struct {
	Platform    string `json:"platform"`
	Quote       string `json:"quote"`
	Reason      string `json:"reason"`
	SourceURL   string `json:"source_url"`
	SourceTitle string `json:"source_title"`
}

// researchCreateRequest is the body for POST /v1/research/tasks.
type researchCreateRequest struct {
	Query        string         `json:"query"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	UserTimezone string         `json:"user_timezone,omitempty"`
}

type researchCreateResponse struct {
	TaskID     string `json:"task_id"`
	ViewURL    string `json:"view_url"`
	Status     string `json:"status"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

// researchStatusResponse is the body for GET /v1/research/tasks/{task_id}.
type researchStatusResponse struct {
	TaskID                 string          `json:"task_id"`
	Status                 string          `json:"status"`
	Result                 string          `json:"result,omitempty"` // markdown, when no schema
	StructuredResult       json.RawMessage `json:"structured_result,omitempty"`
	StructuredOutputStatus string          `json:"structured_output_status,omitempty"`
	RejectionReason        string          `json:"rejection_reason,omitempty"`
}

// Scout creates a Yutori research task for the directive and polls until it
// completes, returning the decoded signals. The create+poll runs inside a
// single call so the Restate workflow can treat it as one side-effect.
func (c *Client) Scout(ctx context.Context, req ScoutRequest) ([]Signal, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("yutori: YUTORI_API_KEY is not configured")
	}

	taskID, err := c.createTask(ctx, req)
	if err != nil {
		return nil, err
	}
	slog.Info("yutori research task created",
		"polarity", req.Polarity, "task_id", taskID,
		"platforms", req.Platforms, "watchlist_size", len(req.Keywords))

	signals, err := c.awaitResult(ctx, taskID)
	if err != nil {
		return nil, err
	}

	slog.Info("yutori research task completed",
		"polarity", req.Polarity, "task_id", taskID, "signals", len(signals))
	return signals, nil
}

func (c *Client) createTask(ctx context.Context, req ScoutRequest) (string, error) {
	body := researchCreateRequest{
		Query:        buildQuery(req),
		OutputSchema: signalSchema(),
		UserTimezone: "America/Los_Angeles",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("yutori: marshal request: %w", err)
	}

	endpoint := c.baseURL + "/v1/research/tasks"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("yutori: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("yutori: create research task: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	resBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return "", fmt.Errorf("yutori: read create response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("yutori: create task returned status %d: %s", resp.StatusCode, truncate(resBody, 512))
	}

	var out researchCreateResponse
	if err := json.Unmarshal(resBody, &out); err != nil {
		return "", fmt.Errorf("yutori: decode create response: %w", err)
	}
	if out.TaskID == "" {
		return "", fmt.Errorf("yutori: create response missing task_id: %s", truncate(resBody, 256))
	}
	return out.TaskID, nil
}

// awaitResult polls the task status until it is succeeded or failed, or until
// researchPollTimeout elapses.
func (c *Client) awaitResult(ctx context.Context, taskID string) ([]Signal, error) {
	deadline := time.Now().Add(researchPollTimeout)
	for {
		status, structured, resultMD, err := c.getStatus(ctx, taskID)
		if err != nil {
			return nil, err
		}
		switch status {
		case "succeeded":
			signals, derr := decodeStructuredSignals(structured)
			if derr != nil {
				return nil, fmt.Errorf("yutori: task %s succeeded but %w (markdown: %s)",
					taskID, derr, truncate([]byte(resultMD), 256))
			}
			return signals, nil
		case "failed":
			return nil, fmt.Errorf("yutori: research task %s failed", taskID)
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("yutori: research task %s timed out after %s", taskID, researchPollTimeout)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(researchPollInterval):
		}
	}
}

func (c *Client) getStatus(ctx context.Context, taskID string) (status string, structured json.RawMessage, resultMD string, err error) {
	endpoint := c.baseURL + "/v1/research/tasks/" + taskID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", nil, "", fmt.Errorf("yutori: build status request: %w", err)
	}
	httpReq.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", nil, "", fmt.Errorf("yutori: status request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", nil, "", fmt.Errorf("yutori: read status response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, "", fmt.Errorf("yutori: status returned %d: %s", resp.StatusCode, truncate(body, 512))
	}

	var out researchStatusResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", nil, "", fmt.Errorf("yutori: decode status response: %w", err)
	}
	return out.Status, out.StructuredResult, out.Result, nil
}

// buildQuery composes the natural-language research directive for a scout.
func buildQuery(req ScoutRequest) string {
	polarityWord := "supporting"
	focus := "demand, willingness to pay, unmet needs, positive traction, market growth"
	if req.Polarity == models.PolarityCon {
		polarityWord = "threatening"
		focus = "competition, market saturation, regulatory risk, user resistance, switching costs"
	}

	return fmt.Sprintf(
		"You are an expert market research scout. Research REAL, verifiable market signals %s "+
			"the product idea below across the web. For each finding, return a direct quote, "+
			"the source URL, a short source title, the platform/domain it came from "+
			"(e.g. reddit, youtube, news, blog, forum, social), and a concise reason why it is a %s signal. "+
			"Focus on: %s. Only include findings attributable to a real, verifiable URL. Return up to 8 distinct findings.\n\n"+
			"IDEA:\n%s\n%s\n\n"+
			"PLATFORMS / DOMAINS TO PRIORITIZE: %s\n"+
			"KEYWORDS / SEARCH RADIUS: %s",
		polarityWord, req.Polarity, focus,
		req.IdeaTitle, req.IdeaDescription,
		joinNonEmpty(req.Platforms),
		joinNonEmpty(req.Keywords),
	)
}

// signalSchema is the JSON Schema sent as output_schema so Yutori returns
// findings as structured data instead of free-form markdown.
func signalSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"signals": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"platform":     map[string]any{"type": "string"},
						"quote":        map[string]any{"type": "string"},
						"reason":       map[string]any{"type": "string"},
						"source_url":   map[string]any{"type": "string"},
						"source_title": map[string]any{"type": "string"},
					},
					"required": []string{"quote", "source_url"},
				},
			},
		},
		"required": []string{"signals"},
	}
}

// decodeStructuredSignals parses the structured_result payload into signals.
// It accepts either {"signals":[...]} or a bare [...].
func decodeStructuredSignals(raw json.RawMessage) ([]Signal, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("structured result was empty")
	}

	// The expected contract is a {"signals": [...]} object.
	var wrapper struct {
		Signals []Signal `json:"signals"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Signals != nil {
		return sanitize(wrapper.Signals), nil
	}

	// Some tasks return a bare array instead of {"signals":[...]}.
	var arr []Signal
	if err := json.Unmarshal(raw, &arr); err == nil {
		return sanitize(arr), nil
	}

	return nil, fmt.Errorf("could not decode signals from structured result: %s", truncate(raw, 256))
}

// sanitize drops any signal missing the essential fields and forces a sane platform.
func sanitize(in []Signal) []Signal {
	out := make([]Signal, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s.Quote) == "" || strings.TrimSpace(s.SourceURL) == "" {
			continue
		}
		if strings.TrimSpace(s.Platform) == "" {
			s.Platform = "web"
		}
		if strings.TrimSpace(s.Reason) == "" {
			s.Reason = "Signal detected during scouting."
		}
		out = append(out, s)
	}
	return out
}

func joinNonEmpty(in []string) string {
	clean := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			clean = append(clean, v)
		}
	}
	return strings.Join(clean, ", ")
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
