package anthropic_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/llm/anthropic"
)

// encode also checks that roles alternate, starting with user, on every encoded request.
func encode(t *testing.T, req llm.Request) sdk.MessageNewParams {
	t.Helper()
	p, err := anthropic.Encode("claude-haiku-4-5@20251001", req)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for i, m := range p.Messages {
		want := sdk.MessageParamRoleUser
		if i%2 == 1 {
			want = sdk.MessageParamRoleAssistant
		}
		if m.Role != want {
			t.Errorf("message %d role = %s, want %s", i, m.Role, want)
		}
	}
	return p
}

func TestAConversationMergesToolResultsAndTheNoticeIntoOneUserMessage(t *testing.T) {
	p := encode(t, llm.Request{
		System: "be brief",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Text: "q"},
			{Role: llm.RoleAssistant, Text: "looking", ToolCalls: []llm.ToolCall{
				{ID: "toolu_1", Name: "get_tables", Args: json.RawMessage(`{"ids":["a/b"]}`)},
				{ID: "toolu_2", Name: "search_model", Args: json.RawMessage(`{"query":"x"}`)},
			}},
			{Role: llm.RoleTool, ToolCallID: "toolu_1", Text: "tables"},
			{Role: llm.RoleTool, ToolCallID: "toolu_2", Text: "hits"},
			{Role: llm.RoleUser, Text: "budget notice"},
		},
	})

	if len(p.System) != 1 || p.System[0].Text != "be brief" {
		t.Errorf("system = %+v", p.System)
	}
	if len(p.Messages) != 3 {
		t.Fatalf("%d messages, want 3", len(p.Messages))
	}

	asst := p.Messages[1].Content
	if len(asst) != 3 || asst[0].OfText == nil || asst[0].OfText.Text != "looking" {
		t.Fatalf("assistant blocks = %+v", asst)
	}
	if tu := asst[1].OfToolUse; tu == nil || tu.ID != "toolu_1" || tu.Name != "get_tables" || string(tu.Input.(json.RawMessage)) != `{"ids":["a/b"]}` {
		t.Errorf("first tool use = %+v", asst[1].OfToolUse)
	}

	user := p.Messages[2].Content
	if len(user) != 3 {
		t.Fatalf("%d user blocks, want 2 results then 1 text", len(user))
	}
	for i, id := range []string{"toolu_1", "toolu_2"} {
		if r := user[i].OfToolResult; r == nil || r.ToolUseID != id {
			t.Errorf("block %d = %+v, want result for %s", i, user[i], id)
		}
	}
	if user[2].OfText == nil || user[2].OfText.Text != "budget notice" {
		t.Errorf("last block = %+v", user[2])
	}
}

func TestToolResultsGoBeforeTextInAMergedMessage(t *testing.T) {
	p := encode(t, llm.Request{Messages: []llm.Message{
		{Role: llm.RoleUser, Text: "q"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "a", Name: "t"}, {ID: "b", Name: "t"}}},
		{Role: llm.RoleTool, ToolCallID: "a", Text: "1"},
		{Role: llm.RoleUser, Text: "notice"},
		{Role: llm.RoleTool, ToolCallID: "b", Text: "2"},
	}})
	var kinds []string
	for _, b := range p.Messages[2].Content {
		switch {
		case b.OfToolResult != nil:
			kinds = append(kinds, "result:"+b.OfToolResult.ToolUseID)
		case b.OfText != nil:
			kinds = append(kinds, "text")
		}
	}
	if !reflect.DeepEqual(kinds, []string{"result:a", "result:b", "text"}) {
		t.Errorf("blocks = %v", kinds)
	}
}

func TestAnAssistantWithoutTextHasOnlyToolUse(t *testing.T) {
	p := encode(t, llm.Request{Messages: []llm.Message{
		{Role: llm.RoleUser, Text: "q"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c", Name: "t"}}},
	}})
	blocks := p.Messages[1].Content
	if len(blocks) != 1 || blocks[0].OfToolUse == nil || string(blocks[0].OfToolUse.Input.(json.RawMessage)) != "{}" {
		t.Errorf("blocks = %+v", blocks)
	}
}

func TestToolsSendTheRawSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`
	p := encode(t, llm.Request{Tools: []llm.ToolDef{{Name: "search_model", Description: "find", Schema: json.RawMessage(schema)}}})
	body, err := json.Marshal(p.Tools[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"input_schema":`+schema) {
		t.Errorf("tool = %s", body)
	}
}

