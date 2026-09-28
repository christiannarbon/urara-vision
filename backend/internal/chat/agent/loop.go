package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/reqctx"
	"urara-vision/backend/internal/chat/tools"
)

type guardedTool func(ctx context.Context, args json.RawMessage) (any, error)

// run is the whole turn: call the model, run the tools it asks for, and stop
// on an answer or a spent budget.
func (a *Agent) run(ctx context.Context, sid, language string, messages []llm.Message) (Answer, error) {
	started := time.Now()
	card, err := a.cards.Get(ctx, a.backend, sid)
	if err != nil {
		return Answer{}, err
	}
	system := SystemPrompt(card, language)

	specs := tools.Build(a.backend, sid)
	defs := make([]llm.ToolDef, len(specs))
	names := make([]string, len(specs))
	guarded := make(map[string]guardedTool, len(specs))
	for i, s := range specs {
		defs[i] = llm.ToolDef{Name: s.Name, Description: s.Description, Schema: s.Schema}
		names[i] = s.Name
		guarded[s.Name] = tools.Guarded(s, a.log)
	}

	var (
		replies                     []llm.Response
		results                     []any
		refused                     = map[string]bool{}
		iterations                  int
		truncated, budgetNoticeSent bool
		estPrompt, estCompletion    int
	)
	for {
		choice := llm.ToolChoiceAuto
		if iterations == 0 {
			// The first call must retrieve: answers that skipped every tool fabricated joins.
			choice = llm.ToolChoiceAny
		}
		estPrompt += estimate(system, messages)
		reply, err := a.model.Generate(ctx, llm.Request{System: system, Messages: messages, Tools: defs, ToolChoice: choice})
		// Python ends the turn on an empty reply and answers NoAnswerProduced (fence 14).
		if errors.Is(err, llm.ErrEmptyReply) {
			a.log.Warn("empty model reply", "request_id", reqctx.RequestID(ctx), "reason", err.Error())
			err = nil
		}
		if err != nil {
			return Answer{}, err
		}
		said := llm.Message{Role: llm.RoleAssistant, Text: reply.Text, ToolCalls: reply.ToolCalls}
		messages = append(messages, said)
		estCompletion += EstimateTokens([]llm.Message{said})
		replies = append(replies, reply)
		iterations++

		// The post-budget pass has had its one extra call; it answers with what is held.
		if len(reply.ToolCalls) == 0 || budgetNoticeSent {
			break
		}
		if held := estimate(system, messages); iterations >= a.opts.MaxToolIterations || held >= a.opts.MaxTurnTokens {
			reason := "iterations"
			if held >= a.opts.MaxTurnTokens {
				reason = "tokens"
			}
			a.log.Warn("turn budget spent", "request_id", reqctx.RequestID(ctx), "reason", reason,
				"estimatedTokens", held, "maxTurnTokens", a.opts.MaxTurnTokens, "iterations", iterations)
			for _, c := range reply.ToolCalls {
				refused[c.ID] = true
				messages = append(messages, llm.Message{Role: llm.RoleTool, ToolCallID: c.ID, ToolName: c.Name, Text: ToolBudgetSpentResult})
			}
			// A user turn, not system: Gemini hoists system messages to the front.
			messages = append(messages, llm.Message{Role: llm.RoleUser, Text: ToolBudgetSpent})
			truncated, budgetNoticeSent = true, true
			continue
		}

		out, err := runTools(ctx, a.log, guarded, names, reply.ToolCalls)
		if err != nil {
			return Answer{}, err
		}
		for i, c := range reply.ToolCalls {
			text, err := resultText(out[i])
			if err != nil {
				return Answer{}, err
			}
			results = append(results, out[i])
			messages = append(messages, llm.Message{Role: llm.RoleTool, ToolCallID: c.ID, ToolName: c.Name, Text: Fence(text, c.Name)})
		}
	}

	answer := NoAnswerProduced
	for i := len(replies) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(replies[i].Text); text != "" {
			answer = text
			break
		}
	}
	citations := ExtractCitations(results, answer)
	a.log.Debug("turn finalised", "snapshot_id", sid, "iterations", iterations,
		"tool_results", len(results), "citations", len(citations), "truncated", truncated)

	records := []CallRecord{}
	usage := map[string]int{}
	reported := true
	for _, r := range replies {
		for _, c := range r.ToolCalls {
			// A call the budget refused was never run.
			if !refused[c.ID] {
				records = append(records, CallRecord{Name: c.Name, Args: orEmpty(c.Args)})
			}
		}
		if r.Usage == nil {
			reported = false
			continue
		}
		for k, v := range r.Usage.Map() {
			usage[k] += v
		}
	}
	result := Answer{
		Text: answer, Citations: citations, ToolCalls: records, Iterations: iterations,
		Truncated: truncated, Model: a.opts.ModelName, Usage: usage,
		PromptTokens: estPrompt, CompletionTokens: estCompletion, TokensEstimated: true,
	}
	// Never mixed: either every call reported, or both counts are estimates.
	if reported && len(usage) > 0 {
		result.PromptTokens, result.CompletionTokens, result.TokensEstimated = usage["input_tokens"], usage["output_tokens"], false
	}
	result.LatencyMS = int(time.Since(started).Milliseconds())
	return result, nil
}

// estimate holds the system prompt against the turn, as Python's message list did.
func estimate(system string, messages []llm.Message) int {
	return EstimateTokens(append([]llm.Message{{Text: system}}, messages...))
}

// runTools runs the calls in parallel, results by call index. The first error
// cancels the rest and ends the turn.
func runTools(ctx context.Context, log *slog.Logger, guarded map[string]guardedTool, names []string, calls []llm.ToolCall) ([]any, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make([]any, len(calls))
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	for i, c := range calls {
		run, ok := guarded[c.Name]
		if !ok {
			// LangGraph's ToolNode wording for a tool that does not exist.
			advice := "Error: " + c.Name + " is not a valid tool, try one of [" + strings.Join(names, ", ") + "]."
			var args any = string(c.Args)
			if json.Valid(c.Args) {
				args = c.Args
			}
			log.Debug("tool call", "tool", c.Name, "tool_args", args, "duration_ms", 0, "returned_error", true, "error", advice)
			out[i] = advice
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := run(ctx, c.Args)
			if err != nil {
				once.Do(func() { firstErr = err; cancel() })
				return
			}
			out[i] = r
		}()
	}
	wg.Wait()
	return out, firstErr
}

// resultText is what the model reads: advice as is, anything else as JSON.
func resultText(r any) (string, error) {
	if s, ok := r.(string); ok {
		return s, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}
