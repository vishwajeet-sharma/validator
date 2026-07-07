// Package workflow implements the durable MarketValidationWorkflow using the
// Restate Go SDK, plus the deterministic search-radius mutation engine.
package workflow

import (
	"fmt"
	"time"

	restate "github.com/restatedev/sdk-go"
	"log/slog"

	"validator-backend/internal/db"
	"validator-backend/internal/models"
	"validator-backend/internal/yutori"
)

// ServiceName is the Restate component name the ingress layer targets.
const ServiceName = "MarketValidationWorkflow"

// Restate key-value state keys.
const (
	stateKeyWatchlist = "watchlist"
	stateKeyCycle     = "cycle"
)

// WorkflowInput is the payload sent by the API server when it starts the
// long-running tracking workflow for an idea.
type WorkflowInput struct {
	IdeaID          string   `json:"idea_id"`
	InitialKeywords []string `json:"initial_keywords"`
	IntervalDays    int      `json:"interval_days"`
}

// MarketValidationWorkflow is the durable, infinite tracking lifecycle. It is
// reflected as a Restate workflow component whose single Run handler loops
// forever: scout -> snapshot -> mutate radius -> sleep.
type MarketValidationWorkflow struct {
	DB     *db.Store
	Yutori *yutori.Client
}

// ServiceName satisfies the restate reflection contract.
func (w *MarketValidationWorkflow) ServiceName() string { return ServiceName }

// terminalf wraps an operation error with context while preserving terminal-error
// semantics. This matters because Restate operations (Run, RunVoid, Get, Sleep)
// already return [restate.TerminalError] values: a permanently-failed side
// effect must terminate the workflow rather than be converted into a retryable
// error (which would replay the journal forever). Wrapping with a plain
// fmt.Errorf would lose that terminal status, so we re-stamp it explicitly.
func terminalf(msg string, err error) restate.TerminalError {
	if err == nil {
		return nil
	}
	return restate.TerminalErrorf("%s: %v", msg, err)
}

// Run is the workflow entry point and infinite loop. It is invoked exactly once
// per idea (keyed by idea_id) and durably suspends during each sleep without
// holding memory or engine resources.
func (w *MarketValidationWorkflow) Run(ctx restate.WorkflowContext, input WorkflowInput) (restate.Void, error) {
	// --- Watchlist state initialization (idempotent) -------------------------
	if err := initWatchlist(ctx, input.InitialKeywords); err != nil {
		return restate.Void{}, terminalf("init watchlist", err)
	}
	if err := initCycle(ctx); err != nil {
		return restate.Void{}, terminalf("init cycle", err)
	}

	ctx.Log().Info("market validation workflow started",
		"idea_id", input.IdeaID, "interval_days", input.IntervalDays)

	for {
		// Read the mutable watchlist + cycle from Restate's KV state engine.
		watchlist, err := restate.Get[[]string](ctx, stateKeyWatchlist)
		if err != nil {
			return restate.Void{}, terminalf("get watchlist", err)
		}
		cycle, err := restate.Get[int](ctx, stateKeyCycle)
		if err != nil {
			return restate.Void{}, terminalf("get cycle", err)
		}

		// Fetch fresh idea metadata inside a deterministic side-effect so the
		// title/description/platforms survive suspension/resume cycles.
		idea, terr := restate.Run(ctx, func(rctx restate.RunContext) (*models.Idea, error) {
			return w.DB.GetIdea(rctx, input.IdeaID)
		})
		if terr != nil {
			return restate.Void{}, terminalf("load idea", terr)
		}

		platforms := platformsFor(idea)
		dayNumber := cycle * input.IntervalDays
		label := cycleLabel(cycle, dayNumber)
		ctx.Log().Info("scout cycle starting",
			"idea_id", input.IdeaID, "cycle", cycle, "day", dayNumber,
			"watchlist_size", len(watchlist), "platforms", platforms)

		// --- Parallel Scout execution ---------------------------------------
		// Each scout is a separate Restate side-effect (journal entry) so the
		// two HTTP calls run concurrently and deterministically. restate.Wait
		// resolves them in completion order.
		proFut := restate.RunAsync[[]yutori.Signal](ctx, func(rctx restate.RunContext) ([]yutori.Signal, error) {
			return w.Yutori.Scout(rctx, yutori.ScoutRequest{
				IdeaTitle:       idea.Title,
				IdeaDescription: idea.Description,
				Polarity:        models.PolarityPro,
				Platforms:       platforms,
				Keywords:        watchlist,
			})
		})
		conFut := restate.RunAsync[[]yutori.Signal](ctx, func(rctx restate.RunContext) ([]yutori.Signal, error) {
			return w.Yutori.Scout(rctx, yutori.ScoutRequest{
				IdeaTitle:       idea.Title,
				IdeaDescription: idea.Description,
				Polarity:        models.PolarityCon,
				Platforms:       platforms,
				Keywords:        watchlist,
			})
		})

		var pros, cons []yutori.Signal
		for fut, waitErr := range restate.Wait(ctx, proFut, conFut) {
			if waitErr != nil {
				return restate.Void{}, terminalf("scout wait", waitErr)
			}
			switch fut {
			case proFut:
				sigs, rerr := proFut.Result()
				if rerr != nil {
					return restate.Void{}, terminalf("pro scout", rerr)
				}
				pros = sigs
			case conFut:
				sigs, rerr := conFut.Result()
				if rerr != nil {
					return restate.Void{}, terminalf("con scout", rerr)
				}
				cons = sigs
			}
		}

		// --- Deterministic search-radius mutation ---------------------------
		combined := toSignalInputs(append(append([]yutori.Signal{}, pros...), cons...))
		newKeywords := evaluateRadiusMutation(watchlist, combined)
		status, statusMessage := resolveStatus(newKeywords)

		// --- Database snapshot writes (transactional side-effect) -----------
		proInputs := toSignalInputs(pros)
		conInputs := toSignalInputs(cons)
		if terr := restate.RunVoid(ctx, func(rctx restate.RunContext) error {
			_, err := w.DB.RecordScoutRun(rctx, input.IdeaID, dayNumber, label,
				proInputs, conInputs, status, statusMessage)
			return err
		}); terr != nil {
			return restate.Void{}, terminalf("record scout run", terr)
		}

		// --- Persist evolved watchlist for the next cycle -------------------
		if len(newKeywords) > 0 {
			watchlist = appendUnique(watchlist, newKeywords)
			restate.Set(ctx, stateKeyWatchlist, watchlist)
			slog.Info("search radius expanded",
				"idea_id", input.IdeaID, "added", newKeywords, "watchlist_size", len(watchlist))
		}
		restate.Set(ctx, stateKeyCycle, cycle+1)

		// --- Durable sleep --------------------------------------------------
		// Restate suspends the workflow here, freeing all resources until the
		// timer fires, then transparently resumes execution.
		duration := time.Duration(input.IntervalDays) * 24 * time.Hour
		if err := restate.Sleep(ctx, duration); err != nil {
			return restate.Void{}, terminalf("durable sleep", err)
		}
	}
}

