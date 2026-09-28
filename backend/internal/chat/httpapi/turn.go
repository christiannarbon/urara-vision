package httpapi

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/chat/agent"
	"urara-vision/backend/internal/chat/reqctx"
	"urara-vision/backend/internal/model"
)

type turnRequest struct {
	Question *string `json:"question"`
	Language string  `json:"language"`
}

// turnResponse is Python's TurnResponse.
type turnResponse struct {
	ConversationID   string             `json:"conversationId"`
	UserMessage      messageResponse    `json:"userMessage"`
	AssistantMessage messageResponse    `json:"assistantMessage"`
	ToolCalls        []agent.CallRecord `json:"toolCalls"`
	Truncated        bool               `json:"truncated"`
	LatencyMS        int                `json:"latencyMs"`
	Model            string             `json:"model"`
}

// takeTurn asks one question of an existing conversation and stores both messages.
// The order of steps is the design.
func (s *Server) takeTurn(w http.ResponseWriter, r *http.Request) {
	var body turnRequest
	if !s.DecodeJSON(w, r, &body) {
		return
	}
	if body.Question == nil {
		FieldError(w, r, Required("question"))
		return
	}
	// Before anything is held or fetched, so a rejected question touches nothing.
	question, bad := cleanQuestion(*body.Question, s.settings.MaxQuestionChars)
	if bad != nil {
		bad.write(w, r)
		return
	}

	// The slot first: the reverse lets a queued turn block its thread.
	release, err := s.limiter.Acquire(r.Context())
	if err != nil {
		s.RenderTurnError(w, r, err)
		return
	}
	defer release()
	cid := chi.URLParam(r, "cid")
	unlock, err := s.locks.Lock(r.Context(), cid)
	if err != nil {
		s.RenderTurnError(w, r, err)
		return
	}
	defer unlock()

	s.runConversationTurn(w, r, cid, question, normaliseLanguage(body.Language))
}

func (s *Server) runConversationTurn(w http.ResponseWriter, r *http.Request, cid, question, language string) {
	ctx := r.Context()
	conv, err := s.backend.GetConversation(ctx, cid)
	if err != nil {
		s.RenderBackendError(w, r, err)
		return
	}
	// Read before the question is written, so it is not in its own history.
	history := conv.Messages

	turns := 0
	for _, m := range history {
		if m.Role == model.RoleAssistant {
			turns++
		}
	}
	// 409: the request is fine, the conversation's length refuses it.
	if limit := s.settings.MaxConversationTurns; turns >= limit {
		WriteError(w, r, http.StatusConflict, map[string]any{"detail": fmt.Sprintf(
			"this conversation has reached its limit of %d turns; start a new conversation to keep asking", limit)})
		return
	}

	// Stored before the model runs, so a provider failure leaves it for a retry.
	userMsg, err := s.backend.AppendMessage(ctx, cid, model.RoleUser, question, nil, nil)
	if err != nil {
		s.RenderBackendError(w, r, err)
		return
	}

	answer, err := s.runTurn(ctx, question, conv.SnapshotID, history, language, cid)
	if err != nil {
		s.RenderTurnError(w, r, err)
		return
	}
	answer = withEmptyLists(answer)
	// Paid for: stored even if the caller has gone.
	store := context.WithoutCancel(ctx)
	record := turnRecord(ctx, answer, conv.SnapshotID, cid)
	s.logTurn(record)

	// The calls themselves are stored; the log line carries only their count.
	meta := maps.Clone(record)
	meta["toolCalls"] = answer.ToolCalls
	meta["usage"] = answer.Usage
	assistantMsg, err := s.backend.AppendMessage(store, cid, model.RoleAssistant, answer.Text, answer.Citations, meta)
	if err != nil {
		// Still a failure: the next fetch would contradict a success.
		s.log.Error("the answer could not be stored", "request_id", reqctx.RequestID(ctx),
			"conversation_id", cid, "answer", answer.Text, "usage", answer.Usage)
		s.RenderBackendError(w, r, err)
		return
	}

	// After the answer is stored, so a title never exists for a turn that produced nothing.
	s.setTitleOnce(store, conv, question)

	writeJSON(w, http.StatusOK, turnResponse{
		ConversationID:   cid,
		UserMessage:      messageJSON(userMsg),
		AssistantMessage: messageJSON(assistantMsg),
		ToolCalls:        answer.ToolCalls,
		Truncated:        answer.Truncated,
		LatencyMS:        answer.LatencyMS,
		Model:            answer.Model,
	})
}

// setTitleOnce titles an untitled thread from its first question. A failure is logged, not returned.
func (s *Server) setTitleOnce(ctx context.Context, conv model.Conversation, question string) {
	if strings.TrimSpace(conv.Title) != "" {
		return
	}
	title := TitleFromQuestion(question)
	if title == "" {
		return
	}
	if _, err := s.backend.SetConversationTitle(ctx, conv.ID, title); err != nil {
		s.log.Warn("could not set the conversation title", "request_id", reqctx.RequestID(ctx),
			"conversation_id", conv.ID, "error", err.Error())
	}
}
