package gemini_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/genai"

	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/llm/gemini"
)

func encode(t *testing.T, req llm.Request) ([]*genai.Content, *genai.GenerateContentConfig) {
	t.Helper()
	contents, cfg, err := gemini.Encode(req)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return contents, cfg
}

func TestAConversationEncodesRolesAndMergesToolResults(t *testing.T) {
	contents, cfg := encode(t, llm.Request{
		System: "be brief",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Text: "q"},
			{Role: llm.RoleAssistant, Text: "looking", ToolCalls: []llm.ToolCall{
				{ID: "call_1", Name: "get_tables", Args: json.RawMessage(`{"ids":["a/b"]}`)},
				{ID: "call_2", Name: "search_model", Args: json.RawMessage(`{"query":"x"}`)},
			}},
			{Role: llm.RoleTool, ToolCallID: "call_1", ToolName: "get_tables", Text: "tables"},
			{Role: llm.RoleTool, ToolCallID: "call_2", ToolName: "search_model", Text: "hits"},
			{Role: llm.RoleUser, Text: "budget notice"},
		},
	})

	if cfg.SystemInstruction == nil || cfg.SystemInstruction.Parts[0].Text != "be brief" {
		t.Errorf("system = %+v", cfg.SystemInstruction)
	}
	if len(contents) != 4 {
		t.Fatalf("%d contents, want 4 (user, model, one merged tool results, user)", len(contents))
	}
	roles := []string{contents[0].Role, contents[1].Role, contents[2].Role, contents[3].Role}
	if !reflect.DeepEqual(roles, []string{"user", "model", "user", "user"}) {
		t.Errorf("roles = %v", roles)
	}

	model := contents[1].Parts
	if len(model) != 3 || model[0].Text != "looking" || model[1].FunctionCall == nil || model[2].FunctionCall == nil {
		t.Fatalf("model parts = %+v", model)
	}
	if fc := model[1].FunctionCall; fc.ID != "call_1" || fc.Name != "get_tables" || !reflect.DeepEqual(fc.Args, map[string]any{"ids": []any{"a/b"}}) {
		t.Errorf("first call = %+v", fc)
	}

	results := contents[2].Parts
	if len(results) != 2 {
		t.Fatalf("%d result parts, want 2 in one content", len(results))
	}
	for i, want := range []struct{ id, name, text string }{{"call_1", "get_tables", "tables"}, {"call_2", "search_model", "hits"}} {
		fr := results[i].FunctionResponse
		if fr == nil || fr.ID != want.id || fr.Name != want.name || !reflect.DeepEqual(fr.Response, map[string]any{"output": want.text}) {
			t.Errorf("result %d = %+v", i, fr)
		}
	}
	if contents[3].Parts[0].Text != "budget notice" {
		t.Errorf("last user = %+v", contents[3].Parts)
	}
}

func TestAnAssistantWithoutTextHasNoEmptyTextPart(t *testing.T) {
	contents, _ := encode(t, llm.Request{Messages: []llm.Message{
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c", Name: "t", Args: json.RawMessage(`{}`)}}},
	}})
	if parts := contents[0].Parts; len(parts) != 1 || parts[0].FunctionCall == nil {
		t.Errorf("parts = %+v", parts)
	}
}

// The thought signature must survive a full round trip, or gemini-3 refuses the follow-up.
func TestOpaqueSurvivesARoundTrip(t *testing.T) {
	sig := []byte{0x01, 0xfe, 0x7f}
	resp := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{
		Role:  "model",
		Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: "get_weather", Args: map[string]any{"city": "Paris"}}, ThoughtSignature: sig}},
	}}}}
	decoded, err := gemini.Decode(resp)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded.ToolCalls[0].Opaque) != string(sig) {
		t.Fatalf("Opaque = %v", decoded.ToolCalls[0].Opaque)
	}

	contents, _ := encode(t, llm.Request{Messages: []llm.Message{{Role: llm.RoleAssistant, ToolCalls: decoded.ToolCalls}}})
	part := contents[0].Parts[0]
	if string(part.ThoughtSignature) != string(sig) || part.FunctionCall.ID == "" || part.FunctionCall.ID != decoded.ToolCalls[0].ID {
		t.Errorf("re-encoded part = %+v", part)
	}
}

