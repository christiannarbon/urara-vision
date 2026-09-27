// Package anthropic is the llm.Model for Claude on Vertex AI, with ADC.
package anthropic

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/vertex"
	"golang.org/x/oauth2/google"

	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/llm"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

type model struct {
	client      anthropic.Client
	name        string
	timeout     time.Duration
	temperature *float64
	maxTokens   int
}

// New builds one Vertex client for the process, with no retries: the SDK
// retries twice by default, and the Python service did not.
func New(ctx context.Context, s config.Settings, _ *slog.Logger) (llm.Model, error) {
	// Looked up here, as vertex.WithGoogleAuth panics without ADC.
	creds, err := google.FindDefaultCredentials(ctx, cloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	client := anthropic.NewClient(
		vertex.WithCredentials(ctx, s.VertexLocation, s.VertexProject, creds),
		option.WithMaxRetries(0),
	)
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
	params, err := Encode(m.name, req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	msg, err := m.client.Messages.New(ctx, params)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic: %w", err)
	}
	out, err := Decode(msg)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic: %w", err)
	}
	return out, nil
}
