package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"urara-vision/backend/internal/chat/llm"
)

// defaultMaxTokens fills the limit this API requires.
const defaultMaxTokens = 2048

// emptySchema is sent for a tool without one: the API requires type object.
const emptySchema = `{"type":"object","properties":{}}`

// Encode turns a neutral request into Messages API params. Pure.
func Encode(model string, req llm.Request) (anthropic.MessageNewParams, error) {
	if req.ToolChoice == llm.ToolChoiceAny && len(req.Tools) == 0 {
		return anthropic.MessageNewParams{}, errors.New("tool choice any needs at least one tool")
	}
	p := anthropic.MessageNewParams{Model: anthropic.Model(model), MaxTokens: defaultMaxTokens}
	if req.MaxOutputTokens > 0 {
		p.MaxTokens = int64(req.MaxOutputTokens)
	}
	if req.System != "" {
		p.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	for _, t := range req.Tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = json.RawMessage(emptySchema)
		}
		tool := &anthropic.ToolParam{Name: t.Name, InputSchema: param.Override[anthropic.ToolInputSchemaParam](schema)}
		if t.Description != "" {
			tool.Description = anthropic.String(t.Description)
		}
		p.Tools = append(p.Tools, anthropic.ToolUnionParam{OfTool: tool})
	}
	if req.ToolChoice == llm.ToolChoiceAny {
		p.ToolChoice = anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{}}
	}
	if req.Temperature != nil {
		p.Temperature = anthropic.Float(*req.Temperature)
	}

	for i, m := range req.Messages {
		switch m.Role {
		case llm.RoleUser:
			p.Messages = appendUser(p.Messages, anthropic.NewTextBlock(m.Text))
		case llm.RoleTool:
			p.Messages = appendUser(p.Messages, anthropic.NewToolResultBlock(m.ToolCallID, m.Text, false))
		case llm.RoleAssistant:
			var blocks []anthropic.ContentBlockParamUnion
			if m.Text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Text))
			}
			for _, call := range m.ToolCalls {
				blocks = append(blocks, anthropic.NewToolUseBlock(call.ID, orEmptyObject(call.Args), call.Name))
			}
			if len(blocks) == 0 {
				return anthropic.MessageNewParams{}, fmt.Errorf("message %d: assistant has no text or tool calls", i)
			}
			p.Messages = append(p.Messages, anthropic.NewAssistantMessage(blocks...))
		default:
			return anthropic.MessageNewParams{}, fmt.Errorf("message %d: unknown role %q", i, m.Role)
		}
	}
	return p, nil
}

// appendUser merges into a trailing user message, as roles must alternate.
// Tool results go before any text in it, as the API requires.
func appendUser(msgs []anthropic.MessageParam, b anthropic.ContentBlockParamUnion) []anthropic.MessageParam {
	n := len(msgs)
	if n == 0 || msgs[n-1].Role != anthropic.MessageParamRoleUser {
		return append(msgs, anthropic.NewUserMessage(b))
	}
	blocks := msgs[n-1].Content
	at := len(blocks)
	if b.OfToolResult != nil {
		for at > 0 && blocks[at-1].OfToolResult == nil {
			at--
		}
	}
	msgs[n-1].Content = slices.Insert(blocks, at, b)
	return msgs
}

// orEmptyObject sends empty args as {}, not null.
func orEmptyObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

// Decode turns a message into a neutral response. Pure.
func Decode(msg *anthropic.Message) (llm.Response, error) {
	if msg == nil {
		return llm.Response{}, errors.New("claude returned no message")
	}
	var out llm.Response
	var text strings.Builder
	for _, block := range msg.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			text.WriteString(b.Text)
		case anthropic.ToolUseBlock:
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{ID: b.ID, Name: b.Name, Args: orEmptyObject(b.Input)})
		}
	}
	out.Text = text.String()
	llm.FillIDs(out.ToolCalls)

	in, outTokens := int(msg.Usage.InputTokens), int(msg.Usage.OutputTokens)
	out.Usage = &llm.Usage{InputTokens: in, OutputTokens: outTokens, TotalTokens: in + outTokens}
	if out.Text == "" && len(out.ToolCalls) == 0 {
		if msg.StopReason == anthropic.StopReasonMaxTokens {
			return out, fmt.Errorf("claude hit the output token limit before answering: %w", llm.ErrEmptyReply)
		}
		return out, fmt.Errorf("claude returned %w (stop reason %s)", llm.ErrEmptyReply, msg.StopReason)
	}
	return out, nil
}
