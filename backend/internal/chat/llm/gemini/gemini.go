// Package gemini is the llm.Model for Gemini on Vertex AI, with ADC.
package gemini

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/genai"

	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/llm"
)

type model struct {
	cfg         genai.ClientConfig
	mu          sync.Mutex
	client      *genai.Client // built on first use
	name        string
	timeout     time.Duration
	temperature *float64
	maxTokens   int
}

// New configures one Vertex client for the process. RetryOptions stays nil:
// the SDK then makes a single attempt, as the Python service did.
func New(_ context.Context, s config.Settings, _ *slog.Logger) (llm.Model, error) {
	return &model{
		cfg:  genai.ClientConfig{Backend: genai.BackendVertexAI, Project: s.VertexProject, Location: s.VertexLocation},
		name: s.LLMModel, timeout: s.LLMTimeout,
		temperature: s.LLMTemperature, maxTokens: s.LLMMaxOutputTokens,
	}, nil
}

// clientFor builds the client on first use, so the service starts without ADC
// as Python does. Only success is kept, so a transient failure is retried.
func (m *model) clientFor(ctx context.Context) (*genai.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client == nil {
		cfg := m.cfg
		client, err := genai.NewClient(context.WithoutCancel(ctx), &cfg)
		if err != nil {
			return nil, err
		}
		m.client = client
	}
	return m.client, nil
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
	client, err := m.clientFor(ctx)
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	resp, err := client.Models.GenerateContent(ctx, m.name, contents, cfg)
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini: %w", err)
	}
	out, err := Decode(resp)
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini: %w", err)
	}
	return out, nil
}
