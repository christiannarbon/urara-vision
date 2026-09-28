package anthropic_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/llm/anthropic"
)

// Without ADC the service still starts; the first call fails instead.
func TestNewNeedsNoCredentials(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/adc.json")
	s := config.Settings{VertexProject: "p", VertexLocation: "us-central1", LLMModel: "m", LLMTimeout: time.Second}
	m, err := anthropic.New(context.Background(), s, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = m.Generate(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Text: "hi"}}})
	if err == nil || !strings.HasPrefix(err.Error(), "anthropic: ") {
		t.Errorf("Generate: %v", err)
	}
}
