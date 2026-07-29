// Package api implements the public REST ingress layer for the Validator
// platform: idea onboarding (which kicks off the Day 0 workflow), human-in-the-
// loop proposal responses, and the Yutori webhook forwarder.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	restateingress "github.com/restatedev/sdk-go/ingress"

	"validator-backend/internal/db"
	"validator-backend/internal/workflow"
)

// Server wires together the database store and the Restate ingress client used
// to trigger the Day 0 workflow and forward webhooks to the worker.
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
	mux.HandleFunc("POST /api/proposals/{id}/respond", s.handleRespondProposal)
	mux.HandleFunc("DELETE /api/scouts/{id}", s.handleDeleteScout)
	mux.HandleFunc("DELETE /api/ideas/{id}", s.handleDeactivateIdea)
	mux.HandleFunc("POST /api/webhooks/yutori/research/{awakeableID}", s.handleResearchWebhook)
	mux.HandleFunc("POST /api/webhooks/yutori", s.handleScoutWebhook)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return s.withCORS(s.withLogging(mux))
}

// withLogging wraps the handler with minimal request logging.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
	})
}

// withCORS adds permissive CORS headers so the browser-served UI can call the
// API directly. Preflight OPTIONS are short-circuited with a 204.
func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
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
	defer r.Body.Close() //nolint:errcheck
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// rfc3339 formats a time for JSON output.
func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// notFound reports whether err is a db.ErrNotFound.
func notFound(err error) bool { return errors.Is(err, db.ErrNotFound) }

// triggerDay0 fire-and-forgets the Day 0 setup workflow for an idea.
func (s *Server) triggerDay0(ideaID, title, description string, freqDays int) error {
	if s.IngressClient == nil {
		slog.Warn("ingress client not configured; day 0 not started", "idea_id", ideaID)
		return nil
	}
	input := &workflow.Day0Input{
		IdeaID:          ideaID,
		IdeaTitle:       title,
		IdeaDescription: description,
		IntervalDays:    freqDays,
	}
	resp, err := restateingress.WorkflowSend[*workflow.Day0Input](
		s.IngressClient, workflow.Day0SetupServiceName, ideaID, "Run").Send(context.Background(), input)
	if err != nil {
		slog.Error("failed to start day 0 workflow", "idea_id", ideaID, "err", err)
		return err
	}
	slog.Info("day 0 workflow started", "idea_id", ideaID, "invocation_id", resp.Id())
	return nil
}

// forwardWebhook fire-and-forwards a raw Yutori webhook payload to the worker's
// ScoutOps.ProcessWebhook handler for durable processing.
func (s *Server) forwardWebhook(raw []byte) error {
	if s.IngressClient == nil {
		slog.Warn("ingress client not configured; webhook dropped")
		return nil
	}
	resp, err := restateingress.ServiceSend[json.RawMessage](
		s.IngressClient, workflow.ScoutOpsServiceName, "ProcessWebhook").Send(context.Background(), raw)
	if err != nil {
		slog.Error("failed to forward webhook", "err", err)
		return err
	}
	slog.Info("webhook forwarded", "invocation_id", resp.Id())
	return nil
}

// resolveResearch fire-and-forwards a Yutori research-task result to the worker's
// ScoutOps.ResolveResearch handler, which resolves (or rejects) the Day 0
// workflow's waiting awakeable. The awakeable id travelled in the webhook URL.
func (s *Server) resolveResearch(awakeableID string, payload []byte) error {
	if s.IngressClient == nil {
		slog.Warn("ingress client not configured; research webhook dropped")
		return nil
	}
	input := &workflow.ResolveResearchInput{
		AwakeableID: awakeableID,
		Payload:     payload,
	}
	resp, err := restateingress.ServiceSend[*workflow.ResolveResearchInput](
		s.IngressClient, workflow.ScoutOpsServiceName, "ResolveResearch").Send(context.Background(), input)
	if err != nil {
		slog.Error("failed to forward research webhook", "err", err)
		return err
	}
	slog.Info("research webhook forwarded", "invocation_id", resp.Id(), "awakeable_id", awakeableID)
	return nil
}

// applyApproval fire-and-forwards an approved prompt update to the worker so it
// can PATCH Yutori natively.
func (s *Server) applyApproval(in workflow.ApprovalInput) error {
	if s.IngressClient == nil {
		slog.Warn("ingress client not configured; yutori patch skipped")
		return nil
	}
	resp, err := restateingress.ServiceSend[*workflow.ApprovalInput](
		s.IngressClient, workflow.ScoutOpsServiceName, "ApplyApproval").Send(context.Background(), &in)
	if err != nil {
		slog.Error("failed to forward approval", "err", err)
		return err
	}
	slog.Info("approval forwarded", "invocation_id", resp.Id(), "scout_id", in.ScoutID)
	return nil
}

// stopScout fire-and-forwards a scout deletion to the worker so it can DELETE
// the scout on Yutori natively, halting recurring credit consumption. The DB
// row has already been marked STOPPED by the caller.
func (s *Server) stopScout(in workflow.DeleteScoutInput) error {
	if s.IngressClient == nil {
		slog.Warn("ingress client not configured; yutori delete skipped")
		return nil
	}
	resp, err := restateingress.ServiceSend[*workflow.DeleteScoutInput](
		s.IngressClient, workflow.ScoutOpsServiceName, "DeleteScout").Send(context.Background(), &in)
	if err != nil {
		slog.Error("failed to forward scout delete", "err", err)
		return err
	}
	slog.Info("scout delete forwarded", "invocation_id", resp.Id(), "scout_id", in.ScoutID)
	return nil
}
