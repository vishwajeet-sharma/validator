// Package workflow implements the Restate durable components for Validator:
// the Day0SetupWorkflow (one-time initial scout deployment per idea) and the
// ScoutOps service (webhook ingestion + prompt-mutation approval handling).
package workflow

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	restate "github.com/restatedev/sdk-go"
	"github.com/google/uuid"

	"validator-backend/internal/db"
	"validator-backend/internal/models"
	"validator-backend/internal/scouts"
)

// Service name constants targeted by the API ingress client.
const (
	Day0SetupServiceName = "Day0SetupWorkflow"
	ScoutOpsServiceName  = "ScoutOps"
)

// Day0Input is the payload sent by the API when an idea is created.
type Day0Input struct {
	IdeaID          string `json:"idea_id"`
	IdeaTitle       string `json:"idea_title"`
	IdeaDescription string `json:"idea_description"`
	IntervalDays    int    `json:"interval_days"`
}

// Day0SetupWorkflow is the one-time workflow that runs the initial research,
// synthesises PRO/CON tracking prompts, deploys two Yutori scouts, and persists
// them. Keyed by idea id (one workflow per idea).
type Day0SetupWorkflow struct {
	DB         *db.Store
	Scouts     *scouts.Client
	WebhookURL string // public URL Yutori calls back; empty = no webhook
	// ScoutIntervalSeconds overrides the recurring-scout output_interval (for
	// fast local testing). 0 = derive from IntervalDays (production).
	ScoutIntervalSeconds int
}

// ServiceName satisfies the restate reflection contract.
func (w *Day0SetupWorkflow) ServiceName() string { return Day0SetupServiceName }

// terminalf preserves terminal-error semantics when wrapping side-effect errors.
// A permanently-failed side effect must terminate the workflow rather than be
// converted into a retryable error (which would replay the journal forever).
func terminalf(msg string, err error) restate.TerminalError {
	if err == nil {
		return nil
	}
	return restate.TerminalErrorf("%s: %v", msg, err)
}

// boundedRunOpts caps how long any single Run/RunAsync/RunVoid side effect may
// retry its TRANSIENT (non-terminal) failures. Without this, the Restate
// default retries forever — which is what turned a transient Yutori failure
// into an infinite, credit-burning POST loop. Definitive errors are made
// terminal by wrapClosureErr and never reach this retry policy.
var boundedRunOpts = []restate.RunOption{
	restate.WithMaxRetryAttempts(5),
	restate.WithMaxRetryDuration(2 * time.Minute),
}

// wrapClosureErr classifies an error returned by a Run closure so it integrates
// with Restate's retry model:
//   - Definitive errors (scouts.DefinitiveError: parse/business/4xx/"task
//     failed"/timeout) are converted to a restate.TerminalError. Terminal errors
//     are journaled as a final failure and NOT retried, so the side effect is
//     never replayed (which would re-issue the external call that caused it).
//   - Everything else (network blips, 429, 5xx, unclassified) is returned
//     as-is: a non-terminal error, retried under boundedRunOpts.
//
// This MUST be applied INSIDE the closure (on the value returned to Run).
// restate.Run only surfaces an error to the caller once it is terminal or the
// retry policy is exhausted — so wrapping with terminalf AFTER Run returns is
// too late: the closure has already been replayed (and re-POSTed) by then.
func wrapClosureErr(err error) error {
	if err == nil {
		return nil
	}
	if scouts.IsDefinitive(err) {
		return restate.TerminalErrorf("%v", err)
	}
	return err
}

// researchWebhookTimeout caps how long a Day 0 research step will wait for the
// Yutori result webhook to arrive. It is a durable timer (restate.After), so the
// workflow suspends cheaply while waiting — no goroutine or in-closure poll is
// held. If Yutori never calls back (lost webhook / task silently dropped), the
// workflow fails terminally instead of hanging forever.
const researchWebhookTimeout = 15 * time.Minute

// awakeableWebhookURL builds the Yutori webhook URL with the Restate awakeable
// id embedded in the PATH (not as a query param). Yutori's webhook delivery
// strips query parameters, so using ?aw=<id> caused the research completion
// callback to arrive without the awakeable id — routing it to ProcessWebhook
// (mutation eval) instead of ResolveResearch (awakeable resolution). Path
// segments are always preserved, making this correlation reliable.
func (w *Day0SetupWorkflow) awakeableWebhookURL(awakeableID string) string {
	if w.WebhookURL == "" {
		return ""
	}
	return w.WebhookURL + "/research/" + url.PathEscape(awakeableID)
}

