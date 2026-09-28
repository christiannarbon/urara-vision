// Package anthropic is the llm.Model for Claude on Vertex AI, with ADC.
package anthropic

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
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
	project     string
	location    string
	mu          sync.Mutex
	client      *anthropic.Client // built on first use
	name        string
	timeout     time.Duration
	temperature *float64
	maxTokens   int
}

// New configures one Vertex client for the process.
func New(_ context.Context, s config.Settings, _ *slog.Logger) (llm.Model, error) {
	return &model{
		project: s.VertexProject, location: s.VertexLocation,
		name: s.LLMModel, timeout: s.LLMTimeout,
		temperature: s.LLMTemperature, maxTokens: s.LLMMaxOutputTokens,
	}, nil
}

// clientFor builds the client on first use, so the service starts without ADC
// as Python does. Only success is kept, so a transient failure is retried.
func (m *model) clientFor(ctx context.Context) (*anthropic.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client == nil {
		ctx = context.WithoutCancel(ctx)
		// Looked up ourselves, as vertex.WithGoogleAuth panics without ADC.
		creds, err := google.FindDefaultCredentials(ctx, cloudPlatformScope)
		if err != nil {
			return nil, err
		}
		// No retries: the SDK retries twice by default, and the Python service did not.
		client := anthropic.NewClient(
			vertex.WithCredentials(ctx, m.location, m.project, creds),
			option.WithMaxRetries(0),
		)
		m.client = &client
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
	params, err := Encode(m.name, req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic: %w", err)
	}
	client, err := m.clientFor(ctx)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	msg, err := client.Messages.New(ctx, params)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic: %w", err)
	}
	out, err := Decode(msg)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic: %w", err)
	}
	return out, nil
}
