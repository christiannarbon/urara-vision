package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/llm/llmtest"
)

func TestStepsComeBackInOrder(t *testing.T) {
	boom := errors.New("boom")
	s := llmtest.NewScripted(
		llmtest.Step{Response: llm.Response{Text: "one"}},
		llmtest.Step{Err: boom},
	)
	ctx := context.Background()
	if r, err := s.Generate(ctx, llm.Request{}); err != nil || r.Text != "one" {
		t.Errorf("call 1 = %+v, %v", r, err)
	}
	if _, err := s.Generate(ctx, llm.Request{}); !errors.Is(err, boom) {
		t.Errorf("call 2 = %v", err)
	}
	if _, err := s.Generate(ctx, llm.Request{}); err == nil || !strings.Contains(err.Error(), "call 3") {
		t.Errorf("call 3 past the script = %v", err)
	}
	if n := len(s.Requests()); n != 3 {
		t.Errorf("%d requests recorded", n)
	}
}

func TestRecordedRequestsAreCopies(t *testing.T) {
	temp := 0.2
	req := llm.Request{
		Temperature: &temp,
		Messages: []llm.Message{{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "t", Args: json.RawMessage(`{"a":1}`), Opaque: []byte("sig")},
		}}},
		Tools: []llm.ToolDef{{Name: "t", Schema: json.RawMessage(`{"type":"object"}`)}},
	}
	s := llmtest.NewScripted(llmtest.Step{})
	if _, err := s.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	// Mutate everything the caller still holds.
	temp = 0.9
	req.Messages[0].Text = "changed"
	req.Messages[0].ToolCalls[0].Args[1] = 'X'
	req.Messages[0].ToolCalls[0].Opaque[0] = 'X'
	req.Tools[0].Schema[1] = 'X'

	got := s.Requests()[0]
	call := got.Messages[0].ToolCalls[0]
	if *got.Temperature != 0.2 || got.Messages[0].Text != "" || string(call.Args) != `{"a":1}` ||
		string(call.Opaque) != "sig" || string(got.Tools[0].Schema) != `{"type":"object"}` {
		t.Errorf("recorded request changed with the caller's: %+v", got)
	}

	// And a copy handed out cannot change the record.
	s.Requests()[0].Messages[0].Text = "changed"
	if s.Requests()[0].Messages[0].Text != "" {
		t.Error("Requests() returned the record itself")
	}
}

func TestAWaitHonoursTheContext(t *testing.T) {
	s := llmtest.NewScripted(llmtest.Step{Wait: time.Minute, Response: llm.Response{Text: "late"}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := s.Generate(ctx, llm.Request{})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Errorf("err = %v after %v", err, time.Since(started))
	}
}
