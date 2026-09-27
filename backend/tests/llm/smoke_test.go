//go:build llm

// Package llm_test calls real models and costs money. Run with `make test-llm`.
package llm_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/llm/anthropic"
	"urara-vision/backend/internal/chat/llm/gemini"
)

type target struct {
	factory         llm.Factory
	model, location string
}

// Defaults are the IDs and regions verified in Phase 17 FINDINGS.
var targets = map[string]target{
	"vertex":           {gemini.New, "gemini-2.5-flash", "us-central1"},
	"vertex-anthropic": {anthropic.New, "claude-haiku-4-5@20251001", "global"},
}

var weather = llm.ToolDef{
	Name:        "get_weather",
	Description: "Get the current weather for a city.",
	Schema:      json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
}

// eachProvider runs fn once per provider in LLM_SMOKE_PROVIDERS.
func eachProvider(t *testing.T, fn func(t *testing.T, m llm.Model)) {
	project := os.Getenv("VERTEX_PROJECT")
	if project == "" {
		t.Skip("set VERTEX_PROJECT (and run `gcloud auth application-default login`) to run this test; it calls a real model and costs money")
	}
	providers := os.Getenv("LLM_SMOKE_PROVIDERS")
	if providers == "" {
		providers = "vertex,vertex-anthropic"
	}
	for _, name := range strings.Split(providers, ",") {
		name = strings.TrimSpace(name)
		t.Run(name, func(t *testing.T) {
			tg, ok := targets[name]
			if !ok {
				t.Fatalf("unknown provider %q", name)
			}
			key := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
			s := config.Settings{
				LLMProvider:        name,
				LLMModel:           envOr("LLM_SMOKE_MODEL_"+key, tg.model),
				VertexProject:      project,
				VertexLocation:     envOr("LLM_SMOKE_LOCATION_"+key, tg.location),
				LLMMaxOutputTokens: 512,
				LLMTimeout:         30 * time.Second,
			}
			t.Logf("model %s at %s", s.LLMModel, s.VertexLocation)
			m, err := tg.factory(context.Background(), s, slog.Default())
			if err != nil {
				t.Fatal(err)
			}
			fn(t, m)
		})
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func generate(t *testing.T, m llm.Model, req llm.Request) llm.Response {
	t.Helper()
	resp, err := m.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func forcedCall(t *testing.T, m llm.Model, prompt string) (llm.Request, llm.Response) {
	t.Helper()
	req := llm.Request{
		Messages:   []llm.Message{{Role: llm.RoleUser, Text: prompt}},
		Tools:      []llm.ToolDef{weather},
		ToolChoice: llm.ToolChoiceAny,
	}
	resp := generate(t, m, req)
	if len(resp.ToolCalls) == 0 {
		t.Fatalf("no tool call; text %q", resp.Text)
	}
	for _, c := range resp.ToolCalls {
		if c.Name != weather.Name || !json.Valid(c.Args) {
			t.Fatalf("call = %s %s", c.Name, c.Args)
		}
	}
	return req, resp
}

func TestSmokePong(t *testing.T) {
	eachProvider(t, func(t *testing.T, m llm.Model) {
		resp := generate(t, m, llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Text: "Reply with exactly: pong"}}})
		if !strings.Contains(strings.ToLower(resp.Text), "pong") || resp.Usage == nil {
			t.Errorf("text %q, usage %v", resp.Text, resp.Usage)
		}
		t.Logf("text %q usage %+v", resp.Text, *resp.Usage)
	})
}

func TestSmokeForcedTool(t *testing.T) {
	eachProvider(t, func(t *testing.T, m llm.Model) {
		_, resp := forcedCall(t, m, "Hello")
		t.Logf("call %s %s", resp.ToolCalls[0].ID, resp.ToolCalls[0].Args)
	})
}

// Catches a dropped thought signature or a role-order error. A weather
// question, as a "Hello" is answered without the result.
func TestSmokeToolRoundTrip(t *testing.T) {
	eachProvider(t, func(t *testing.T, m llm.Model) {
		req, first := forcedCall(t, m, "What is the weather in Paris?")
		req.Messages = append(req.Messages, llm.Message{Role: llm.RoleAssistant, Text: first.Text, ToolCalls: first.ToolCalls})
		for _, c := range first.ToolCalls {
			req.Messages = append(req.Messages, llm.Message{
				Role: llm.RoleTool, ToolCallID: c.ID, ToolName: c.Name, Text: "18°C and sunny",
			})
		}
		req.ToolChoice = llm.ToolChoiceAuto

		resp := generate(t, m, req)
		if !strings.Contains(resp.Text, "18") {
			t.Errorf("answer does not mention the result: %q", resp.Text)
		}
		t.Logf("answer %q", resp.Text)
	})
}
