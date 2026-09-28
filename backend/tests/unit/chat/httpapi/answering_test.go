package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/agent"
	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/model"
)

// fakeAgent records each turn and answers with answer or err. A wait holds
// the turn until its context ends.
type fakeAgent struct {
	answer agent.Answer
	err    error
	wait   bool
	calls  []struct{ question, snapshotID, language string }
}

func (a *fakeAgent) Answer(ctx context.Context, question, snapshotID string, _ []model.Message, language string) (agent.Answer, error) {
	a.calls = append(a.calls, struct{ question, snapshotID, language string }{question, snapshotID, language})
	if a.wait {
		<-ctx.Done()
		return agent.Answer{}, ctx.Err()
	}
	return a.answer, a.err
}

func answerServer(t *testing.T, a *fakeAgent, store *toolStore, timeout time.Duration) (http.Handler, *logs) {
	t.Helper()
	settings := chatSettings()
	settings.MaxQuestionChars = 4000
	settings.AnswerTimeout = timeout
	l := &logs{}
	return httpapi.New(httpapi.Deps{
		Settings: settings,
		Log:      slog.New(slog.NewJSONHandler(l, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Tools:    store,
		Agent:    a,
	}).Handler(), l
}

func ask(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/debug/answer", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "trace-answer")
	return serve(h, req)
}

func answerBody(snapshot, question string) string {
	b, _ := json.Marshal(map[string]string{"snapshotId": snapshot, "question": question})
	return string(b)
}

func TestAnswerQuestionGoldens(t *testing.T) {
	a := &fakeAgent{}
	h, _ := answerServer(t, a, &toolStore{t: t}, time.Minute)
	assertGolden(t, "errors/question-empty.json", ask(t, h, answerBody("snap-1", "   ")))
	assertGolden(t, "errors/question-long.json", ask(t, h, answerBody("snap-1", strings.Repeat("a", 4001))))
	assertGolden(t, "errors/snapshot-unknown.json", ask(t, h, answerBody("nosuch", "hi")))
	if len(a.calls) != 0 {
		t.Errorf("the agent ran %d times", len(a.calls))
	}
}

// Counted in characters: 4000 Japanese characters are 12000 bytes.
func TestAnswerQuestionLimitCountsCharacters(t *testing.T) {
	a := &fakeAgent{answer: agent.Answer{Text: "ok"}}
	h, _ := answerServer(t, a, &toolStore{t: t}, time.Minute)
	if rec := ask(t, h, answerBody("snap-1", strings.Repeat("注", 4000))); rec.Code != 200 {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if rec := ask(t, h, answerBody("snap-1", strings.Repeat("注", 4001))); rec.Code != 400 ||
		!strings.Contains(rec.Body.String(), `is 4001 characters, over the 4000`) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

// A bad question costs no backend call.
func TestAnswerQuestionIsCleanedBeforeTheSnapshotResolves(t *testing.T) {
	store := &toolStore{t: t}
	h, _ := answerServer(t, &fakeAgent{}, store, time.Minute)
	if rec := ask(t, h, answerBody("nosuch", "")); rec.Code != 400 || len(store.resolved) != 0 {
		t.Errorf("%d, resolved %v", rec.Code, store.resolved)
	}
}

func TestAnswerQuestionIsTrimmedAndTheSnapshotResolved(t *testing.T) {
	a := &fakeAgent{answer: agent.Answer{Text: "ok"}}
	h, _ := answerServer(t, a, &toolStore{t: t}, time.Minute)
	ask(t, h, answerBody("latest", "  what is fact_orders?\n"))
	if len(a.calls) != 1 || a.calls[0].question != "what is fact_orders?" || a.calls[0].snapshotID != "real-id" {
		t.Errorf("calls %+v", a.calls)
	}
}

func TestAnswerLanguageIsNormalised(t *testing.T) {
	for in, want := range map[string]string{"fr": "EN", " ja ": "JA", "JA": "JA", "": "EN"} {
		a := &fakeAgent{answer: agent.Answer{Text: "ok"}}
		h, _ := answerServer(t, a, &toolStore{t: t}, time.Minute)
		body, _ := json.Marshal(map[string]string{"snapshotId": "snap-1", "question": "q", "language": in})
		ask(t, h, string(body))
		if len(a.calls) != 1 || a.calls[0].language != want {
			t.Errorf("%q reached the agent as %+v", in, a.calls)
		}
	}
	// Omitted entirely.
	a := &fakeAgent{answer: agent.Answer{Text: "ok"}}
	h, _ := answerServer(t, a, &toolStore{t: t}, time.Minute)
	ask(t, h, answerBody("snap-1", "q"))
	if a.calls[0].language != "EN" {
		t.Errorf("default %q", a.calls[0].language)
	}
}

func TestAnswerRequestShape(t *testing.T) {
	h, _ := answerServer(t, &fakeAgent{answer: agent.Answer{Text: "ok"}}, &toolStore{t: t}, time.Minute)
	for body, want := range map[string]string{
		`{"question":"q"}`:                                 "snapshotId",
		`{"snapshotId":"snap-1"}`:                          "question",
		`{"snapshotId":"snap-1","question":"q","extra":1}`: "extra",
		`{"snapshotId":"snap-1","question":5}`:             "question",
	} {
		rec := ask(t, h, body)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"field":"`+want+`"`) {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := ask(t, h, `{"snapshot_id":"snap-1","question":"q"}`); rec.Code != 200 {
		t.Errorf("snake case: %d %s", rec.Code, rec.Body)
	}
}

func TestAnswerSuccessHasEveryKey(t *testing.T) {
	full := agent.Answer{
		Text: "fact_orders is one per order.", Citations: []string{"ordering/fact_orders"},
		ToolCalls:  []agent.CallRecord{{Name: "get_tables", Args: json.RawMessage(`{"ids":["ordering/fact_orders"]}`)}},
		Iterations: 2, Model: "gemini-2.5-flash", LatencyMS: 812,
		Usage: map[string]int{"input_tokens": 10, "output_tokens": 2, "total_tokens": 12},
	}
	h, l := answerServer(t, &fakeAgent{answer: full}, &toolStore{t: t}, time.Minute)
	rec := ask(t, h, answerBody("snap-1", "what is fact_orders?"))
	want := map[string]any{
		"text": "fact_orders is one per order.", "citations": []any{"ordering/fact_orders"},
		"toolCalls":  []any{map[string]any{"name": "get_tables", "args": map[string]any{"ids": []any{"ordering/fact_orders"}}}},
		"iterations": 2.0, "truncated": false, "model": "gemini-2.5-flash", "latencyMs": 812.0,
		"usage": map[string]any{"input_tokens": 10.0, "output_tokens": 2.0, "total_tokens": 12.0},
	}
	if got := decode(t, rec); rec.Code != 200 || !reflect.DeepEqual(got, want) {
		t.Errorf("%d %v", rec.Code, got)
	}

	var turn map[string]any
	for _, line := range l.lines(t) {
		if line["msg"] == "turn answered" {
			turn = line
		}
	}
	for k, v := range map[string]any{
		"event": "turn", "outcome": "answered", "requestId": "trace-answer", "conversationId": nil, "snapshotId": "snap-1",
		"model": "gemini-2.5-flash", "toolCalls": 1.0, "tools": []any{"get_tables"}, "iterations": 2.0, "citations": 1.0,
		"latencyMs": 812.0, "truncated": false, "tokensEstimated": false, "promptTokens": 0.0, "completionTokens": 0.0,
	} {
		if got, ok := turn[k]; !ok || !reflect.DeepEqual(got, v) {
			t.Errorf("turn log %s = %v, want %v", k, got, v)
		}
	}
}

// Callers index into these, so they are [] and {} rather than null.
func TestAnswerEmptyCollectionsAreNotNull(t *testing.T) {
	h, l := answerServer(t, &fakeAgent{answer: agent.Answer{Text: "ok"}}, &toolStore{t: t}, time.Minute)
	rec := ask(t, h, answerBody("snap-1", "q"))
	for _, want := range []string{`"citations":[]`, `"toolCalls":[]`, `"usage":{}`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("missing %s in %s", want, rec.Body)
		}
	}
	for _, line := range l.lines(t) {
		if line["msg"] == "turn answered" && !reflect.DeepEqual(line["tools"], []any{}) {
			t.Errorf("tools = %v", line["tools"])
		}
	}
}

func TestAnswerFailuresMapToTheRightStatus(t *testing.T) {
	for _, c := range []struct {
		name   string
		agent  *fakeAgent
		status int
		body   string
	}{
		{"turn deadline", &fakeAgent{wait: true}, 502, "the language model did not answer"},
		{"deadline error", &fakeAgent{err: context.DeadlineExceeded}, 502, "the language model did not answer"},
		{"provider error", &fakeAgent{err: errors.New("429 quota")}, 502, "the language model did not answer"},
		{"backend not found", &fakeAgent{err: &apiclient.Error{Status: 404, Message: "gone"}}, 404, "not found"},
		{"backend forbidden", &fakeAgent{err: &apiclient.Error{Status: 403, Message: "no"}}, 403, "not allowed"},
		{"backend down", &fakeAgent{err: &apiclient.Error{Status: 500, Message: "boom"}}, 502, "the model store is unavailable"},
	} {
		h, _ := answerServer(t, c.agent, &toolStore{t: t}, 20*time.Millisecond)
		rec := ask(t, h, answerBody("snap-1", "q"))
		if e, _ := decode(t, rec)["error"].(string); rec.Code != c.status || e != c.body || rec.Header().Get("X-Request-Id") == "" {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body)
		}
	}
}

// A provider message can quote the prompt and a key back; neither reaches a log or the caller.
func TestAnswerProviderErrorsAreRedacted(t *testing.T) {
	question := "what is the grain of ordering/fact_orders?"
	leak := "400 invalid request: " + question + " key AIzaSyA0123456789abcdef"
	h, l := answerServer(t, &fakeAgent{err: errors.New(leak)}, &toolStore{t: t}, time.Minute)
	rec := ask(t, h, answerBody("snap-1", question))
	if strings.Contains(rec.Body.String(), "AIza") || strings.Contains(rec.Body.String(), question) {
		t.Errorf("response leaks: %s", rec.Body)
	}

	var failed, called map[string]any
	for _, line := range l.lines(t) {
		for k, v := range line {
			if s, ok := v.(string); ok && (strings.Contains(s, "AIza") || strings.Contains(s, question)) {
				t.Errorf("log %q leaks under %s: %q", line["msg"], k, s)
			}
		}
		switch line["msg"] {
		case "turn failed":
			failed = line
		case "language model call failed":
			called = line
		}
	}
	if failed["level"] != "WARN" || failed["event"] != "turn" || failed["outcome"] != "failed" || failed["error"] != "*errors.errorString" ||
		failed["snapshotId"] != "snap-1" || failed["requestId"] != "trace-answer" || failed["conversationId"] != nil {
		t.Errorf("turn failed = %v", failed)
	}
	if reason, _ := called["reason"].(string); called["level"] != "ERROR" || !strings.Contains(reason, "400 invalid request: [question]") ||
		!strings.Contains(reason, "[key]") {
		t.Errorf("language model call failed = %v", called)
	}
}

// A caller hanging up is not a provider failure.
func TestAnswerACallerHangingUpIsNotAProviderFailure(t *testing.T) {
	h, l := answerServer(t, &fakeAgent{wait: true}, &toolStore{t: t}, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("POST", "/debug/answer", strings.NewReader(answerBody("snap-1", "q"))).WithContext(ctx)
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	serve(h, req)
	msgs := []any{}
	for _, line := range l.lines(t) {
		msgs = append(msgs, line["msg"])
	}
	if slices.Contains(msgs, "language model call failed") || !slices.Contains(msgs, "request cancelled by the caller") {
		t.Errorf("logs %v", msgs)
	}
}

// The type names the failure; the message could quote the prompt.
func TestAnswerTurnFailedNamesTheInnermostError(t *testing.T) {
	for want, err := range map[string]error{
		"DeadlineExceeded":    fmt.Errorf("gemini: %w", context.DeadlineExceeded),
		"*apiclient.Error":    fmt.Errorf("anthropic: %w", &apiclient.Error{Status: 500, Message: "boom"}),
		"*errors.errorString": errors.New("plain"),
	} {
		h, l := answerServer(t, &fakeAgent{err: err}, &toolStore{t: t}, time.Minute)
		ask(t, h, answerBody("snap-1", "q"))
		var got any
		for _, line := range l.lines(t) {
			if line["msg"] == "turn failed" {
				got = line["error"]
			}
		}
		if got != want {
			t.Errorf("error = %v, want %s", got, want)
		}
	}
}

// A deadline on the request itself is the turn running out, not a bad backend response.
func TestAnswerARequestDeadlineIsAProviderFailure(t *testing.T) {
	h, l := answerServer(t, &fakeAgent{wait: true}, &toolStore{t: t}, time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest("POST", "/debug/answer", strings.NewReader(answerBody("snap-1", "q"))).WithContext(ctx)
	rec := serve(h, req)
	if e, _ := decode(t, rec)["error"].(string); rec.Code != 502 || e != "the language model did not answer" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	for _, line := range l.lines(t) {
		if m, _ := line["msg"].(string); strings.Contains(m, "did not match") {
			t.Errorf("logged %q", m)
		}
	}
}
