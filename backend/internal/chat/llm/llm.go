// Package llm is the provider-neutral model interface. Standard library only;
// each SDK lives in its own adapter package.
package llm

import (
	"context"
	"encoding/json"
	"strconv"
)

type Model interface {
	Generate(ctx context.Context, req Request) (Response, error)
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role
	Text       string     // user text, assistant text, or tool result
	ToolCalls  []ToolCall // assistant only
	ToolCallID string     // tool only
	ToolName   string     // tool only; Gemini needs the name on a function response
}

type ToolChoice int

const (
	ToolChoiceAuto ToolChoice = iota
	ToolChoiceAny
)

type ToolDef struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

type ToolCall struct {
	ID     string
	Name   string
	Args   json.RawMessage
	Opaque []byte // provider state to send back unread, e.g. Gemini thought signatures
}

type Request struct {
	System          string
	Messages        []Message
	Tools           []ToolDef
	ToolChoice      ToolChoice
	Temperature     *float64 // nil: provider default
	MaxOutputTokens int      // 0: provider default
}

type Response struct {
	Text      string
	ToolCalls []ToolCall
	Usage     *Usage // nil when the provider did not report it
}

// Usage counts thinking as output, so TotalTokens = InputTokens + OutputTokens.
type Usage struct {
	InputTokens, OutputTokens, TotalTokens int
}

// Map is the stored meta shape, LangChain's keys, so stats read Python and Go turns alike.
func (u Usage) Map() map[string]int {
	return map[string]int{
		"input_tokens":  u.InputTokens,
		"output_tokens": u.OutputTokens,
		"total_tokens":  u.TotalTokens,
	}
}

// FillIDs gives each call without a provider ID one of the form call_<n>, n
// counting from 1, so the loop can always match results to calls.
func FillIDs(calls []ToolCall) {
	for i := range calls {
		if calls[i].ID == "" {
			calls[i].ID = "call_" + strconv.Itoa(i+1)
		}
	}
}
