package agent

import (
	"encoding/json"
	"unicode/utf8"

	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/model"
)

// TruncateHistory keeps the last limit messages, then drops leading ones until
// a user message leads: an answer with no question above it reads as the
// model talking to itself.
func TruncateHistory(msgs []model.Message, limit int) []model.Message {
	if limit <= 0 {
		return []model.Message{}
	}
	kept := msgs[max(0, len(msgs)-limit):]
	for len(kept) > 0 && kept[0].Role != "user" {
		kept = kept[1:]
	}
	return kept
}

// ToLLMMessages converts stored turns; roles other than user and assistant are
// dropped, so a stored system message cannot fight the rebuilt prompt.
func ToLLMMessages(msgs []model.Message) []llm.Message {
	out := []llm.Message{}
	for _, m := range msgs {
		switch m.Role {
		case "user":
			out = append(out, llm.Message{Role: llm.RoleUser, Text: m.Content})
		case "assistant":
			out = append(out, llm.Message{Role: llm.RoleAssistant, Text: m.Content})
		}
	}
	return out
}

// EstimateTokens is characters / 4, not a tokeniser: an approximate limit is
// not worth a dependency.
func EstimateTokens(msgs []llm.Message) int {
	chars := 0
	for _, m := range msgs {
		chars += utf8.RuneCountInString(m.Text)
		if len(m.ToolCalls) > 0 {
			args := make([]json.RawMessage, len(m.ToolCalls))
			for i, c := range m.ToolCalls {
				args[i] = orEmpty(c.Args)
			}
			b, _ := json.Marshal(args)
			chars += utf8.RuneCount(b)
		}
	}
	return chars / 4
}

func orEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}
