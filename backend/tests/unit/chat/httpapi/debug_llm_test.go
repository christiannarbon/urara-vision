package httpapi_test

import (
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/llm/llmtest"
)

func probe(t *testing.T, model llm.Model, timeout time.Duration) (map[string]any, int, *logs) {
	t.Helper()
	l := &logs{}
	h := httpapi.New(httpapi.Deps{
		Settings:     chatSettings(),
		Log:          slog.New(slog.NewJSONHandler(l, nil)),
		Model:        model,
		ProbeTimeout: timeout,
	}).Handler()
	rec := get(h, "/debug/llm", "trace-llm")
	return decode(t, rec), rec.Code, l
}

func TestDebugLLMAnswers(t *testing.T) {
	model := llmtest.NewScripted(llmtest.Step{Response: llm.Response{Text: "pong"}})
	body, code, _ := probe(t, model, 0)

	if code != http.StatusOK {
		t.Fatalf("status %d, body %v", code, body)
	}
	if _, ok := body["latencyMs"].(float64); !ok {
		t.Errorf("latencyMs = %v", body["latencyMs"])
	}
	delete(body, "latencyMs")
	want := map[string]any{"text": "pong", "provider": "vertex", "model": "gemini-2.5-flash", "location": "us-central1"}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body = %v", body)
	}

	reqs := model.Requests()
	if len(reqs) != 1 || len(reqs[0].Tools) != 0 || !reflect.DeepEqual(reqs[0].Messages, []llm.Message{
		{Role: llm.RoleUser, Text: "Reply with exactly: pong"},
	}) {
		t.Errorf("requests = %+v", reqs)
	}
}

// The reason is logged redacted: no prompt, no key.
func TestDebugLLMProviderErrorIs502AndLoggedRedacted(t *testing.T) {
	model := llmtest.NewScripted(llmtest.Step{
		Err: errors.New("429 quota exceeded; prompt was Reply with exactly: pong; key AIzaSyA0123456789abcdef"),
	})
	body, code, l := probe(t, model, 0)

	want := map[string]any{
		"detail":    "the language model provider did not answer; see the service logs",
		"requestId": "trace-llm",
	}
	if code != http.StatusBadGateway || !reflect.DeepEqual(body, want) {
		t.Errorf("%d %v", code, body)
	}
	failed := probeFailure(t, l)
	for k, v := range failed {
		if s, ok := v.(string); ok && (strings.Contains(s, "Reply with exactly") || strings.Contains(s, "AIza")) {
			t.Errorf("unredacted %s: %q", k, s)
		}
	}
	reason, _ := failed["reason"].(string)
	if failed["provider"] != "vertex" || failed["model"] != "gemini-2.5-flash" || failed["timeout"] != false ||
		!strings.Contains(reason, "429 quota exceeded") || !strings.Contains(reason, "[question]") {
		t.Errorf("probe failure log = %v", failed)
	}
}

func probeFailure(t *testing.T, l *logs) map[string]any {
	t.Helper()
	for _, line := range l.lines(t) {
		if line["msg"] == "llm probe failed" {
			return line
		}
	}
	t.Fatal("no llm probe failed log")
	return nil
}

func TestDebugLLMGivesUpAtTheProbeTimeout(t *testing.T) {
	model := llmtest.NewScripted(llmtest.Step{Wait: 20 * time.Second, Response: llm.Response{Text: "late"}})
	start := time.Now()
	body, code, l := probe(t, model, 20*time.Millisecond)

	if code != http.StatusBadGateway || body["detail"] == nil {
		t.Errorf("%d %v", code, body)
	}
	if failed := probeFailure(t, l); failed["timeout"] != true {
		t.Errorf("probe failure log = %v", failed)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("took %s", took)
	}
}