// The whole request as sent, so an SDK upgrade that changes the shape fails here.
func TestTheWireShape(t *testing.T) {
	p := encode(t, llm.Request{
		System: "be brief",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Text: "q"},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "toolu_1", Name: "list_domains"}}},
			{Role: llm.RoleTool, ToolCallID: "toolu_1", Text: "none"},
		},
		Tools:      []llm.ToolDef{{Name: "list_domains", Description: "every domain"}},
		ToolChoice: llm.ToolChoiceAny,
	})
	got, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
		"model": "claude-haiku-4-5@20251001",
		"max_tokens": 2048,
		"system": [{"type": "text", "text": "be brief"}],
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "q"}]},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "toolu_1", "name": "list_domains", "input": {}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "toolu_1", "is_error": false,
				"content": [{"type": "text", "text": "none"}]}]}
		],
		"tools": [{"name": "list_domains", "description": "every domain", "input_schema": {"type": "object", "properties": {}}}],
		"tool_choice": {"type": "any"}
	}`
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("request =\n%s", got)
	}
}

func TestToolChoice(t *testing.T) {
	if p := encode(t, llm.Request{ToolChoice: llm.ToolChoiceAny, Tools: []llm.ToolDef{{Name: "t"}}}); p.ToolChoice.OfAny == nil {
		t.Errorf("Any: %+v", p.ToolChoice)
	}
	if p := encode(t, llm.Request{ToolChoice: llm.ToolChoiceAuto}); !reflect.ValueOf(p.ToolChoice).IsZero() {
		t.Errorf("Auto: %+v", p.ToolChoice)
	}
}

func TestTemperatureAndOutputLimit(t *testing.T) {
	p := encode(t, llm.Request{})
	if p.MaxTokens != 2048 || p.Temperature.Valid() {
		t.Errorf("unset: max %d, temperature %v", p.MaxTokens, p.Temperature)
	}
	temp := 0.2
	p = encode(t, llm.Request{MaxOutputTokens: 500, Temperature: &temp})
	if p.MaxTokens != 500 || p.Temperature.Value != 0.2 {
		t.Errorf("set: max %d, temperature %v", p.MaxTokens, p.Temperature)
	}
}

func TestEncodeRefuses(t *testing.T) {
	for name, req := range map[string]llm.Request{
		"unknown role":      {Messages: []llm.Message{{Role: "system", Text: "x"}}},
		"empty assistant":   {Messages: []llm.Message{{Role: llm.RoleAssistant}}},
		"any without tools": {ToolChoice: llm.ToolChoiceAny},
	} {
		if _, err := anthropic.Encode("m", req); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func message(t *testing.T, body string) *sdk.Message {
	t.Helper()
	var msg sdk.Message
	if err := json.Unmarshal([]byte(body), &msg); err != nil {
		t.Fatal(err)
	}
	return &msg
}

func TestDecode(t *testing.T) {
	const usage = `"usage":{"input_tokens":30,"output_tokens":12}`
	textBlock := `{"type":"text","text":"Hello"}`
	toolBlock := `{"type":"tool_use","id":"toolu_1","name":"get_tables","input":{"ids":["a/b"]}}`
	call := llm.ToolCall{ID: "toolu_1", Name: "get_tables", Args: json.RawMessage(`{"ids":["a/b"]}`)}

	for _, tc := range []struct {
		name, content, stop string
		text                string
		calls               []llm.ToolCall
	}{
		{"text", textBlock + `,{"type":"text","text":" there"}`, "end_turn", "Hello there", nil},
		{"tool uses", toolBlock, "tool_use", "", []llm.ToolCall{call}},
		{"both", textBlock + "," + toolBlock, "tool_use", "Hello", []llm.ToolCall{call}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := anthropic.Decode(message(t, `{"content":[`+tc.content+`],"stop_reason":"`+tc.stop+`",`+usage+`}`))
			if err != nil {
				t.Fatal(err)
			}
			if got.Text != tc.text || len(got.ToolCalls) != len(tc.calls) {
				t.Fatalf("got %+v", got)
			}
			for i, c := range tc.calls {
				g := got.ToolCalls[i]
				if g.ID != c.ID || g.Name != c.Name || string(g.Args) != string(c.Args) {
					t.Errorf("call %d = %+v", i, g)
				}
			}
			if *got.Usage != (llm.Usage{InputTokens: 30, OutputTokens: 12, TotalTokens: 42}) {
				t.Errorf("usage = %+v", got.Usage)
			}
		})
	}

	t.Run("max tokens with nothing", func(t *testing.T) {
		_, err := anthropic.Decode(message(t, `{"content":[],"stop_reason":"max_tokens",`+usage+`}`))
		if err == nil || !strings.Contains(err.Error(), "output token limit") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("refusal with nothing", func(t *testing.T) {
		_, err := anthropic.Decode(message(t, `{"content":[],"stop_reason":"refusal",`+usage+`}`))
		if err == nil || !strings.Contains(err.Error(), "refusal") {
			t.Errorf("err = %v", err)
		}
	})
}
