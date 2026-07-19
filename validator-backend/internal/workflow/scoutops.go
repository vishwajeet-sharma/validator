package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	restate "github.com/restatedev/sdk-go"

	"validator-backend/internal/db"
	"validator-backend/internal/models"
	"validator-backend/internal/scouts"
)

// ApprovalInput is sent by the API surface after a human approves a proposal.
// The DB has already been updated; the worker only needs to PATCH Yutori.
type ApprovalInput struct {
	ProposalID    string `json:"proposal_id"`
	ScoutID       string `json:"scout_id"`
	YutoriScoutID string `json:"yutori_scout_id"`
	NewPrompt     string `json:"new_prompt"`
}

// ScoutOps is the Restate service that durably processes inbound Yutori webhook
// updates and applies approved prompt mutations to Yutori.
type ScoutOps struct {
	DB     *db.Store
	Scouts *scouts.Client
}

// ServiceName satisfies the restate reflection contract.
func (s *ScoutOps) ServiceName() string { return ScoutOpsServiceName }

// webhookEnvelope covers the shapes Yutori's "scout" webhook format may emit.
// The canonical shape is NESTED: a top-level {event_type, scout, update, delivery}
// object where the stable scout id lives at scout.id and the structured signals
// live at update.structured_result. The flat fields (scout_id/task_id/id and the
// bare updates array) are retained as fallbacks for alternate/legacy shapes.
type webhookEnvelope struct {
	// Canonical nested "scout" webhook format.
	EventType string          `json:"event_type"`
	Scout     scoutRef        `json:"scout"`
	Update    json.RawMessage `json:"update"`
	Delivery  json.RawMessage `json:"delivery"`

	// Flat / legacy fields (not used by the canonical shape).
	ScoutID          string            `json:"scout_id"`
	TaskID           string            `json:"task_id"`
	ID               string            `json:"id"`
	StructuredResult json.RawMessage   `json:"structured_result"`
	Updates          []json.RawMessage `json:"updates"`
}

// scoutRef is the nested scout identifier block: scout.id is the STABLE scout id
// returned at scout creation (what scouts.current_prompt / DB rows key on). It
// must NOT be confused with update.id, which is a transient per-output id.
type scoutRef struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Query       string `json:"query"`
}

// updateBody is the nested update block. structured_result carries the
// output_schema-shaped signals (when configured); report_content holds the
// human-readable markdown. has_changes=false means nothing new was found.
type updateBody struct {
	ID                string          `json:"id"`
	Status            string          `json:"status"`
	HasChanges        bool            `json:"has_changes"`
	StructuredResult  json.RawMessage `json:"structured_result"`
}

