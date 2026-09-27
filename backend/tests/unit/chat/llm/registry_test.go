package llm_test

import (
	"context"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"

	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/llm/llmtest"
)

// The registry is process-wide, so each test registers names of its own.
func fakeFactory(m llm.Model) llm.Factory {
	return func(context.Context, config.Settings, *slog.Logger) (llm.Model, error) { return m, nil }
}

func TestNewBuildsTheRegisteredProvider(t *testing.T) {
	want := llmtest.NewScripted()
	llm.Register("test-new", fakeFactory(want))

	got, err := llm.New(context.Background(), config.Settings{LLMProvider: "test-new"}, slog.New(slog.DiscardHandler))
	if err != nil || got != want {
		t.Fatalf("New = %v, %v", got, err)
	}
}

func TestAnUnknownProviderIsRefused(t *testing.T) {
	_, err := llm.New(context.Background(), config.Settings{LLMProvider: "nope"}, slog.New(slog.DiscardHandler))
	if err == nil || !strings.Contains(err.Error(), `unknown provider "nope"`) {
		t.Errorf("New(nope) = %v", err)
	}
}

func TestNamesAreSorted(t *testing.T) {
	llm.Register("test-zeta", fakeFactory(nil))
	llm.Register("test-alpha", fakeFactory(nil))
	names := llm.Names()
	if !slices.IsSorted(names) || !slices.Contains(names, "test-alpha") || !slices.Contains(names, "test-zeta") {
		t.Errorf("Names() = %v", names)
	}
}

func TestRegisteringTwiceIsABug(t *testing.T) {
	llm.Register("test-twice", fakeFactory(nil))
	defer func() {
		if recover() == nil {
			t.Error("a duplicate Register did not panic")
		}
	}()
	llm.Register("test-twice", fakeFactory(nil))
}

func TestDescribe(t *testing.T) {
	cases := map[string]map[string]string{
		"vertex":           {"provider": "vertex", "model": "m", "location": "global"},
		"vertex-anthropic": {"provider": "vertex-anthropic", "model": "m", "location": "global"},
		"openai":           {"provider": "openai", "model": "m"},
	}
	for provider, want := range cases {
		got := llm.Describe(config.Settings{LLMProvider: provider, LLMModel: "m", VertexLocation: "global"})
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Describe(%s) = %v, want %v", provider, got, want)
		}
	}
}

func TestUsageMapUsesTheStoredKeys(t *testing.T) {
	got := llm.Usage{InputTokens: 10, OutputTokens: 4, TotalTokens: 14}.Map()
	want := map[string]int{"input_tokens": 10, "output_tokens": 4, "total_tokens": 14}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Map() = %v", got)
	}
}

func TestFillIDsKeepsProviderIDs(t *testing.T) {
	calls := []llm.ToolCall{{Name: "a"}, {ID: "toolu_9", Name: "b"}, {Name: "c"}}
	llm.FillIDs(calls)
	if calls[0].ID != "call_1" || calls[1].ID != "toolu_9" || calls[2].ID != "call_3" {
		t.Errorf("ids = %q, %q, %q", calls[0].ID, calls[1].ID, calls[2].ID)
	}
}
