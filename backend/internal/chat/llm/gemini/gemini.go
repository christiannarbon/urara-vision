// Package gemini is the llm.Model for Gemini on Vertex AI, with ADC.
package gemini

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/genai"

	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/llm"
)

type model struct {
	client      *genai.Client
	name        string
	timeout     time.Duration
	temperature *float64
	maxTokens   int
}

// New builds one Vertex client for the process. RetryOptions stays nil: the
// SDK then makes a single attempt, as the Python service did.
func New(ctx context.Context, s config.Settings, _ *slog.Logger) (llm.Model, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  s.VertexProject,
		Location: s.VertexLocation,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	return &model{
		client: client, name: s.LLMModel, timeout: s.LLMTimeout,
		temperature: s.LLMTemperature, maxTokens: s.LLMMaxOutputTokens,
	}, nil
}

// Generate fills unset temperature and output limit from settings.
func (m *model) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	if req.Temperature == nil {
		req.Temperature = m.temperature
	}
	if req.MaxOutputTokens == 0 {
		req.MaxOutputTokens = m.maxTokens
	}
	contents, cfg, err := Encode(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	resp, err := m.client.Models.GenerateContent(ctx, m.name, contents, cfg)
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini: %w", err)
	}
	out, err := Decode(resp)
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini: %w", err)
	}
	return out, nil
}