// ProcessWebhook is invoked (via the API surface) with a raw Yutori scout-update
// payload. It durably records the harvested signals and runs an isolated LLM
// evaluation that may open a prompt proposal for the affected scout only.
func (s *ScoutOps) ProcessWebhook(ctx restate.Context, raw json.RawMessage) (restate.Void, error) {
	// Defense-in-depth: a Day 0 research result contains pro_prompt/con_prompt
	// in its structured_result. If one arrives here (e.g. a misrouted webhook
	// from an old task created before path-based routing), drop it instead of
	// running a meaningless mutation eval with empty scout context.
	if looksLikeResearchResult(raw) {
		slog.Warn("scoutops: rejecting Day 0 research result misrouted to ProcessWebhook")
		return restate.Void{}, nil
	}

	scoutID, signals, err := parseWebhookPayload(raw)
	if err != nil {
		slog.Warn("scoutops: ignoring unparseable webhook", "err", err)
		return restate.Void{}, nil // bad payload is not retried forever
	}
	if scoutID == "" {
		slog.Warn("scoutops: webhook missing scout id")
		return restate.Void{}, nil
	}
	if len(signals) == 0 {
		slog.Info("scoutops: webhook carried no signals", "yutori_scout_id", scoutID)
		return restate.Void{}, nil
	}

	// --- Record signals + resolve the owning scout (one DB side-effect) -----
	rec, terr := restate.Run(ctx, func(rctx restate.RunContext) (recordResult, error) {
		scout, err := s.DB.GetScoutByYutoriID(rctx, scoutID)
		if err != nil {
			// Unknown scout: signal the body via the notFound flag rather than
			// returning an error. An unknown-scout webhook is permanent, so it
			// must neither be retried nor (worse) detected after the fact:
			// restate.TerminalError does not preserve the error chain, so the
			// old errors.Is(terr, db.ErrNotFound) check could never fire.
			if errors.Is(err, db.ErrNotFound) {
				return recordResult{NotFound: true}, nil
			}
			return recordResult{}, wrapClosureErr(err)
		}
		if err := s.DB.RecordSignals(rctx, scout.IdeaID, scout.ID, scout.ScoutType, toSignalInputs(signals)); err != nil {
			return recordResult{}, wrapClosureErr(err)
		}
		return recordResult{
			ScoutID:    scout.ID,
			IdeaID:     scout.IdeaID,
			ScoutType:  string(scout.ScoutType),
			Prompt:     scout.CurrentPrompt,
			Status:     scout.Status,
			SignalText: buildSignalsDigest(signals),
		}, nil
	}, boundedRunOpts...)
	if terr != nil {
		return restate.Void{}, terminalf("record signals", terr)
	}
	if rec.NotFound {
		slog.Warn("scoutops: webhook for unknown scout", "yutori_scout_id", scoutID)
		return restate.Void{}, nil
	}

	// A scout already awaiting review must not accumulate a second proposal.
	if rec.Status == models.ScoutStatusPendingMutation {
		slog.Info("scoutops: scout already pending mutation; skipping eval",
			"scout_id", rec.ScoutID)
		return restate.Void{}, nil
	}

	// --- Mutation eval: cheap OpenAI-compatible LLM, never Yutori -----------
	// When no LLM is configured we skip the eval entirely (signals are already
	// recorded) instead of falling back to a paid Yutori research task.
	if !s.Scouts.LLMConfigured() {
		slog.Info("scoutops: skipping mutation eval (LLM not configured)",
			"scout_id", rec.ScoutID)
		return restate.Void{}, nil
	}

	review, terr := restate.Run(ctx, func(rctx restate.RunContext) (scouts.MutationReview, error) {
		rv, err := s.Scouts.ReviewMutation(rctx, rec.ScoutType, rec.Prompt, rec.SignalText)
		return rv, wrapClosureErr(err)
	}, boundedRunOpts...)
	if terr != nil {
		slog.Error("scoutops: mutation review failed", "scout_id", rec.ScoutID, "err", terr)
		return restate.Void{}, nil // do not block signal ingestion on eval failure
	}

	if !review.ShouldExpand || review.ProposedPrompt == "" {
		slog.Info("scoutops: no mutation recommended",
			"scout_id", rec.ScoutID, "type", rec.ScoutType, "reasoning", review.Reasoning)
		return restate.Void{}, nil
	}

	// --- Open a proposal for THIS scout only; the other scout is untouched --
	if terr := restate.RunVoid(ctx, func(rctx restate.RunContext) error {
		_, err := s.DB.CreateProposal(rctx, rec.ScoutID, review.ProposedPrompt)
		return wrapClosureErr(err)
	}, boundedRunOpts...); terr != nil {
		return restate.Void{}, terminalf("create proposal", terr)
	}

	slog.Info("scoutops: prompt proposal opened",
		"scout_id", rec.ScoutID, "type", rec.ScoutType, "reasoning", review.Reasoning)
	return restate.Void{}, nil
}

// ApplyApproval PATCHes a Yutori scout with an approved prompt. The DB has
// already been updated by the API surface; this only syncs Yutori natively.
func (s *ScoutOps) ApplyApproval(ctx restate.Context, input ApprovalInput) (restate.Void, error) {
	if terr := restate.RunVoid(ctx, func(rctx restate.RunContext) error {
		return wrapClosureErr(s.Scouts.UpdateScoutPrompt(rctx, input.YutoriScoutID, input.NewPrompt))
	}, boundedRunOpts...); terr != nil {
		return restate.Void{}, terminalf("patch yutori scout", terr)
	}
	slog.Info("scoutops: yutori scout patched", "scout_id", input.ScoutID, "yutori", input.YutoriScoutID)
	return restate.Void{}, nil
}

// DeleteScoutInput is sent by the API surface when a user stops a scout. The DB
// row has already been marked STOPPED; the worker only needs to delete the
// scout on Yutori (which is what actually halts recurring credit consumption).
type DeleteScoutInput struct {
	ScoutID       string `json:"scout_id"`
	YutoriScoutID string `json:"yutori_scout_id"`
}

// DeleteScout deletes a scout on Yutori. It is idempotent: a 404 (scout already
// gone) is treated as success. The DB was already updated synchronously by the
// API surface.
func (s *ScoutOps) DeleteScout(ctx restate.Context, input DeleteScoutInput) (restate.Void, error) {
	if terr := restate.RunVoid(ctx, func(rctx restate.RunContext) error {
		return wrapClosureErr(s.Scouts.DeleteScout(rctx, input.YutoriScoutID))
	}, boundedRunOpts...); terr != nil {
		return restate.Void{}, terminalf("delete yutori scout", terr)
	}
	slog.Info("scoutops: yutori scout deleted", "scout_id", input.ScoutID, "yutori", input.YutoriScoutID)
	return restate.Void{}, nil
}

