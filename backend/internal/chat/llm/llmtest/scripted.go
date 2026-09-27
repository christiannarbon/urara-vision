// Package llmtest holds a scripted fake llm.Model for tests.
package llmtest

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"urara-vision/backend/internal/chat/llm"
)

// Step is one scripted reply. Wait delays it, honouring the context.
type Step struct {
	Response llm.Response
	Err      error
	Wait     time.Duration
}

// Scripted returns its steps in order and records every request.
type Scripted struct {
	mu       sync.Mutex
	steps    []Step
	requests []llm.Request
}

func NewScripted(steps ...Step) *Scripted {
	return &Scripted{steps: steps}
}

func (s *Scripted) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	s.mu.Lock()
	n := len(s.requests)
	s.requests = append(s.requests, copyRequest(req))
	s.mu.Unlock()

	if n >= len(s.steps) {
		return llm.Response{}, fmt.Errorf("llmtest: no scripted step for call %d", n+1)
	}
	step := s.steps[n]
	if step.Wait > 0 {
		select {
		case <-ctx.Done():
			return llm.Response{}, ctx.Err()
		case <-time.After(step.Wait):
		}
	}
	return step.Response, step.Err
}

// Requests returns deep copies of every request so far.
func (s *Scripted) Requests() []llm.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]llm.Request, len(s.requests))
	for i, r := range s.requests {
		out[i] = copyRequest(r)
	}
	return out
}

func copyRequest(r llm.Request) llm.Request {
	out := r
	if r.Temperature != nil {
		t := *r.Temperature
		out.Temperature = &t
	}
	out.Messages = slices.Clone(r.Messages)
	for i := range out.Messages {
		out.Messages[i].ToolCalls = copyCalls(out.Messages[i].ToolCalls)
	}
	out.Tools = slices.Clone(r.Tools)
	for i := range out.Tools {
		out.Tools[i].Schema = slices.Clone(out.Tools[i].Schema)
	}
	return out
}

func copyCalls(calls []llm.ToolCall) []llm.ToolCall {
	out := slices.Clone(calls)
	for i := range out {
		out[i].Args = slices.Clone(out[i].Args)
		out[i].Opaque = slices.Clone(out[i].Opaque)
	}
	return out
}
