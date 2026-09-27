package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"urara-vision/backend/internal/chat/agent"
	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/reqctx"
	"urara-vision/backend/internal/model"
)

// Agent answers one question about one resolved snapshot.
type Agent interface {
	Answer(ctx context.Context, question, snapshotID string, history []model.Message, language string) (agent.Answer, error)
}

type apiError struct {
	status int
	detail string
}

func (e *apiError) write(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, e.status, map[string]any{"detail": e.detail})
}

// cleanQuestion trims the question and enforces its limit, counted in characters.
func cleanQuestion(q string, max int) (string, *apiError) {
	cleaned := strings.TrimSpace(q)
	if cleaned == "" {
		return "", &apiError{http.StatusBadRequest, `"question" must not be empty`}
	}
	// The limit is in the message, so a caller pasting a document knows how much to cut.
	if n := utf8.RuneCountInString(cleaned); n > max {
		return "", &apiError{http.StatusBadRequest, fmt.Sprintf(`"question" is %d characters, over the %d character limit`, n, max)}
	}
	return cleaned, nil
}

// normaliseLanguage falls back to EN rather than refusing the question.
func normaliseLanguage(s string) string {
	if l := strings.ToUpper(strings.TrimSpace(s)); l == "EN" || l == "JA" {
		return l
	}
	return "EN"
}

// runTurn runs one turn under the answer timeout. Backend errors keep their
// meaning; a timed-out turn and any other failure are the provider's.
func (s *Server) runTurn(ctx context.Context, question, snapshotID string, history []model.Message, language, conversationID string) (agent.Answer, error) {
	started := time.Now()
	turnCtx, cancel := context.WithTimeout(ctx, s.settings.AnswerTimeout)
	defer cancel()
	answer, err := s.agent.Answer(turnCtx, question, snapshotID, history, language)
	if err == nil {
		return answer, nil
	}

	// The type only: a provider message can quote the prompt back.
	s.log.Warn("turn failed", "event", "turn", "outcome", "failed", "requestId", reqctx.RequestID(ctx),
		"conversationId", nullable(conversationID), "snapshotId", snapshotID,
		"error", fmt.Sprintf("%T", err), "latencyMs", time.Since(started).Milliseconds())

	var apiErr *apiclient.Error
	switch {
	case ctx.Err() != nil:
		return agent.Answer{}, ctx.Err() // the caller hung up
	// Checked before backend errors: the deadline can fire inside a backend call.
	case errors.Is(turnCtx.Err(), context.DeadlineExceeded):
		return agent.Answer{}, &ProviderError{Reason: llm.Redact(err.Error(), question)}
	case errors.As(err, &apiErr):
		return agent.Answer{}, err
	}
	return agent.Answer{}, &ProviderError{Reason: llm.Redact(err.Error(), question)}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// turnRecord is logged per turn and, from Phase 19, stored as the answer's meta.
func turnRecord(ctx context.Context, a agent.Answer, snapshotID, conversationID string) map[string]any {
	names := make([]string, len(a.ToolCalls))
	for i, c := range a.ToolCalls {
		names[i] = c.Name
	}
	return map[string]any{
		"outcome":          "answered",
		"requestId":        reqctx.RequestID(ctx),
		"conversationId":   nullable(conversationID),
		"snapshotId":       snapshotID,
		"model":            a.Model,
		"promptTokens":     a.PromptTokens,
		"completionTokens": a.CompletionTokens,
		"tokensEstimated":  a.TokensEstimated,
		"toolCalls":        len(a.ToolCalls),
		"tools":            names,
		"iterations":       a.Iterations,
		"citations":        len(a.Citations),
		"latencyMs":        a.LatencyMS,
		"truncated":        a.Truncated,
	}
}

var recordKeys = []string{
	"outcome", "requestId", "conversationId", "snapshotId", "model", "promptTokens", "completionTokens",
	"tokensEstimated", "toolCalls", "tools", "iterations", "citations", "latencyMs", "truncated",
}

func (s *Server) logTurn(record map[string]any) {
	args := []any{"event", "turn"}
	for _, k := range recordKeys {
		args = append(args, k, record[k])
	}
	s.log.Info("turn answered", args...)
}
