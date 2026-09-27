package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/tools"
	"urara-vision/backend/internal/model"
)

type Options struct {
	ModelName         string
	MaxHistory        int
	MaxToolIterations int
	MaxTurnTokens     int
}

// CallRecord is one tool call the turn made, for tracing an answer to what it read.
type CallRecord struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// Answer is one turn's answer and everything needed to explain it later.
type Answer struct {
	Text       string
	Citations  []string
	ToolCalls  []CallRecord
	Iterations int
	Truncated  bool
	Model      string
	LatencyMS  int
	// Usage is what the provider reported, summed; empty rather than guessed.
	Usage            map[string]int
	PromptTokens     int
	CompletionTokens int
	// TokensEstimated is true when any call went unreported, so counts are never mixed.
	TokensEstimated bool
}

// Backend is what a turn reads: the tools' calls and the context card.
type Backend interface {
	tools.Backend
	CardBackend
}

type Agent struct {
	model   llm.Model
	backend Backend
	cards   *CardCache
	opts    Options
	log     *slog.Logger
}

func New(m llm.Model, b Backend, cards *CardCache, opts Options, log *slog.Logger) *Agent {
	return &Agent{model: m, backend: b, cards: cards, opts: opts, log: log}
}

var errLatestSnapshot = errors.New("answer requires a concrete snapshot ID, not 'latest'; resolve it first")

// Answer answers one question about one snapshot.
func (a *Agent) Answer(ctx context.Context, question, snapshotID string, history []model.Message, language string) (Answer, error) {
	// Reading the wrong snapshot gives a confidently wrong answer.
	if snapshotID == "latest" {
		return Answer{}, errLatestSnapshot
	}
	messages := ToLLMMessages(TruncateHistory(history, a.opts.MaxHistory))
	messages = append(messages, llm.Message{Role: llm.RoleUser, Text: question})
	return a.run(ctx, snapshotID, language, messages)
}