// ResolveResearchInput is sent by the API surface when Yutori delivers a
// research-task result via webhook. The awakeable id was embedded in the webhook
// URL (?aw=<id>) by the Day 0 workflow when it created the task.
type ResolveResearchInput struct {
	AwakeableID string          `json:"awakeable_id"`
	Payload     json.RawMessage `json:"payload"`
}

// ResolveResearch resolves a waiting Day 0 research awakeable with the inbound
// Yutori research-task result. On a successful task it resolves the awakeable
// with the raw structured_result (the workflow decodes it against its schema);
// on a failed/empty task it rejects the awakeable so the workflow fails fast
// instead of waiting for the durable timeout to fire.
func (s *ScoutOps) ResolveResearch(ctx restate.Context, input ResolveResearchInput) (restate.Void, error) {
	if input.AwakeableID == "" {
		slog.Warn("scoutops: research webhook missing awakeable id")
		return restate.Void{}, nil
	}
	raw, err := extractStructuredResult(input.Payload)
	if err != nil {
		restate.RejectAwakeable(ctx, input.AwakeableID, err)
		slog.Warn("scoutops: research awakeable rejected",
			"awakeable_id", input.AwakeableID, "err", err)
		return restate.Void{}, nil
	}
	restate.ResolveAwakeable[json.RawMessage](ctx, input.AwakeableID, raw)
	slog.Info("scoutops: research awakeable resolved", "awakeable_id", input.AwakeableID)
	return restate.Void{}, nil
}

// extractStructuredResult pulls the structured_result raw JSON out of a Yutori
// research-task webhook payload (scout format), accepting the same envelope
// shapes as the scouting webhook. An empty/missing result is a Definitive error
// (the task failed or produced no structured output): replaying would not help.
func extractStructuredResult(payload []byte) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(payload))
	if len(trimmed) == 0 {
		return nil, scouts.Definitive(fmt.Errorf("research webhook payload was empty"))
	}

	// Shape A: bare array of envelopes.
	if trimmed[0] == '[' {
		var arr []webhookEnvelope
		if err := json.Unmarshal(payload, &arr); err == nil {
			for _, e := range arr {
				if raw := structuredResultFromEnvelope(e); len(raw) > 0 {
					return raw, nil
				}
			}
		}
		return nil, scouts.Definitive(fmt.Errorf("research webhook array carried no structured_result: %s", truncate(string(payload), 256)))
	}

	// Shape B: single object (envelope or raw update).
	var env webhookEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, scouts.Definitive(fmt.Errorf("decode research webhook: %w", err))
	}
	if raw := structuredResultFromEnvelope(env); len(raw) > 0 {
		return raw, nil
	}
	return nil, scouts.Definitive(fmt.Errorf("research webhook carried no structured_result: %s", truncate(string(payload), 256)))
}

// structuredResultFromEnvelope returns the structured_result from an envelope,
// descending into nested update/updates fields.
func structuredResultFromEnvelope(env webhookEnvelope) json.RawMessage {
	if len(env.StructuredResult) > 0 {
		return env.StructuredResult
	}
	for _, u := range env.Updates {
		var inner webhookEnvelope
		if json.Unmarshal(u, &inner) == nil && len(inner.StructuredResult) > 0 {
			return inner.StructuredResult
		}
	}
	if len(env.Update) > 0 {
		var inner webhookEnvelope
		if json.Unmarshal(env.Update, &inner) == nil && len(inner.StructuredResult) > 0 {
			return inner.StructuredResult
		}
	}
	return nil
}

// recordResult carries the scout context needed across side-effects. All fields
// MUST be exported: this value is the return of a restate.Run closure, so Restate
// journals it via JSON, and encoding/json silently drops unexported fields —
// which previously left every downstream field empty (empty prompt/signals to
// the eval, empty scout_id to CreateProposal -> "invalid input syntax for type
// uuid").
type recordResult struct {
	ScoutID    string
	IdeaID     string
	ScoutType  string
	Prompt     string
	Status     models.ScoutStatus
	SignalText string
	// NotFound is set when no scout matches the inbound yutori_scout_id, so the
	// body can ack-and-drop the webhook instead of failing the handler.
	NotFound bool
}