// initWatchlist seeds the watchlist from the initial keywords exactly once.
func initWatchlist(ctx restate.WorkflowContext, initial []string) error {
	existing, err := restate.Get[*[]string](ctx, stateKeyWatchlist)
	if err != nil {
		return err
	}
	if existing == nil {
		if initial == nil {
			initial = []string{}
		}
		restate.Set(ctx, stateKeyWatchlist, initial)
	}
	return nil
}

// initCycle seeds the cycle counter to zero exactly once.
func initCycle(ctx restate.WorkflowContext) error {
	existing, err := restate.Get[*int](ctx, stateKeyCycle)
	if err != nil {
		return err
	}
	if existing == nil {
		restate.Set(ctx, stateKeyCycle, 0)
	}
	return nil
}

// platformsFor derives the active target platforms for an idea, defaulting to a
// sensible set when none are configured.
func platformsFor(idea *models.Idea) []string {
	clean := make([]string, 0, len(idea.Channels))
	for _, c := range idea.Channels {
		if c != "" {
			clean = append(clean, c)
		}
	}
	if len(clean) == 0 {
		return []string{string(models.PlatformReddit), string(models.PlatformYoutube), string(models.PlatformNews)}
	}
	return clean
}

func cycleLabel(cycle, dayNumber int) string {
	if cycle == 0 {
		return "Initial Scan"
	}
	return fmt.Sprintf("Day %d Scan", dayNumber)
}

func resolveStatus(newKeywords []string) (status, statusMessage string) {
	if len(newKeywords) > 0 {
		return "expanded", fmt.Sprintf(
			"Expanded: added %d keyword(s) (%q) based on the latest cycle findings",
			len(newKeywords), newKeywords)
	}
	return "stable", "Stable: no watchlist expansion needed this cycle"
}

// toSignalInputs converts Yutori signals into the DB layer's input shape,
// normalizing the platform value against the known Platform enum.
func toSignalInputs(in []yutori.Signal) []db.SignalInput {
	out := make([]db.SignalInput, 0, len(in))
	for _, s := range in {
		out = append(out, db.SignalInput{
			Platform:    models.Platform(s.Platform),
			Quote:       s.Quote,
			Reason:      s.Reason,
			SourceURL:   s.SourceURL,
			SourceTitle: s.SourceTitle,
		})
	}
	return out
}

// appendUnique concatenates add into base while skipping duplicates (case
// insensitive) so the watchlist never carries redundant terms.
func appendUnique(base, add []string) []string {
	seen := make(map[string]bool, len(base)+len(add))
	for _, k := range base {
		seen[normalizeKeyword(k)] = true
	}
	for _, k := range add {
		nk := normalizeKeyword(k)
		if !seen[nk] {
			seen[nk] = true
			base = append(base, k)
		}
	}
	return base
}
