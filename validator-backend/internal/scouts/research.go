package scouts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// researchCreateRequest is the body for POST /v1/research/tasks.
type researchCreateRequest struct {
	Query         string         `json:"query"`
	OutputSchema  map[string]any `json:"output_schema,omitempty"`
	UserTimezone  string         `json:"user_timezone,omitempty"`
	SkipEmail     bool           `json:"skip_email"`
	WebhookURL    string         `json:"webhook_url,omitempty"`
	WebhookFormat string         `json:"webhook_format,omitempty"`
}

type researchCreateResponse struct {
	TaskID    string `json:"task_id"`
	ViewURL   string `json:"view_url"`
	Status    string `json:"status"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

// researchStatusResponse is the body for GET /v1/research/tasks/{task_id}.
type researchStatusResponse struct {
	TaskID                 string          `json:"task_id"`
	Status                 string          `json:"status"`
	Result                 string          `json:"result,omitempty"`
	StructuredResult       json.RawMessage `json:"structured_result,omitempty"`
	StructuredOutputStatus string          `json:"structured_output_status,omitempty"`
	RejectionReason        string          `json:"rejection_reason,omitempty"`
}

// CreateResearchTask creates a one-time Yutori research task and returns its id.
// When webhookURL is non-empty, the task is configured to deliver its structured
// result to that URL (webhook_format "scout") on completion instead of (or in
// addition to, if skip_email is false) emailing. This lets a Restate workflow
// await the result via a webhook/awakeable correlation rather than polling.
func (c *Client) CreateResearchTask(ctx context.Context, query string, outputSchema map[string]any, webhookURL string) (taskID, viewURL string, err error) {
	if c.apiKey == "" {
		return "", "", Definitive(fmt.Errorf("scouts: YUTORI_API_KEY is not configured"))
	}
	body := researchCreateRequest{
		Query:        query,
		OutputSchema: outputSchema,
		UserTimezone: "America/Los_Angeles",
		SkipEmail:    true,
		WebhookURL:   webhookURL,
	}
	if webhookURL != "" {
		body.WebhookFormat = "scout"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", "", fmt.Errorf("scouts: marshal research request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/research/tasks", bytes.NewReader(raw))
	if err != nil {
		return "", "", fmt.Errorf("scouts: build research request: %w", err)
	}
	resBody, status, err := c.do(req)
	if err != nil {
		return "", "", fmt.Errorf("scouts: create research task: %w", err)
	}
	if status < 200 || status >= 300 {
		return "", "", httpError("scouts: create research task", status, resBody)
	}
	var out researchCreateResponse
	if err := json.Unmarshal(resBody, &out); err != nil {
		return "", "", fmt.Errorf("scouts: decode research create response: %w", err)
	}
	if out.TaskID == "" {
		return "", "", Definitive(fmt.Errorf("scouts: research create response missing task_id: %s", truncate(resBody, 256)))
	}
	return out.TaskID, out.ViewURL, nil
}

// CreateDay0TaskWithWebhook creates the single Day 0 research task that BOTH
// harvests market signals AND synthesises the PRO/CON monitoring prompts,
// delivering the combined result to webhookURL on completion. It does not poll —
// the caller (a Restate workflow) awaits the result via the webhook/awakeable
// correlation. The returned taskID is for logging only. `brief` is the research
// brief (ideally LLM-expanded) that grounds the research; pass the raw idea text
// when no LLM is available.
func (c *Client) CreateDay0TaskWithWebhook(ctx context.Context, brief, webhookURL string) (taskID, viewURL string, err error) {
	return c.CreateResearchTask(ctx, day0Query(brief), Day0Schema(), webhookURL)
}

// GenerateResearchBrief expands a raw product idea (title + description) into a
// crisp, information-dense research brief that the Day 0 Yutori research task
// runs against. The brief articulates the concept, target users, value
// proposition, key assumptions, and the market questions worth validating — far
// better grounding than the raw description alone. It uses the OpenAI-compatible
// LLM (Groq). Returns ErrLLMNotConfigured when no LLM is wired; the caller falls
// back to the raw idea so Day 0 still works without an LLM key.
func (c *Client) GenerateResearchBrief(ctx context.Context, ideaTitle, ideaDescription string) (string, error) {
	if !c.LLMConfigured() {
		return "", ErrLLMNotConfigured
	}
	system := "You are a sharp product analyst. Given a raw product idea, write a tight, " +
		"information-dense research brief that a web research agent will use to validate the market. " +
		"Articulate: what the product is and does, the target users and their pain, the core value " +
		"proposition and differentiation, the key assumptions that must hold, and the most important " +
		"market questions (demand, willingness to pay, competition, alternatives, differentiation, " +
		"regulatory/technical risk). Be specific and concrete; do NOT invent facts or invent sources. " +
		"Output only the brief text, no preamble, no headings, no bullet lists."
	user := fmt.Sprintf("TITLE:\n%s\n\nRAW DESCRIPTION:\n%s\n\nWrite the research brief.",
		ideaTitle, ideaDescription)
	return c.llm.Complete(ctx, system, user)
}

// DecodeDay0Result parses the combined Day 0 result, validating that both
// monitoring prompts are present. Missing prompts are a Definitive (non-
// retryable) failure: re-running the same task would likely omit them again.
// Harvested signals are split by polarity and sanitized (those missing a
// quote/source_url are dropped).
func DecodeDay0Result(raw json.RawMessage) (Day0Result, error) {
	var r Day0Result
	if err := json.Unmarshal(raw, &r); err != nil {
		return Day0Result{}, Definitive(fmt.Errorf("scouts: decode day 0 result: %w", err))
	}
	if r.ProPrompt == "" || r.ConPrompt == "" {
		return Day0Result{}, Definitive(fmt.Errorf("scouts: day 0 result missing prompts: %s", truncate(raw, 256)))
	}
	r.ProSignals = sanitize(r.ProSignals)
	r.ConSignals = sanitize(r.ConSignals)
	return r, nil
}

// day0Query renders the single Day 0 directive: research the market for real,
// sourced signals, SPLIT them into demand (pro_signals) and threat (con_signals)
// findings, then design two distinct monitoring queries (PRO demand + CON
// threats), all grounded in the (ideally LLM-expanded) research brief.
func day0Query(brief string) string {
	return fmt.Sprintf(
		"Research the market for the product idea below and design two monitoring scouts.\n\n"+
			"FIRST, surface real, verifiable signals across the web: customer demand and unmet needs, "+
			"competitors and alternatives, recent news and announcements, user sentiment, and market size "+
			"trends. For each finding return a direct quote and a real source URL. Classify every "+
			"finding into pro_signals (demand-side: customer pain, willingness to pay, unmet needs, "+
			"positive traction, adoption, market growth) or con_signals (threat-side: competitors, "+
			"saturation, regulatory risk, switching costs, user resistance, negative sentiment).\n\n"+
			"THEN, grounded in what you found, write TWO distinct, high-quality monitoring queries in "+
			"natural language that a web-scouting agent will run on a recurring schedule:\n"+
			"1) A PRO scout query that surfaces DEMAND signals: customer pain, willingness to pay, "+
			"unmet needs, positive traction, adoption, market growth.\n"+
			"2) A CON scout query that surfaces THREAT signals: competitors, saturation, regulatory risk, "+
			"switching costs, user resistance, negative sentiment.\n"+
			"Each query must be specific, name concrete things to watch for, and instruct the scout to "+
			"return findings with real source URLs. Do NOT mention JSON or output formatting.\n\n"+
			"PRODUCT BRIEF (authoritative description of the idea):\n%s",
		brief)
}

// GetResearchTask returns the current status + structured result of a task.
func (c *Client) GetResearchTask(ctx context.Context, taskID string) (status string, structured json.RawMessage, resultMD string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/research/tasks/"+taskID, nil)
	if err != nil {
		return "", nil, "", fmt.Errorf("scouts: build research status request: %w", err)
	}
	body, code, err := c.do(req)
	if err != nil {
		return "", nil, "", fmt.Errorf("scouts: research status request: %w", err)
	}
	if code < 200 || code >= 300 {
		return "", nil, "", httpError("scouts: research status", code, body)
	}
	var out researchStatusResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", nil, "", fmt.Errorf("scouts: decode research status: %w", err)
	}
	return out.Status, out.StructuredResult, out.Result, nil
}

// Research creates a research task and polls it to completion, returning the raw
// structured_result payload. The caller decodes it against its own schema.
//
// This is the polling path, used only for the webhook-path's mutation eval
// (ReviewMutation), whose result is awaited inline. The Day 0 setup flow does
// NOT use this — it awaits results via webhook/awakeable to avoid holding a
// long poll open inside a Run side-effect.
func (c *Client) Research(ctx context.Context, query string, outputSchema map[string]any) (json.RawMessage, error) {
	taskID, _, err := c.CreateResearchTask(ctx, query, outputSchema, "")
	if err != nil {
		return nil, err
	}
	slog.Info("scouts: research task created", "task_id", taskID)

	deadline := time.Now().Add(researchPollTimeout)
	for {
		status, structured, resultMD, err := c.GetResearchTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		switch status {
		case "succeeded":
			if len(structured) == 0 {
				return nil, Definitive(fmt.Errorf("scouts: research task %s succeeded but produced no structured result (markdown: %s)",
					taskID, truncate([]byte(resultMD), 256)))
			}
			slog.Info("scouts: research task completed", "task_id", taskID)
			return structured, nil
		case "failed":
			return nil, Definitive(fmt.Errorf("scouts: research task %s failed", taskID))
		}
		if time.Now().After(deadline) {
			return nil, Definitive(fmt.Errorf("scouts: research task %s timed out after %s", taskID, researchPollTimeout))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(researchPollInterval):
		}
	}
}

// ReviewMutation decides whether a scout's prompt should expand given the latest
// harvested signals. It runs against the configured OpenAI-compatible LLM — NOT
// the Yutori Research API — so evals are cheap and never burn Yutori credits. If
// no LLM is configured it returns ErrLLMNotConfigured and the caller skips the
// eval (signals are still recorded). Its failures are swallowed by the caller.
func (c *Client) ReviewMutation(ctx context.Context, scoutType, currentPrompt, signalsDigest string) (MutationReview, error) {
	if !c.LLMConfigured() {
		return MutationReview{}, ErrLLMNotConfigured
	}
	polarityFocus := "demand, willingness to pay, unmet needs, positive traction"
	if scoutType == "CON" {
		polarityFocus = "competition, saturation, regulatory risk, user resistance"
	}
	system := "You evaluate whether a market-monitoring scout should expand its search radius. " +
		"Respond with ONLY a JSON object, no prose, matching exactly: " +
		`{"should_expand": <bool>, "proposed_prompt": <string>, "reasoning": <string>}.`
	user := fmt.Sprintf(
		"You are evaluating whether a %s market-monitoring scout should expand its search radius.\n"+
			"Given the scout's current tracking prompt and the latest harvested signals, decide whether "+
			"the prompt should be broadened to capture emerging %s.\n"+
			"If yes, set should_expand=true and provide a FULL revised prompt (the current prompt improved "+
			"with the new angle, ready to deploy) in proposed_prompt. If no expansion is warranted, set "+
			"should_expand=false and leave proposed_prompt empty.\n\n"+
			"CURRENT TRACKING PROMPT:\n%s\n\nLATEST SIGNALS:\n%s",
		scoutType, polarityFocus, currentPrompt, signalsDigest)

	content, err := c.llm.Complete(ctx, system, user)
	if err != nil {
		return MutationReview{}, fmt.Errorf("scouts: mutation eval llm: %w", err)
	}
	raw := extractJSONObject(content)
	var review MutationReview
	if err := json.Unmarshal(raw, &review); err != nil {
		return MutationReview{}, Definitive(fmt.Errorf("scouts: decode mutation review: %w (content: %s)", err, truncate([]byte(content), 256)))
	}
	return review, nil
}

// extractJSONObject pulls the first {...} JSON object out of an LLM response,
// tolerating code fences or surrounding prose.
func extractJSONObject(s string) json.RawMessage {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", -1)
		// drop leading fence line
		if len(lines) > 0 {
			lines = lines[1:]
		}
		// drop trailing fence line
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		s = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return json.RawMessage(s[start : end+1])
	}
	return json.RawMessage(s)
}
