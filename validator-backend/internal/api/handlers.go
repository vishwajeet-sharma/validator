package api

import (
	"io"
	"log/slog"
	"net/http"
	"strings"

	"validator-backend/internal/models"
	"validator-backend/internal/workflow"
)

// createIdeaRequest is the JSON body for POST /api/ideas.
type createIdeaRequest struct {
	Title                 *string `json:"title,omitempty"`
	Description           string  `json:"description"`
	ScoutingFrequencyDays int     `json:"scoutingFrequencyDays"`
}

// handleListIdeas returns all tracked ideas with their scout statuses.
func (s *Server) handleListIdeas(w http.ResponseWriter, r *http.Request) {
	ideas, err := s.DB.ListIdeas(r.Context())
	if err != nil {
		slog.Error("list ideas failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load ideas")
		return
	}
	ids := make([]string, 0, len(ideas))
	for _, i := range ideas {
		ids = append(ids, i.ID)
	}
	scoutsByIdea, err := s.DB.GetScoutsByIdeaIDs(r.Context(), ids)
	if err != nil {
		slog.Error("load scouts failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load scouts")
		return
	}

	out := make([]IdeaSummaryDTO, 0, len(ideas))
	for _, idea := range ideas {
		scouts := scoutsByIdea[idea.ID]
		out = append(out, IdeaSummaryDTO{
			ID:                    idea.ID,
			Title:                 idea.Title,
			Description:           idea.Description,
			ScoutingFrequencyDays: idea.FrequencyDays,
			Status:                string(idea.Status),
			TotalPros:             idea.TotalPros,
			TotalCons:             idea.TotalCons,
			ProScoutStatus:        scoutStatusFor(scouts, models.ScoutTypePro),
			ConScoutStatus:        scoutStatusFor(scouts, models.ScoutTypeCon),
			CreatedAt:             rfc3339(idea.CreatedAt),
			LastUpdated:           rfc3339(idea.UpdatedAt),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetIdea returns a single idea with its scouts and recent findings.
func (s *Server) handleGetIdea(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing idea id")
		return
	}
	idea, err := s.DB.GetIdea(r.Context(), id)
	if err != nil {
		if notFound(err) {
			writeError(w, http.StatusNotFound, "idea not found")
			return
		}
		slog.Error("get idea failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load idea")
		return
	}
	scouts, err := s.DB.GetScoutsByIdea(r.Context(), id)
	if err != nil {
		slog.Error("get scouts failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load scouts")
		return
	}
	signals, err := s.DB.GetSignalsByIdea(r.Context(), id)
	if err != nil {
		slog.Error("get signals failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load signals")
		return
	}

	scoutIDs := make([]string, 0, len(scouts))
	for _, sc := range scouts {
		scoutIDs = append(scoutIDs, sc.ID)
	}
	proposals, err := s.DB.GetPendingProposalsByScoutIDs(r.Context(), scoutIDs)
	if err != nil {
		slog.Error("get proposals failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load proposals")
		return
	}

	pros, cons := splitSignals(signals, 20)
	detail := IdeaDetailDTO{
		IdeaSummaryDTO: IdeaSummaryDTO{
			ID:                    idea.ID,
			Title:                 idea.Title,
			Description:           idea.Description,
			ScoutingFrequencyDays: idea.FrequencyDays,
			Status:                string(idea.Status),
			TotalPros:             idea.TotalPros,
			TotalCons:             idea.TotalCons,
			ProScoutStatus:        scoutStatusFor(scouts, models.ScoutTypePro),
			ConScoutStatus:        scoutStatusFor(scouts, models.ScoutTypeCon),
			CreatedAt:             rfc3339(idea.CreatedAt),
			LastUpdated:           rfc3339(idea.UpdatedAt),
		},
		Scouts:        scoutsForDetail(scouts, proposals),
		RecentPros:    pros,
		RecentCons:    cons,
		RefinedPrompt: idea.RefinedPrompt,
	}
	writeJSON(w, http.StatusOK, detail)
}

// proposalResponse is the JSON body for POST /api/proposals/{id}/respond.
type proposalResponse struct {
	Action     string  `json:"action"`
	EditedText *string `json:"edited_text,omitempty"`
}

// handleRespondProposal applies a human APPROVE/REJECT decision. On approve it
// updates the scout prompt in the DB and forwards a Yutori PATCH to the worker.
func (s *Server) handleRespondProposal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing proposal id")
		return
	}
	var req proposalResponse
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	action := strings.ToUpper(strings.TrimSpace(req.Action))
	if action != "APPROVE" && action != "REJECT" {
		writeError(w, http.StatusBadRequest, "action must be APPROVE or REJECT")
		return
	}

	proposal, err := s.DB.GetProposal(r.Context(), id)
	if err != nil {
		if notFound(err) {
			writeError(w, http.StatusNotFound, "proposal not found")
			return
		}
		slog.Error("get proposal failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load proposal")
		return
	}
	if proposal.Status != models.ProposalPending {
		writeError(w, http.StatusConflict, "proposal has already been resolved")
		return
	}

	if action == "REJECT" {
		if err := s.DB.ResolveProposal(r.Context(), id, models.ProposalRejected); err != nil {
			slog.Error("reject proposal failed", "err", err)
			writeError(w, http.StatusInternalServerError, "could not reject proposal")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "REJECTED"})
		return
	}

	// APPROVE: choose the final prompt text.
	newPrompt := proposal.ProposedPrompt
	if req.EditedText != nil && strings.TrimSpace(*req.EditedText) != "" {
		newPrompt = strings.TrimSpace(*req.EditedText)
	}

	scout, err := s.DB.GetScout(r.Context(), proposal.ScoutID)
	if err != nil {
		slog.Error("get scout for proposal failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load scout")
		return
	}

	if err := s.DB.UpdateScoutPrompt(r.Context(), scout.ID, newPrompt); err != nil {
		slog.Error("update scout prompt failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not update scout prompt")
		return
	}
	if err := s.DB.ResolveProposal(r.Context(), id, models.ProposalApproved); err != nil {
		slog.Error("approve proposal failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not approve proposal")
		return
	}

	// Forward the Yutori PATCH to the worker (fire-and-forget).
	_ = s.applyApproval(workflow.ApprovalInput{
		ProposalID:    id,
		ScoutID:       scout.ID,
		YutoriScoutID: scout.YutoriScoutID,
		NewPrompt:     newPrompt,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "APPROVED"})
}

