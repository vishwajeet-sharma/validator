package scouts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CreateScoutRequest is the body for POST /v1/scouting/tasks.
type CreateScoutRequest struct {
	Query         string         `json:"query"`
	OutputSchema  map[string]any `json:"output_schema,omitempty"`
	OutputInterval int           `json:"output_interval,omitempty"`
	WebhookURL    string         `json:"webhook_url,omitempty"`
	WebhookFormat string         `json:"webhook_format,omitempty"`
	UserTimezone  string         `json:"user_timezone,omitempty"`
	SkipEmail     bool           `json:"skip_email"`
	IsPublic      bool           `json:"is_public"`
}

// CreateScoutResponse mirrors Yutori's CreateScoutResponsePublic.
type CreateScoutResponse struct {
	ID           string `json:"id"`
	Query        string `json:"query"`
	DisplayName  string `json:"display_name"`
	ViewURL      string `json:"view_url"`
	NextRunAt    string `json:"next_run_timestamp"`
	WebhookURL   string `json:"webhook_url,omitempty"`
	RejectionReason string `json:"rejection_reason,omitempty"`
}

// CreateScout deploys a new long-running Yutori scout.
func (c *Client) CreateScout(ctx context.Context, req CreateScoutRequest) (CreateScoutResponse, error) {
	if c.apiKey == "" {
		return CreateScoutResponse{}, Definitive(fmt.Errorf("scouts: YUTORI_API_KEY is not configured"))
	}
	if req.OutputInterval == 0 {
		req.OutputInterval = 86400 // daily default
	}
	if req.WebhookFormat == "" && req.WebhookURL != "" {
		req.WebhookFormat = "scout"
	}
	if req.UserTimezone == "" {
		req.UserTimezone = "America/Los_Angeles"
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return CreateScoutResponse{}, fmt.Errorf("scouts: marshal create scout: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/scouting/tasks", bytes.NewReader(raw))
	if err != nil {
		return CreateScoutResponse{}, fmt.Errorf("scouts: build create scout: %w", err)
	}
	body, status, err := c.do(httpReq)
	if err != nil {
		return CreateScoutResponse{}, fmt.Errorf("scouts: create scout: %w", err)
	}
	if status < 200 || status >= 300 {
		return CreateScoutResponse{}, httpError("scouts: create scout", status, body)
	}
	var out CreateScoutResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return CreateScoutResponse{}, fmt.Errorf("scouts: decode create scout: %w", err)
	}
	if out.ID == "" {
		return CreateScoutResponse{}, Definitive(fmt.Errorf("scouts: create scout response missing id: %s", truncate(body, 256)))
	}
	return out, nil
}

// PatchScoutRequest is the body for PATCH /v1/scouting/tasks/{scout_id}. Only
// provided fields are updated.
type PatchScoutRequest struct {
	Query          string         `json:"query,omitempty"`
	OutputInterval int            `json:"output_interval,omitempty"`
	WebhookURL     *string        `json:"webhook_url,omitempty"` // nil = omit; "" = remove
	OutputSchema   map[string]any `json:"output_schema,omitempty"`
	SkipEmail      *bool          `json:"skip_email,omitempty"`
}

// PatchScout partially updates an existing Yutori scout (used when an approved
// proposal updates the scout's tracking prompt).
func (c *Client) PatchScout(ctx context.Context, scoutID string, req PatchScoutRequest) error {
	raw, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("scouts: marshal patch scout: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		c.baseURL+"/v1/scouting/tasks/"+scoutID, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("scouts: build patch scout: %w", err)
	}
	body, status, err := c.do(httpReq)
	if err != nil {
		return fmt.Errorf("scouts: patch scout: %w", err)
	}
	if status < 200 || status >= 300 {
		return httpError("scouts: patch scout", status, body)
	}
	return nil
}

// UpdateScoutPrompt is the high-level helper used by the approval flow: it
// PATCHes the scout's query on Yutori so the live scout adopts the new prompt.
func (c *Client) UpdateScoutPrompt(ctx context.Context, scoutID, newPrompt string) error {
	return c.PatchScout(ctx, scoutID, PatchScoutRequest{Query: newPrompt})
}

// DeleteScout permanently deletes a Yutori scouting task, stopping its
// recurring runs (and thus its credit consumption). A 404 is treated as
// success: the scout is already gone on Yutori (e.g. deleted via the
// dashboard), and the caller still wants the local row marked stopped.
func (c *Client) DeleteScout(ctx context.Context, scoutID string) error {
	if c.apiKey == "" {
		return Definitive(fmt.Errorf("scouts: YUTORI_API_KEY is not configured"))
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/v1/scouting/tasks/"+scoutID, nil)
	if err != nil {
		return fmt.Errorf("scouts: build delete scout: %w", err)
	}
	body, status, err := c.do(httpReq)
	if err != nil {
		return fmt.Errorf("scouts: delete scout: %w", err)
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status < 200 || status >= 300 {
		return httpError("scouts: delete scout", status, body)
	}
	return nil
}