func TestToolChoice(t *testing.T) {
	_, cfg := encode(t, llm.Request{ToolChoice: llm.ToolChoiceAny, Tools: []llm.ToolDef{{Name: "a"}, {Name: "b"}}})
	if cfg.ToolConfig == nil || cfg.ToolConfig.FunctionCallingConfig.Mode != genai.FunctionCallingConfigModeAny ||
		!reflect.DeepEqual(cfg.ToolConfig.FunctionCallingConfig.AllowedFunctionNames, []string{"a", "b"}) {
		t.Errorf("Any: %+v", cfg.ToolConfig.FunctionCallingConfig)
	}
	if _, cfg := encode(t, llm.Request{ToolChoice: llm.ToolChoiceAuto}); cfg.ToolConfig != nil {
		t.Errorf("Auto: %+v", cfg.ToolConfig)
	}
}

func TestTemperatureAndOutputLimit(t *testing.T) {
	_, cfg := encode(t, llm.Request{})
	if cfg.Temperature != nil || cfg.MaxOutputTokens != 0 {
		t.Errorf("unset: temperature %v, max %d", cfg.Temperature, cfg.MaxOutputTokens)
	}
	temp := 0.2
	_, cfg = encode(t, llm.Request{Temperature: &temp, MaxOutputTokens: 512})
	if cfg.Temperature == nil || *cfg.Temperature != float32(0.2) || cfg.MaxOutputTokens != 512 {
		t.Errorf("set: temperature %v, max %d", cfg.Temperature, cfg.MaxOutputTokens)
	}
}