// handleDeleteScout stops a scout: marks it STOPPED in the DB synchronously,
// then fire-and-forgets a worker call to DELETE it on Yutori (which is what
// actually halts recurring credit consumption). The scout's pending proposals,
// if any, are resolved as rejected since they can no longer be applied.
func (s *Server) handleDeleteScout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing scout id")
		return
	}
	scout, err := s.DB.GetScout(r.Context(), id)
	if err != nil {
		if notFound(err) {
			writeError(w, http.StatusNotFound, "scout not found")
			return
		}
		slog.Error("get scout failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load scout")
		return
	}
	if scout.Status == models.ScoutStatusStopped {
		writeJSON(w, http.StatusOK, map[string]string{"status": "STOPPED"})
		return
	}

	if err := s.DB.SetScoutStatus(r.Context(), id, models.ScoutStatusStopped); err != nil {
		slog.Error("stop scout failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not stop scout")
		return
	}

	// A stopped scout's pending proposals can never be applied (the underlying
	// Yutori scout is gone). Resolve them as rejected so they don't linger as
	// unreachable review items in the UI. Best-effort: a failure here doesn't
	// un-stop the scout.
	if err := s.DB.RejectPendingProposals(r.Context(), id); err != nil {
		slog.Warn("reject pending proposals on stop failed", "scout_id", id, "err", err)
	}

	// Forward the Yutori delete to the worker (fire-and-forget). The DB is
	// already stopped, so a delayed/failed Yutori delete does not leave the
	// user able to act on the scout; the worker retries under boundedRunOpts.
	_ = s.stopScout(workflow.DeleteScoutInput{
		ScoutID:       scout.ID,
		YutoriScoutID: scout.YutoriScoutID,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "STOPPED"})
}

