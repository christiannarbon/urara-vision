package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"urara-vision/backend/internal/chat/llm"
)

// resultKey is the key Gemini documents for a function's output.
const resultKey = "output"

// Encode turns a neutral request into genai contents and config. Pure.
func Encode(req llm.Request) ([]*genai.Content, *genai.GenerateContentConfig, error) {
	if req.ToolChoice == llm.ToolChoiceAny && len(req.Tools) == 0 {
		return nil, nil, errors.New("tool choice any needs at least one tool")
	}
	cfg := &genai.GenerateContentConfig{}
	if req.System != "" {
		cfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: req.System}}}
	}
	if len(req.Tools) > 0 {
		decls := make([]*genai.FunctionDeclaration, len(req.Tools))
		for i, t := range req.Tools {
			decls[i] = &genai.FunctionDeclaration{Name: t.Name, Description: t.Description}
			// A nil RawMessage inside an any would still be sent, as null.
			if len(t.Schema) > 0 {
				decls[i].ParametersJsonSchema = t.Schema
			}
		}
		cfg.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
	}
	if req.ToolChoice == llm.ToolChoiceAny {
		// Every name listed, as LangChain sends it: without the list, Gemini
		// sometimes answered the forced first call with nothing (20.2).
		names := make([]string, len(req.Tools))
		for i, t := range req.Tools {
			names[i] = t.Name
		}
		cfg.ToolConfig = &genai.ToolConfig{FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingConfigModeAny, AllowedFunctionNames: names,
		}}
	}
	if req.Temperature != nil {
		t := float32(*req.Temperature)
		cfg.Temperature = &t
	}
	cfg.MaxOutputTokens = int32(req.MaxOutputTokens)

	var contents []*genai.Content
	for i, m := range req.Messages {
		switch m.Role {
		case llm.RoleUser:
			contents = append(contents, &genai.Content{Role: "user", Parts: []*genai.Part{{Text: m.Text}}})
		case llm.RoleAssistant:
			c := &genai.Content{Role: "model"}
			if m.Text != "" {
				c.Parts = append(c.Parts, &genai.Part{Text: m.Text})
			}
			for _, call := range m.ToolCalls {
				var args map[string]any
				if len(call.Args) > 0 {
					if err := json.Unmarshal(call.Args, &args); err != nil {
						return nil, nil, fmt.Errorf("message %d: tool call %s args: %w", i, call.Name, err)
					}
				}
				c.Parts = append(c.Parts, &genai.Part{
					FunctionCall:     &genai.FunctionCall{ID: call.ID, Name: call.Name, Args: args},
					ThoughtSignature: call.Opaque,
				})
			}
			if len(c.Parts) == 0 {
				return nil, nil, fmt.Errorf("message %d: assistant has no text or tool calls", i)
			}
			contents = append(contents, c)
		case llm.RoleTool:
			if m.ToolName == "" {
				return nil, nil, fmt.Errorf("message %d: tool result has no tool name", i)
			}
			part := &genai.Part{FunctionResponse: &genai.FunctionResponse{
				ID: m.ToolCallID, Name: m.ToolName, Response: map[string]any{resultKey: m.Text},
			}}
			// Consecutive results share one content.
			if n := len(contents); n > 0 && isToolResults(contents[n-1]) {
				contents[n-1].Parts = append(contents[n-1].Parts, part)
			} else {
				contents = append(contents, &genai.Content{Role: "user", Parts: []*genai.Part{part}})
			}
		default:
			return nil, nil, fmt.Errorf("message %d: unknown role %q", i, m.Role)
		}
	}
	return contents, cfg, nil
}

func isToolResults(c *genai.Content) bool {
	return c.Role == "user" && len(c.Parts) > 0 && c.Parts[0].FunctionResponse != nil
}

// Decode turns the first candidate into a neutral response. Pure.
func Decode(resp *genai.GenerateContentResponse) (llm.Response, error) {
	if resp == nil || len(resp.Candidates) == 0 {
		reason := ""
		if resp != nil && resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != "" {
			reason = fmt.Sprintf(" (prompt blocked: %s)", resp.PromptFeedback.BlockReason)
		}
		return llm.Response{}, errors.New("gemini returned no candidates" + reason)
	}
	cand := resp.Candidates[0]
	if cand.Content == nil {
		// A safety stop is a failure, not an empty answer.
		return llm.Response{}, fmt.Errorf("gemini returned no content (finish reason %s)", cand.FinishReason)
	}

	var out llm.Response
	var text strings.Builder
	for _, p := range cand.Content.Parts {
		switch {
		case p.Thought:
		case p.FunctionCall != nil:
			args, err := json.Marshal(p.FunctionCall.Args)
			if err != nil {
				return llm.Response{}, fmt.Errorf("tool call %s args: %w", p.FunctionCall.Name, err)
			}
			if p.FunctionCall.Args == nil {
				args = []byte("{}")
			}
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				ID: p.FunctionCall.ID, Name: p.FunctionCall.Name, Args: args, Opaque: p.ThoughtSignature,
			})
		default:
			text.WriteString(p.Text)
		}
	}
	out.Text = text.String()
	llm.FillIDs(out.ToolCalls)

	if u := resp.UsageMetadata; u != nil {
		in := int(u.PromptTokenCount)
		// Thinking is billed as output.
		outTokens := int(u.CandidatesTokenCount + u.ThoughtsTokenCount)
		out.Usage = &llm.Usage{InputTokens: in, OutputTokens: outTokens, TotalTokens: in + outTokens}
	}
	if out.Text == "" && len(out.ToolCalls) == 0 {
		if cand.FinishReason == genai.FinishReasonMaxTokens {
			return out, fmt.Errorf("gemini hit the output token limit before answering: %w", llm.ErrEmptyReply)
		}
		return out, fmt.Errorf("gemini returned %w (finish reason %s)", llm.ErrEmptyReply, cand.FinishReason)
	}
	return out, nil
}
