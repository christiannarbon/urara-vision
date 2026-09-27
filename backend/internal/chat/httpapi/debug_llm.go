package httpapi

import (
	"context"
	"math"
	"net/http"
	"time"

	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/reqctx"
)

const (
	defaultProbeTimeout = 15 * time.Second
	probePrompt         = "Reply with exactly: pong"
)

// debugLLM sends one fixed prompt: the cheapest check that credentials work.
func (s *Server) debugLLM(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.probeTimeout)
	defer cancel()

	started := time.Now()
	resp, err := s.model.Generate(ctx, llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Text: probePrompt}},
	})
	if err != nil {
		// The provider's message can echo the prompt, so it is not logged.
		s.log.Error("llm probe failed", "request_id", reqctx.RequestID(r.Context()),
			"provider", s.settings.LLMProvider, "model", s.settings.LLMModel)
		WriteError(w, r, http.StatusBadGateway, map[string]any{
			"detail": "the language model provider did not answer; see the service logs",
		})
		return
	}

	body := map[string]any{
		"text":      resp.Text,
		"latencyMs": math.Round(float64(time.Since(started).Microseconds())/10) / 100,
	}
	for k, v := range llm.Describe(*s.settings) {
		body[k] = v
	}
	writeJSON(w, http.StatusOK, body)
}