// handleDeactivateIdea deactivates an idea: marks it INACTIVE, stops both its
// scouts, and rejects their pending proposals (all in one DB transaction), then
// fire-and-forgets worker calls to delete the scouts on Yutori (which is what
// actually halts recurring credit usage). Existing findings are kept. Idempotent
// for an already-INACTIVE idea.
func (s *Server) handleDeactivateIdea(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing idea id")
		return
	}
	idea, err := s.DB.GetIdea(r.Context(), id)
	if err != nil {
		if notFound(err) {
			writeError(w, http.StatusNotFound, "idea not found")
			return
		}
		slog.Error("get idea failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load idea")
		return
	}
	if idea.Status == models.IdeaStatusInactive {
		writeJSON(w, http.StatusOK, map[string]string{"status": "INACTIVE"})
		return
	}

	// Load scouts BEFORE deactivating so we know which Yutori scouts to delete.
	scouts, err := s.DB.GetScoutsByIdea(r.Context(), id)
	if err != nil {
		slog.Error("get scouts for deactivate failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load scouts")
		return
	}

	if err := s.DB.DeactivateIdea(r.Context(), id); err != nil {
		slog.Error("deactivate idea failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not deactivate idea")
		return
	}

	// Fire-and-forget a Yutori delete per still-live scout. STOPPED scouts were
	// already deleted on Yutori when they were stopped.
	for _, sc := range scouts {
		if sc.Status == models.ScoutStatusStopped {
			continue
		}
		_ = s.stopScout(workflow.DeleteScoutInput{
			ScoutID:       sc.ID,
			YutoriScoutID: sc.YutoriScoutID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "INACTIVE"})
}

// handleResearchWebhook receives a Day 0 research-task completion callback.
// The awakeable id is embedded in the URL PATH (/api/webhooks/yutori/research/{awakeableID}),
// not as a query param — Yutori strips query params, so path-based routing is
// the only reliable way to correlate the callback with the waiting workflow.
// Forwards to ScoutOps.ResolveResearch which resolves (or rejects) the awakeable.
func (s *Server) handleResearchWebhook(w http.ResponseWriter, r *http.Request) {
	awakeableID := r.PathValue("awakeableID")
	if awakeableID == "" {
		writeError(w, http.StatusBadRequest, "missing awakeable id in path")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read webhook body")
		return
	}
	if len(raw) == 0 {
		writeError(w, http.StatusBadRequest, "empty webhook body")
		return
	}
	if err := s.resolveResearch(awakeableID, raw); err != nil {
		slog.Error("research webhook forward failed", "err", err)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// handleScoutWebhook receives a recurring scouting update from a deployed
// Yutori scout. Forwards to ScoutOps.ProcessWebhook for durable signal
// ingestion + mutation evaluation. A defensive query-param fallback is kept
// for any in-flight research task created before the path-based routing change.
func (s *Server) handleScoutWebhook(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read webhook body")
		return
	}
	if len(raw) == 0 {
		writeError(w, http.StatusBadRequest, "empty webhook body")
		return
	}

	// Fallback: old research tasks may still carry ?aw=<id> from before the
	// path-based routing change. Route them to ResolveResearch instead of
	// ProcessWebhook so they don't trigger a spurious mutation eval.
	if awakeableID := r.URL.Query().Get("aw"); awakeableID != "" {
		if err := s.resolveResearch(awakeableID, raw); err != nil {
			slog.Error("research webhook forward failed (query-param fallback)", "err", err)
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
		return
	}

	if err := s.forwardWebhook(raw); err != nil {
		slog.Error("webhook forward failed", "err", err)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}
