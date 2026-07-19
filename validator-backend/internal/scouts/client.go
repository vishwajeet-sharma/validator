// Package scouts wraps the Yutori HTTP APIs used by the Validator worker:
// the Research API (one-time research + structured synthesis) and the Scouting
// API (long-running monitoring scouts with webhook delivery, PATCH updates, and
// email settings). All calls authenticate with the X-API-Key header.
package scouts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"validator-backend/internal/llm"
)

// Client talks to the Yutori API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	llm        *llm.Client // optional; used for the mutation eval so it never hits Yutori
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient injects a custom *http.Client (useful for tests / tuning).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithLLM injects an OpenAI-compatible LLM client used by the mutation eval
// (ReviewMutation). Without it, ReviewMutation is skipped (see LLMConfigured).
func WithLLM(c *llm.Client) Option {
	return func(cl *Client) { cl.llm = c }
}

// LLMConfigured reports whether an LLM client is wired and ready, so callers can
// skip the mutation eval (instead of falling back to a paid Yutori research task).
func (c *Client) LLMConfigured() bool { return c.llm != nil && c.llm.Configured() }

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

const (
	// researchPollInterval is the delay between status checks while a task runs.
	researchPollInterval = 8 * time.Second
	// researchPollTimeout caps the total wait for a single research task.
	researchPollTimeout = 8 * time.Minute
)

// Signal is one harvested finding returned in a scout's structured output.
type Signal struct {
	Platform    string `json:"platform"`
	Quote       string `json:"quote"`
	Reason      string `json:"reason"`
	SourceURL   string `json:"source_url"`
	SourceTitle string `json:"source_title"`
}

// SignalSchema is the JSON Schema sent as output_schema so Yutori returns
// findings as structured data (both for Research and Scouting tasks).
func SignalSchema() map[string]any {
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

// Day0Schema is the combined output_schema for the single Day 0 research task.
// It asks Yutori to harvest market signals AND synthesise the two PRO/CON
// monitoring prompts in one pass, returning findings SPLIT by polarity so they
// can be recorded directly under the matching scout (PRO demand vs CON threats).
// Only the prompts are required; signals ground the synthesis but are optional.
func Day0Schema() map[string]any {
	signalItem := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"platform":     map[string]any{"type": "string"},
			"quote":        map[string]any{"type": "string"},
			"reason":       map[string]any{"type": "string"},
			"source_url":   map[string]any{"type": "string"},
			"source_title": map[string]any{"type": "string"},
		},
		"required": []string{"quote", "source_url"},
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pro_signals": map[string]any{
				"type":  "array",
				"items": signalItem,
				"description": "Demand-side findings: customer pain, willingness to pay, unmet needs, positive traction, market growth.",
			},
			"con_signals": map[string]any{
				"type":  "array",
				"items": signalItem,
				"description": "Threat-side findings: competitors, saturation, regulatory risk, switching costs, user resistance, negative sentiment.",
			},
			"pro_prompt": map[string]any{"type": "string"},
			"con_prompt": map[string]any{"type": "string"},
			"reasoning":  map[string]any{"type": "string"},
		},
		"required": []string{"pro_prompt", "con_prompt"},
	}
}

// MutationReviewSchema is the output_schema used by the webhook's LLM block to
// decide whether a scout's search radius should expand given fresh signals.
func MutationReviewSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"should_expand":   map[string]any{"type": "boolean"},
			"proposed_prompt": map[string]any{"type": "string"},
			"reasoning":       map[string]any{"type": "string"},
		},
		"required": []string{"should_expand"},
	}
}

// Day0Result is the decoded result of the single Day 0 research task: harvested
// signals SPLIT by polarity (so each side populates the matching pros/cons table)
// plus the synthesised PRO/CON monitoring prompts.
type Day0Result struct {
	ProSignals []Signal `json:"pro_signals"`
	ConSignals []Signal `json:"con_signals"`
	ProPrompt  string   `json:"pro_prompt"`
	ConPrompt  string   `json:"con_prompt"`
	Reasoning  string   `json:"reasoning,omitempty"`
}

// MutationReview is the decoded result of the webhook LLM evaluation call.
type MutationReview struct {
	ShouldExpand   bool   `json:"should_expand"`
	ProposedPrompt string `json:"proposed_prompt,omitempty"`
	Reasoning      string `json:"reasoning,omitempty"`
}

// DecodeSignals parses a structured_result payload into signals, accepting
// either {"signals":[...]} or a bare [...].
func DecodeSignals(raw []byte) ([]Signal, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, Definitive(fmt.Errorf("structured result was empty"))
	}

	var wrapper struct {
		Signals []Signal `json:"signals"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Signals != nil {
		return sanitize(wrapper.Signals), nil
	}

	var arr []Signal
	if err := json.Unmarshal(raw, &arr); err == nil {
		return sanitize(arr), nil
	}
	return nil, Definitive(fmt.Errorf("could not decode signals from structured result: %s", truncate(raw, 256)))
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

// ErrLLMNotConfigured is returned by ReviewMutation when no LLM client is wired.
// Callers treat it as "skip the eval", not a failure.
var ErrLLMNotConfigured = errors.New("scouts: llm not configured for mutation eval")

// DefinitiveError marks an error as non-retryable: a business-logic, parse, or
// 4xx failure that will deterministically fail again if the side effect is
// replayed (e.g. a research task that returned "failed", a malformed structured
// result, or a bad API key). Restate closures convert these into a
// restate.TerminalError so the step fails fast instead of being retried — and
// re-issuing the external call that produced the error in the first place.
//
// Errors NOT wrapped in DefinitiveError are treated as transient (network
// blips, 429, 5xx) and are retried under the bounded Run retry policy.
type DefinitiveError struct{ err error }

func (e *DefinitiveError) Error() string { return e.err.Error() }
func (e *DefinitiveError) Unwrap() error { return e.err }

// Definitive wraps err as a non-retryable DefinitiveError. It returns nil if err is nil.
func Definitive(err error) error {
	if err == nil {
		return nil
	}
	return &DefinitiveError{err: err}
}

// IsDefinitive reports whether err is, or wraps, a DefinitiveError.
func IsDefinitive(err error) bool {
	var d *DefinitiveError
	return errors.As(err, &d)
}

// do performs an HTTP request with the X-API-Key header and returns the raw
// body, enforcing a 1 MiB cap and a non-2xx error.
func (c *Client) do(req *http.Request) ([]byte, int, error) {
	req.Header.Set("X-API-Key", c.apiKey)
	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close() //nolint:errcheck
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func httpError(prefix string, status int, body []byte) error {
	err := fmt.Errorf("%s: status %d: %s", prefix, status, truncate(body, 512))
	// 429 (rate limited) and 5xx (server) are transient: leave them retryable
	// under the bounded Run retry policy. Every other non-2xx (4xx auth/config/
	// protocol errors) is definitive — replaying the call will fail identically.
	if status == 429 || status >= 500 {
		return err
	}
	return Definitive(err)
}