// awaitResearch runs a Yutori research task whose structured result is delivered
// by webhook to a freshly-created awakeable — no in-closure polling. Flow:
//  1. Create a Restate awakeable and embed its id in the task's webhook URL.
//  2. Create the task (a short, journaled Run that completes once the POST
//     returns, so a journal replay reuses it instead of re-POSTing).
//  3. Race the awakeable against a durable timeout; whoever fires first wins.
//
// On success it returns the raw structured_result JSON for the caller to decode.
// A task failure (delivered by webhook) rejects the awakeable -> terminal error.
// A lost webhook -> timeout fires -> terminal error. This replaces the old
// create-then-poll-for-8-minutes-inside-a-Run pattern that caused the retry storm.
func (w *Day0SetupWorkflow) awaitResearch(
	ctx restate.WorkflowContext,
	createTask func(rctx restate.RunContext, webhookURL string) (string, error),
	label string,
) (json.RawMessage, error) {
	aw := restate.Awakeable[json.RawMessage](ctx)
	webhookURL := w.awakeableWebhookURL(aw.Id())

	if _, terr := restate.Run(ctx, func(rctx restate.RunContext) (string, error) {
		id, err := createTask(rctx, webhookURL)
		return id, wrapClosureErr(err)
	}, boundedRunOpts...); terr != nil {
		return nil, terminalf(label, terr)
	}

	// Durable race: result webhook vs. lost-webhook timeout.
	winner, werr := restate.WaitFirst(ctx, aw, restate.After(ctx, researchWebhookTimeout))
	if werr != nil {
		return nil, terminalf(label+" wait", werr)
	}
	switch winner {
	case aw:
		raw, rerr := aw.Result()
		if rerr != nil {
			// A rejected awakeable carries the failure reason (e.g. task failed).
			return nil, terminalf(label, rerr)
		}
		return raw, nil
	default:
		// The timer won: the research task did not report back in time.
		return nil, restate.TerminalErrorf("%s: research task did not report back within %s", label, researchWebhookTimeout)
	}
}

// deployResult is the value returned by each parallel scout-deployment branch.
type deployResult struct {
	ScoutID      string
	YutoriScoutID string
	ScoutType    models.ScoutType
}

