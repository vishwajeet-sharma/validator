// Package api implements the public REST ingress layer for the Validator
// platform: idea creation (which also kicks off the durable workflow) and the
// compiled LLM-payload endpoint.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	restateingress "github.com/restatedev/sdk-go/ingress"

	"validator-backend/internal/db"
	"validator-backend/internal/models"
	"validator-backend/internal/workflow"
)

// Server wires together the database store and the Restate ingress client used
// to trigger the MarketValidationWorkflow.
type Server struct {
	DB            *db.Store
	IngressClient *restateingress.Client
}

// NewServer returns a configured Server.
func NewServer(store *db.Store, ingress *restateingress.Client) *Server {
	return &Server{DB: store, IngressClient: ingress}
}

// Routes returns an http.Handler with all REST endpoints registered.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ideas", s.handleListIdeas)
	mux.HandleFunc("POST /api/ideas", s.handlePostIdea)
	mux.HandleFunc("GET /api/ideas/{id}", s.handleGetIdea)
	mux.HandleFunc("GET /api/ideas/{id}/payload", s.handleGetPayload)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return s.withCORS(s.withLogging(mux))
}

// createIdeaRequest is the JSON body for POST /api/ideas. Field names mirror the
// UI's NewIdeaForm (camelCase); unknown fields are ignored for forward-compat.
type createIdeaRequest struct {
	Title                 string                 `json:"title"`
	Description           string                 `json:"description"`
	ScoutingFrequencyDays int                    `json:"scoutingFrequencyDays"`
	Keywords              []string               `json:"keywords"`
	Channels              []string               `json:"channels"`
	CustomChannels        []models.CustomChannel `json:"customChannels"`
}

// createIdeaResponse is returned on successful idea creation.
type createIdeaResponse struct {
	Idea       IdeaDTO `json:"idea"`
	WorkflowID string  `json:"workflow_id"`
	Invocation string  `json:"invocation_id,omitempty"`
}

// handleListIdeas returns every tracked idea with its embedded scout cycles.
func (s *Server) handleListIdeas(w http.ResponseWriter, r *http.Request) {
	ideas, err := s.DB.ListIdeas(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list ideas: %v", err)
		return
	}

	ids := make([]string, 0, len(ideas))
	for _, i := range ideas {
		ids = append(ids, i.ID)
	}
	runsByIdea, err := s.DB.GetScoutRunsByIdeaIDs(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load scout runs: %v", err)
		return
	}
	signalsByIdea, err := s.DB.GetSignalsByIdeaIDs(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load signals: %v", err)
		return
	}

	out := make([]IdeaDTO, 0, len(ideas))
	for _, idea := range ideas {
		out = append(out, BuildIdeaDTO(idea, runsByIdea[idea.ID], signalsByIdea[idea.ID]))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetIdea returns a single idea with all of its scout cycles assembled.
func (s *Server) handleGetIdea(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing idea id")
		return
	}

	idea, err := s.DB.GetIdea(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "idea not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "load idea: %v", err)
		return
	}

	runsByIdea, err := s.DB.GetScoutRunsByIdeaIDs(r.Context(), []string{id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load scout runs: %v", err)
		return
	}
	signalsByIdea, err := s.DB.GetSignalsByIdeaIDs(r.Context(), []string{id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load signals: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, BuildIdeaDTO(idea, runsByIdea[id], signalsByIdea[id]))
}

// handlePostIdea persists a new idea and asynchronously starts the durable
// MarketValidationWorkflow via the Restate ingress.
func (s *Server) handlePostIdea(w http.ResponseWriter, r *http.Request) {
	var req createIdeaRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Description) == "" {
		writeError(w, http.StatusBadRequest, "title and description are required")
		return
	}
	if len(req.Keywords) == 0 {
		writeError(w, http.StatusBadRequest, "at least one keyword is required")
		return
	}
	if req.ScoutingFrequencyDays <= 0 {
		req.ScoutingFrequencyDays = 7
	}
	if len(req.Channels) == 0 {
		req.Channels = []string{
			string(models.PlatformReddit),
			string(models.PlatformYoutube),
			string(models.PlatformNews),
		}
	}

	idea := &models.Idea{
		ID:             uuid.NewString(),
		Title:          strings.TrimSpace(req.Title),
		Description:    strings.TrimSpace(req.Description),
		FrequencyDays:  req.ScoutingFrequencyDays,
		Keywords:       req.Keywords,
		Channels:       req.Channels,
		CustomChannels: req.CustomChannels,
		Status:         "pending",
		StatusMessage:  "Initial scout run pending",
	}

	if err := s.DB.CreateIdea(r.Context(), idea); err != nil {
		writeError(w, http.StatusInternalServerError, "persist idea: %v", err)
		return
	}

	// Asynchronously trigger the long-running workflow keyed by idea id.
	invocationID := s.triggerWorkflow(r.Context(), idea)

	writeJSON(w, http.StatusCreated, createIdeaResponse{
		Idea:       BuildIdeaDTO(idea, nil, nil),
		WorkflowID: idea.ID,
		Invocation: invocationID,
	})
}

// triggerWorkflow fires-and-forgets the MarketValidationWorkflow via the Restate
// ingress. The workflow id mirrors the idea id so each idea is tracked by
// exactly one workflow instance. Trigger failures are logged but never fail the
// HTTP request, since the idea has already been persisted and can be retried.
func (s *Server) triggerWorkflow(ctx context.Context, idea *models.Idea) string {
	if s.IngressClient == nil {
		slog.Warn("ingress client not configured; workflow not started", "idea_id", idea.ID)
		return ""
	}

	input := &workflow.WorkflowInput{
		IdeaID:          idea.ID,
		InitialKeywords: idea.Keywords,
		IntervalDays:    idea.FrequencyDays,
	}

	resp, err := restateingress.WorkflowSend[*workflow.WorkflowInput](
		s.IngressClient, workflow.ServiceName, idea.ID, "Run",
	).Send(ctx, input)
	if err != nil {
		slog.Error("failed to start validation workflow",
			"idea_id", idea.ID, "err", err)
		return ""
	}

	slog.Info("validation workflow started",
		"idea_id", idea.ID, "invocation_id", resp.Id())
	return resp.Id()
}

// handleGetPayload compiles the latest scout cycle for an idea into the clean
// markdown "Copy to LLM" payload.
func (s *Server) handleGetPayload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing idea id")
		return
	}

	idea, err := s.DB.GetIdea(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "idea not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "load idea: %v", err)
		return
	}

	run, err := s.DB.GetLatestScoutRun(r.Context(), id)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "load latest scout run: %v", err)
		return
	}

	var signals []models.MarketSignal
	if run != nil {
		signals, err = s.DB.GetSignalsByRun(r.Context(), run.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load signals: %v", err)
			return
		}
	}

	payload := BuildLLMPayload(idea, run, signals)
	slog.Info("payload generated", "idea_id", id, "signals", len(signals))

	writeJSON(w, http.StatusOK, map[string]string{"payload": payload})
}

// withLogging wraps the handler with minimal request logging.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		slog.Info("http request",
			"method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
	})
}

// withCORS adds permissive CORS headers so the browser-served UI (e.g. the Vite
// dev server on another origin) can call the API directly. Preflight OPTIONS
// requests are short-circuited with a 204.
func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		h.Set("Access-Control-Max-Age", "600")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- encoding / response helpers -------------------------------------------

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	// Unknown fields are intentionally allowed so the API stays tolerant of
	// additive changes to the client payload.
	defer r.Body.Close() //nolint:errcheck
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, format string, args ...any) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(msg)})
}
