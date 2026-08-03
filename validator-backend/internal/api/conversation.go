package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"validator-backend/internal/llm"
	"validator-backend/internal/models"
)

const conversationSystemPrompt = `You are an expert market research analyst helping a user refine their business idea into a research brief for automated market validation.

CRITICAL: ANALYZE what the user has ALREADY told you before asking anything. Only ask about what is genuinely MISSING from their description. Never ask about something they already explained.

How to decide what to ask:
- If the user described the product but not WHO it's for → ask about target audience.
- If they named the audience but not the problem → ask about the core pain point.
- If both are clear but competitors aren't mentioned → ask about alternatives they know of.
- If they mentioned competitors but not how they're different → ask about differentiation.
- If the idea is well-described but monetization is unclear → ask about pricing model.
- If they've given enough detail across most areas → skip remaining questions and produce the brief.

Rules:
1. Ask ONE question at a time. Make it specific to THEIR idea, not a generic template.
2. Do NOT ask a question if the user's description already answers it. Acknowledge what they said.
3. Aim for 2-4 questions total. If the initial description is detailed, you may need fewer.
4. Be conversational and encouraging. Reference their specific idea in your questions.
5. Once you have enough context, produce the research brief immediately — don't pad with unnecessary questions.
6. If the user adds more info after the brief is produced, update it accordingly.

RESPONSE FORMAT — always return valid JSON:
- For a question: {"type": "question", "content": "your question here"}
- For the research brief: {"type": "prompt", "content": "the full brief in markdown"}

RESEARCH BRIEF FORMAT:
## Research Brief: [Concise Title]

**Target Market:** [specific audience — from what the user told you]
**Core Problem:** [one sentence]

### Key Research Questions:
- [question 1]
- [question 2]
- [3-5 more specific, researchable questions]

### Competitive Landscape:
- [key competitors/alternatives to track]

### Success Indicators (PRO):
- [what signals would validate this idea]

### Risk Factors (CON):
- [what signals would challenge this idea]`

type llmConversationResponse struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// handlePostIdea creates a new idea in DRAFT status and generates the first
// clarifying question from the LLM. The idea is NOT yet sent to research — that
// happens when the user clicks "Start Research" via handleStartResearch.
func (s *Server) handlePostIdea(w http.ResponseWriter, r *http.Request) {
	var req createIdeaRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		writeError(w, http.StatusBadRequest, "description is required")
		return
	}
	if req.ScoutingFrequencyDays <= 0 {
		req.ScoutingFrequencyDays = 7
	}

	title := deriveTitle(desc)
	idea := &models.Idea{
		ID:            uuid.NewString(),
		Title:         title,
		Description:   desc,
		FrequencyDays: req.ScoutingFrequencyDays,
		Status:        models.IdeaStatusDraft,
	}
	if err := s.DB.CreateIdea(r.Context(), idea); err != nil {
		slog.Error("persist idea failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not persist idea")
		return
	}

	now := time.Now().UTC()
	conversation := []models.ChatMessage{
		{Role: "user", Content: desc, Timestamp: now},
	}

	var assistantMsg models.ChatMessage
	if s.LLM != nil && s.LLM.Configured() {
		llmResp, err := s.callConversationLLM(r.Context(), []llm.Message{{Role: "user", Content: desc}})
		if err != nil {
			slog.Error("first question generation failed", "err", err)
			assistantMsg = models.ChatMessage{
				Role:        "assistant",
				Content:     "I'd love to learn more! Who is your primary target audience for this idea?",
				MessageType: "question",
				Timestamp:   time.Now().UTC(),
			}
		} else {
			assistantMsg = models.ChatMessage{
				Role:        "assistant",
				Content:     llmResp.Content,
				MessageType: llmResp.Type,
				Timestamp:   time.Now().UTC(),
			}
			if llmResp.Type == "prompt" {
				idea.RefinedPrompt = llmResp.Content
				_ = s.DB.UpdateRefinedPrompt(r.Context(), idea.ID, llmResp.Content)
			}
		}
	} else {
		assistantMsg = models.ChatMessage{
			Role:        "assistant",
			Content:     "LLM is not configured. You can start research directly.",
			MessageType: "question",
			Timestamp:   time.Now().UTC(),
		}
	}
	conversation = append(conversation, assistantMsg)
	_ = s.DB.UpdateConversation(r.Context(), idea.ID, conversation)

	writeJSON(w, http.StatusCreated, ConversationDTO{
		ID:            idea.ID,
		Title:         idea.Title,
		Status:        string(idea.Status),
		Conversation:  chatMessagesToDTOs(conversation),
		RefinedPrompt: idea.RefinedPrompt,
	})
}