// parseWebhookPayload flexibly extracts the Yutori scout id and decoded signals
// from any of the common webhook envelope shapes.
func parseWebhookPayload(raw []byte) (yutoriScoutID string, signals []scouts.Signal, err error) {
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) == 0 {
		return "", nil, fmt.Errorf("empty payload")
	}

	// Shape A: bare array of updates.
	if trimmed[0] == '[' {
		var arr []webhookEnvelope
		if jerr := json.Unmarshal(raw, &arr); jerr == nil {
			for _, e := range arr {
				if id, sigs := extractFromEnvelope(e); id != "" && len(sigs) > 0 {
					return id, sigs, nil
				}
			}
		}
		return "", nil, fmt.Errorf("could not extract signals from array payload: %s", truncate(string(raw), 256))
	}

	// Shape B: single object (envelope or raw update).
	var env webhookEnvelope
	if jerr := json.Unmarshal(raw, &env); jerr != nil {
		return "", nil, fmt.Errorf("decode webhook object: %w", jerr)
	}
	id, sigs := extractFromEnvelope(env)
	if id != "" && len(sigs) > 0 {
		return id, sigs, nil
	}
	return "", nil, fmt.Errorf("payload carried no extractable signals: %s", truncate(string(raw), 256))
}

// extractFromEnvelope pulls the scout id + signals from a single envelope.
// Scout id resolution prefers the canonical nested scout.id; only if absent does
// it fall back to the flat scout_id/task_id/id fields or (last resort) the
// transient update.id. Signals are read from update.structured_result (canonical)
// or the flat structured_result (legacy).
func extractFromEnvelope(env webhookEnvelope) (string, []scouts.Signal) {
	// Stable scout id first; transient update.id is only a last-resort fallback.
	id := firstNonEmpty(env.Scout.ID, env.ScoutID, env.TaskID, env.ID)

	// Canonical: signals nested under update.structured_result.
	if len(env.Update) > 0 {
		var u updateBody
		if err := json.Unmarshal(env.Update, &u); err == nil {
			if id == "" {
				id = u.ID // transient fallback only
			}
			if len(u.StructuredResult) > 0 {
				if sigs, err := scouts.DecodeSignals(u.StructuredResult); err == nil && len(sigs) > 0 {
					return id, sigs
				}
			}
		}
	}

	// Legacy flat top-level structured_result.
	if len(env.StructuredResult) > 0 {
		if sigs, err := scouts.DecodeSignals(env.StructuredResult); err == nil && len(sigs) > 0 {
			return id, sigs
		}
	}
	// Legacy bare updates array.
	for _, u := range env.Updates {
		if sid, sigs := extractFromRawUpdate(u, id); sid != "" && len(sigs) > 0 {
			return sid, sigs
		}
	}
	return id, nil
}

func extractFromRawUpdate(raw []byte, fallbackID string) (string, []scouts.Signal) {
	var u webhookEnvelope
	if err := json.Unmarshal(raw, &u); err != nil {
		return fallbackID, nil
	}
	id := firstNonEmpty(u.ScoutID, u.TaskID, u.ID, fallbackID)
	if len(u.StructuredResult) > 0 {
		if sigs, err := scouts.DecodeSignals(u.StructuredResult); err == nil && len(sigs) > 0 {
			return id, sigs
		}
	}
	return id, nil
}

func toSignalInputs(in []scouts.Signal) []db.SignalInput {
	out := make([]db.SignalInput, 0, len(in))
	for _, s := range in {
		out = append(out, db.SignalInput{
			Platform:    s.Platform,
			Quote:       s.Quote,
			Reason:      s.Reason,
			SourceURL:   s.SourceURL,
			SourceTitle: s.SourceTitle,
		})
	}
	return out
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// looksLikeResearchResult checks whether a webhook payload is actually a Day 0
// research result (carries pro_prompt/con_prompt in its structured_result)
// rather than a recurring scout update. This is a defense-in-depth guard: if a
// research webhook is ever misrouted to ProcessWebhook, we drop it instead of
// running a spurious mutation evaluation with empty scout context.
func looksLikeResearchResult(raw []byte) bool {
	var probe struct {
		StructuredResult json.RawMessage `json:"structured_result"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	target := probe.StructuredResult
	if len(target) == 0 {
		target = raw
	}
	var schema struct {
		ProPrompt string `json:"pro_prompt"`
		ConPrompt string `json:"con_prompt"`
	}
	if err := json.Unmarshal(target, &schema); err != nil {
		return false
	}
	return schema.ProPrompt != "" || schema.ConPrompt != ""
}