func TestTheSearchModelSchemaSurvivesEncode(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "golden", "debug", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Body []struct {
			Name   string          `json:"name"`
			Schema json.RawMessage `json:"schema"`
		} `json:"body"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var schema json.RawMessage
	for _, tool := range golden.Body {
		if tool.Name == "search_model" {
			schema = tool.Schema
		}
	}
	if schema == nil {
		t.Fatal("search_model not in tools.json")
	}

	_, cfg := encode(t, llm.Request{Tools: []llm.ToolDef{{Name: "search_model", Description: "d", Schema: schema}}})
	decl := cfg.Tools[0].FunctionDeclarations[0]
	sent, err := json.Marshal(decl.ParametersJsonSchema)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	_ = json.Unmarshal(sent, &got)
	_ = json.Unmarshal(schema, &want)
	if !reflect.DeepEqual(got, want) || decl.Name != "search_model" || decl.Parameters != nil {
		t.Errorf("schema changed:\n got %s\nwant %s", sent, schema)
	}
}

func TestAToolWithoutASchemaSendsNone(t *testing.T) {
	_, cfg := encode(t, llm.Request{Tools: []llm.ToolDef{{Name: "list_domains"}}})
	b, _ := json.Marshal(cfg.Tools[0].FunctionDeclarations[0])
	if strings.Contains(string(b), "parametersJsonSchema") {
		t.Errorf("declaration = %s", b)
	}
}

func candidate(parts ...*genai.Part) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{Role: "model", Parts: parts}}}}
}

func TestDecode(t *testing.T) {
	t.Run("text only, thoughts skipped", func(t *testing.T) {
		resp := candidate(&genai.Part{Text: "hidden", Thought: true}, &genai.Part{Text: "hello "}, &genai.Part{Text: "there"})
		got, err := gemini.Decode(resp)
		if err != nil || got.Text != "hello there" || got.ToolCalls != nil || got.Usage != nil {
			t.Errorf("got %+v, %v", got, err)
		}
	})
	t.Run("tool calls only", func(t *testing.T) {
		got, err := gemini.Decode(candidate(
			&genai.Part{FunctionCall: &genai.FunctionCall{ID: "call_800326", Name: "a", Args: map[string]any{"x": 1.0}}},
			&genai.Part{FunctionCall: &genai.FunctionCall{Name: "b"}},
		))
		if err != nil || got.Text != "" || len(got.ToolCalls) != 2 {
			t.Fatalf("got %+v, %v", got, err)
		}
		if c := got.ToolCalls[0]; c.ID != "call_800326" || string(c.Args) != `{"x":1}` {
			t.Errorf("first = %+v", c)
		}
		if c := got.ToolCalls[1]; !strings.HasPrefix(c.ID, "call_") || string(c.Args) != "{}" {
			t.Errorf("second = %+v; want a generated ID and {} args", c)
		}
	})
	t.Run("text and calls with usage", func(t *testing.T) {
		resp := candidate(&genai.Part{Text: "checking"}, &genai.Part{FunctionCall: &genai.FunctionCall{Name: "a"}})
		resp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount: 24, CandidatesTokenCount: 5, ThoughtsTokenCount: 71, TotalTokenCount: 100,
		}
		got, err := gemini.Decode(resp)
		if err != nil || got.Text != "checking" || len(got.ToolCalls) != 1 {
			t.Fatalf("got %+v, %v", got, err)
		}
		if *got.Usage != (llm.Usage{InputTokens: 24, OutputTokens: 76, TotalTokens: 100}) {
			t.Errorf("usage = %+v; thinking counts as output", *got.Usage)
		}
	})
	t.Run("no candidates", func(t *testing.T) {
		_, err := gemini.Decode(&genai.GenerateContentResponse{PromptFeedback: &genai.GenerateContentResponsePromptFeedback{BlockReason: genai.BlockedReasonSafety}})
		if err == nil || !strings.Contains(err.Error(), "no candidates") || !strings.Contains(err.Error(), "SAFETY") || errors.Is(err, llm.ErrEmptyReply) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("output limit hit while thinking", func(t *testing.T) {
		resp := candidate(&genai.Part{Text: "hidden", Thought: true})
		resp.Candidates[0].FinishReason = genai.FinishReasonMaxTokens
		if _, err := gemini.Decode(resp); !errors.Is(err, llm.ErrEmptyReply) || !strings.Contains(err.Error(), "output token limit") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a safety stop with no parts is not an empty reply", func(t *testing.T) {
		resp := candidate()
		resp.Candidates[0].FinishReason = genai.FinishReasonSafety
		if _, err := gemini.Decode(resp); err == nil || errors.Is(err, llm.ErrEmptyReply) || !strings.Contains(err.Error(), "SAFETY") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("an empty reply keeps its usage", func(t *testing.T) {
		resp := candidate()
		resp.Candidates[0].FinishReason = genai.FinishReasonStop
		resp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 90, ThoughtsTokenCount: 10}
		got, err := gemini.Decode(resp)
		if !errors.Is(err, llm.ErrEmptyReply) || got.Usage == nil || *got.Usage != (llm.Usage{InputTokens: 90, OutputTokens: 10, TotalTokens: 100}) {
			t.Errorf("got %+v, %v", got, err)
		}
	})
	t.Run("a candidate with no content", func(t *testing.T) {
		_, err := gemini.Decode(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{FinishReason: genai.FinishReasonSafety}}})
		if err == nil || !strings.Contains(err.Error(), "SAFETY") || errors.Is(err, llm.ErrEmptyReply) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestEncodeRefuses(t *testing.T) {
	for name, req := range map[string]llm.Request{
		"unknown role":        {Messages: []llm.Message{{Role: "system", Text: "x"}}},
		"empty assistant":     {Messages: []llm.Message{{Role: llm.RoleAssistant}}},
		"tool result unnamed": {Messages: []llm.Message{{Role: llm.RoleTool, ToolCallID: "c", Text: "r"}}},
		"any without tools":   {ToolChoice: llm.ToolChoiceAny},
	} {
		if _, _, err := gemini.Encode(req); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