// Run is the workflow entry point.
func (w *Day0SetupWorkflow) Run(ctx restate.WorkflowContext, input Day0Input) (restate.Void, error) {
	if input.IntervalDays <= 0 {
		input.IntervalDays = 7
	}
	intervalSeconds := input.IntervalDays * 24 * 60 * 60
	// SCOUT_INTERVAL_SECONDS overrides the scout output_interval for testing so
	// recurring scouts deliver data quickly without waiting days. 0 = use days.
	if w.ScoutIntervalSeconds > 0 {
		intervalSeconds = w.ScoutIntervalSeconds
	}

	// Day 0 is webhook-driven: research results arrive via Yutori's webhook into
	// a Restate awakeable. Without a public callback URL there is no way to
	// receive them without falling back to the credit-burning in-closure poll,
	// so we fail fast with a clear, terminal configuration error.
	if w.WebhookURL == "" {
		return restate.Void{}, restate.TerminalErrorf(
			"day 0 requires WEBHOOK_PUBLIC_URL to be set; webhook-driven research cannot run without a public callback URL")
	}

	ctx.Log().Info("day 0 setup started",
		"idea_id", input.IdeaID, "interval_days", input.IntervalDays)

	// --- Step 0: expand the raw idea into a research brief (LLM/Groq) -------
	// Gives the Yutori research task far better grounding than the raw
	// description: concept, target users, value prop, assumptions to validate.
	// Falls back to the raw idea text when no LLM is configured, so Day 0 works
	// without an LLM key. This is a durable side effect (the brief must be
	// stable across journal replays).
	brief := input.IdeaTitle + "\n" + input.IdeaDescription
	if w.Scouts.LLMConfigured() {
		if b, terr := restate.Run(ctx, func(rctx restate.RunContext) (string, error) {
			return w.Scouts.GenerateResearchBrief(rctx, input.IdeaTitle, input.IdeaDescription)
		}, boundedRunOpts...); terr == nil && strings.TrimSpace(b) != "" {
			brief = b
			ctx.Log().Info("day 0 research brief generated",
				"idea_id", input.IdeaID, "len", len(b))
		} else if terr != nil {
			ctx.Log().Warn("day 0 brief generation failed; using raw idea",
				"idea_id", input.IdeaID, "err", terr)
		}
	}

	// --- Step 1: single research task = market signals + PRO/CON prompts ---
	// One Research call both harvests context signals and synthesises the two
	// monitoring prompts, grounded in the full findings (no lossy digest
	// round-trip). The result is delivered by webhook into a Restate awakeable.
	day0Raw, terr := w.awaitResearch(ctx, func(rctx restate.RunContext, webhookURL string) (string, error) {
		id, _, err := w.Scouts.CreateDay0TaskWithWebhook(rctx, brief, webhookURL)
		return id, err
	}, "day 0 research")
	if terr != nil {
		return restate.Void{}, terr
	}
	day0, err := scouts.DecodeDay0Result(day0Raw)
	if err != nil {
		return restate.Void{}, terminalf("day 0 research", err)
	}
	ctx.Log().Info("day 0 research complete",
		"idea_id", input.IdeaID,
		"pro_signals", len(day0.ProSignals), "con_signals", len(day0.ConSignals))

	// --- Step 2: deploy PRO and CON scouts in parallel ---------------------
	proFut := restate.RunAsync[deployResult](ctx, func(rctx restate.RunContext) (deployResult, error) {
		res, err := w.deployScout(rctx, input.IdeaID, models.ScoutTypePro, day0.ProPrompt, intervalSeconds)
		return res, wrapClosureErr(err)
	}, boundedRunOpts...)
	conFut := restate.RunAsync[deployResult](ctx, func(rctx restate.RunContext) (deployResult, error) {
		res, err := w.deployScout(rctx, input.IdeaID, models.ScoutTypeCon, day0.ConPrompt, intervalSeconds)
		return res, wrapClosureErr(err)
	}, boundedRunOpts...)

	var proRes, conRes deployResult
	for fut, waitErr := range restate.Wait(ctx, proFut, conFut) {
		if waitErr != nil {
			return restate.Void{}, terminalf("deploy wait", waitErr)
		}
		switch fut {
		case proFut:
			if r, err := proFut.Result(); err != nil {
				return restate.Void{}, terminalf("deploy pro scout", err)
			} else {
				proRes = r
				ctx.Log().Info("pro scout deployed", "scout_id", r.ScoutID, "yutori", r.YutoriScoutID)
			}
		case conFut:
			if r, err := conFut.Result(); err != nil {
				return restate.Void{}, terminalf("deploy con scout", err)
			} else {
				conRes = r
				ctx.Log().Info("con scout deployed", "scout_id", r.ScoutID, "yutori", r.YutoriScoutID)
			}
		}
	}

	// --- Step 3: record the harvested research signals under their scouts ---
	// Populates the pros/cons tables immediately from the Day 0 research, so the
	// board is not empty until the first recurring scout run. Each side is
	// recorded under its own scout; missing sides are skipped.
	if len(day0.ProSignals) > 0 {
		if terr := restate.RunVoid(ctx, func(rctx restate.RunContext) error {
			return wrapClosureErr(w.DB.RecordSignals(rctx, input.IdeaID, proRes.ScoutID, models.ScoutTypePro, toSignalInputs(day0.ProSignals)))
		}, boundedRunOpts...); terr != nil {
			return restate.Void{}, terminalf("record pro signals", terr)
		}
	}
	if len(day0.ConSignals) > 0 {
		if terr := restate.RunVoid(ctx, func(rctx restate.RunContext) error {
			return wrapClosureErr(w.DB.RecordSignals(rctx, input.IdeaID, conRes.ScoutID, models.ScoutTypeCon, toSignalInputs(day0.ConSignals)))
		}, boundedRunOpts...); terr != nil {
			return restate.Void{}, terminalf("record con signals", terr)
		}
	}
	ctx.Log().Info("day 0 signals recorded",
		"idea_id", input.IdeaID, "pro", len(day0.ProSignals), "con", len(day0.ConSignals))

	// --- Step 4: activate the idea -----------------------------------------
	if terr := restate.RunVoid(ctx, func(rctx restate.RunContext) error {
		return wrapClosureErr(w.DB.ActivateIdea(rctx, input.IdeaID))
	}, boundedRunOpts...); terr != nil {
		return restate.Void{}, terminalf("activate idea", terr)
	}

	ctx.Log().Info("day 0 setup complete", "idea_id", input.IdeaID)
	return restate.Void{}, nil
}

// deployScout creates a Yutori scouting task (email notifications disabled —
// users review findings via the UI) and persists the scout row. It runs as a
// single Restate side-effect.
func (w *Day0SetupWorkflow) deployScout(ctx restate.RunContext, ideaID string, scoutType models.ScoutType, prompt string, intervalSeconds int) (deployResult, error) {
	created, err := w.Scouts.CreateScout(ctx, scouts.CreateScoutRequest{
		Query:          prompt,
		OutputSchema:   scouts.SignalSchema(),
		OutputInterval: intervalSeconds,
		WebhookURL:     w.WebhookURL,
		SkipEmail:      true,
	})
	if err != nil {
		return deployResult{}, err
	}

	scout := &models.Scout{
		ID:            uuid.NewString(),
		IdeaID:        ideaID,
		YutoriScoutID: created.ID,
		ScoutType:     scoutType,
		CurrentPrompt: prompt,
		Status:        models.ScoutStatusActive,
	}
	if err := w.DB.CreateScout(ctx, scout); err != nil {
		return deployResult{}, err
	}

	return deployResult{ScoutID: scout.ID, YutoriScoutID: created.ID, ScoutType: scoutType}, nil
}

// buildSignalsDigest renders harvested signals into a compact text digest that
// the prompt-synthesis LLM block can reason over.
func buildSignalsDigest(signals []scouts.Signal) string {
	if len(signals) == 0 {
		return "No initial signals were harvested; synthesise prompts from the idea alone."
	}
	out := ""
	for i, s := range signals {
		if i >= 12 {
			break
		}
		out += fmt.Sprintf("- [%s] %s (%s)\n", s.Platform, truncate(s.Quote, 200), s.SourceURL)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
