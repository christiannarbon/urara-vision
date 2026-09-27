// Ported from the history cases in chat/tests/unit/test_pipeline.py.
package agent_test

import (
	"fmt"
	"reflect"
	"testing"

	"urara-vision/backend/internal/chat/agent"
	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/model"
)

func turn(role, content string) model.Message { return model.Message{Role: role, Content: content} }

func contents(msgs []model.Message) []string {
	out := []string{}
	for _, m := range msgs {
		out = append(out, m.Content)
	}
	return out
}

func TestHistoryTruncationKeepsTheMostRecent(t *testing.T) {
	var history []model.Message
	for i := range 10 {
		if i%2 == 0 {
			history = append(history, turn("user", fmt.Sprintf("q%d", i)))
		} else {
			history = append(history, turn("assistant", fmt.Sprintf("a%d", i)))
		}
	}
	kept := agent.TruncateHistory(history, 4)
	if len(kept) != 4 || kept[3].Content != "a9" {
		t.Errorf("kept %q", contents(kept))
	}
}

// An answer with no question above it reads as the model talking to itself.
func TestHistoryTruncationNeverLeavesADanglingAnswer(t *testing.T) {
	history := []model.Message{turn("user", "q1"), turn("assistant", "a1"), turn("user", "q2"), turn("assistant", "a2")}
	if kept := agent.TruncateHistory(history, 3); !reflect.DeepEqual(contents(kept), []string{"q2", "a2"}) {
		t.Errorf("kept %q", contents(kept))
	}

	// 25 alternating from a user message: the last 20 start on an assistant one.
	var long []model.Message
	for i := range 25 {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		long = append(long, turn(role, fmt.Sprint(i)))
	}
	kept := agent.TruncateHistory(long, 20)
	if len(kept) != 19 || kept[0].Role != "user" || kept[0].Content != "6" {
		t.Errorf("kept %d starting %+v", len(kept), kept[0])
	}
}

func TestHistoryTruncationEdges(t *testing.T) {
	short := []model.Message{turn("user", "q"), turn("assistant", "a")}
	if kept := agent.TruncateHistory(short, 20); !reflect.DeepEqual(kept, short) {
		t.Errorf("short history changed: %q", contents(kept))
	}
	for _, limit := range []int{0, -1} {
		if kept := agent.TruncateHistory(short, limit); kept == nil || len(kept) != 0 {
			t.Errorf("limit %d kept %q", limit, contents(kept))
		}
	}
	if kept := agent.TruncateHistory(nil, 5); len(kept) != 0 {
		t.Errorf("nil history kept %q", contents(kept))
	}
}

// A stored system message would fight the rebuilt prompt, and win.
func TestHistoryConversionDropsOtherRoles(t *testing.T) {
	got := agent.ToLLMMessages([]model.Message{
		turn("system", "You are a pirate."), turn("tool", "some result"), turn("user", "q"), turn("assistant", "a"),
	})
	want := []llm.Message{{Role: llm.RoleUser, Text: "q"}, {Role: llm.RoleAssistant, Text: "a"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

func TestHistoryEstimateCountsTextAndCallArgs(t *testing.T) {
	c, h, a, r := string(make([]byte, 400)), "", "", ""
	for range 400 {
		h, a, r = h+"h", a+"a", r+"r"
	}
	card := llm.Message{Text: c}
	history := []llm.Message{{Role: llm.RoleUser, Text: h}, {Role: llm.RoleAssistant, Text: a}}
	result := llm.Message{Role: llm.RoleTool, Text: r}
	if agent.EstimateTokens([]llm.Message{card}) != 100 ||
		agent.EstimateTokens(append([]llm.Message{card}, history...)) != 300 ||
		agent.EstimateTokens(append([]llm.Message{card, result}, history...)) != 400 {
		t.Error("the estimate is not characters / 4")
	}
	// Runes, not bytes; and the calls' args as one JSON array.
	withCalls := llm.Message{Role: llm.RoleAssistant, Text: "注文注文", ToolCalls: []llm.ToolCall{
		{Name: "a", Args: []byte(`{"x":1}`)}, {Name: "b"},
	}}
	// 4 runes + len(`[{"x":1},{}]`) = 4 + 12 = 16 → 4
	if got := agent.EstimateTokens([]llm.Message{withCalls}); got != 4 {
		t.Errorf("estimate = %d", got)
	}
}