// handleChat continues the refinement conversation.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
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
		slog.Error("get idea for chat failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load idea")
		return
	}
	if idea.Status != models.IdeaStatusDraft {
		writeError(w, http.StatusConflict, "idea is not in DRAFT status; conversation is locked")
		return
	}

	var req ChatRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	conversation := idea.Conversation
	conversation = append(conversation, models.ChatMessage{
		Role: "user", Content: msg, Timestamp: time.Now().UTC(),
	})

	var assistantMsg models.ChatMessage
	var newPrompt string
	if s.LLM != nil && s.LLM.Configured() {
		llmMsgs := make([]llm.Message, 0, len(conversation))
		for _, m := range conversation {
			llmMsgs = append(llmMsgs, llm.Message{Role: m.Role, Content: m.Content})
		}
		llmResp, err := s.callConversationLLM(r.Context(), llmMsgs)
		if err != nil {
			slog.Error("chat LLM call failed", "err", err)
			writeError(w, http.StatusInternalServerError, "LLM conversation failed")
			return
		}
		assistantMsg = models.ChatMessage{
			Role:        "assistant",
			Content:     llmResp.Content,
			MessageType: llmResp.Type,
			Timestamp:   time.Now().UTC(),
		}
		if llmResp.Type == "prompt" {
			newPrompt = llmResp.Content
			_ = s.DB.UpdateRefinedPrompt(r.Context(), id, newPrompt)
		}
	} else {
		writeError(w, http.StatusServiceUnavailable, "LLM is not configured")
		return
	}

	conversation = append(conversation, assistantMsg)
	_ = s.DB.UpdateConversation(r.Context(), id, conversation)

	writeJSON(w, http.StatusOK, ChatResponseDTO{
		Message: ChatMessageDTO{
			Role:        assistantMsg.Role,
			Content:     assistantMsg.Content,
			MessageType: assistantMsg.MessageType,
			Timestamp:   rfc3339(assistantMsg.Timestamp),
		},
		Prompt: newPrompt,
		Status: string(idea.Status),
	})
}

// handleGetConversation returns the full conversation + refined prompt.
func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusInternalServerError, "could not load idea")
		return
	}
	writeJSON(w, http.StatusOK, ConversationDTO{
		ID:            idea.ID,
		Title:         idea.Title,
		Status:        string(idea.Status),
		Conversation:  chatMessagesToDTOs(idea.Conversation),
		RefinedPrompt: idea.RefinedPrompt,
	})
}

// handleUpdatePrompt lets the user manually edit the refined research prompt.
func (s *Server) handleUpdatePrompt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing idea id")
		return
	}
	var req PromptUpdateDTO
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if err := s.DB.UpdateRefinedPrompt(r.Context(), id, prompt); err != nil {
		if notFound(err) {
			writeError(w, http.StatusNotFound, "idea not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not update prompt")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleStartResearch transitions an idea from DRAFT to INITIAL_SWEEP and
// triggers the Day 0 setup workflow.
func (s *Server) handleStartResearch(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusInternalServerError, "could not load idea")
		return
	}
	if idea.Status != models.IdeaStatusDraft {
		writeError(w, http.StatusConflict, "idea is not in DRAFT status")
		return
	}

	if err := s.DB.StartResearch(r.Context(), id); err != nil {
		slog.Error("start research failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not start research")
		return
	}

	researchPrompt := idea.RefinedPrompt
	if researchPrompt == "" {
		researchPrompt = idea.Description
	}

	_ = s.triggerDay0(idea.ID, idea.Title, researchPrompt, idea.FrequencyDays)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":         idea.ID,
		"status":     string(models.IdeaStatusInitialSweep),
		"workflowId": idea.ID,
	})
}

// callConversationLLM calls the LLM with the conversation system prompt and
// parses the JSON response.
func (s *Server) callConversationLLM(ctx context.Context, messages []llm.Message) (*llmConversationResponse, error) {
	raw, err := s.LLM.CompleteConversation(ctx, conversationSystemPrompt, messages, true)
	if err != nil {
		return nil, err
	}
	var resp llmConversationResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return &llmConversationResponse{
			Type:    "question",
			Content: raw,
		}, nil
	}
	if resp.Type == "" {
		resp.Type = "question"
	}
	return &resp, nil
}

func chatMessagesToDTOs(msgs []models.ChatMessage) []ChatMessageDTO {
	out := make([]ChatMessageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, ChatMessageDTO{
			Role:        m.Role,
			Content:     m.Content,
			MessageType: m.MessageType,
			Timestamp:   rfc3339(m.Timestamp),
		})
	}
	return out
}
