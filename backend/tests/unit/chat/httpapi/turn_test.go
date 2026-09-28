// Ported from chat/tests/unit/test_turn.py and the turn cases of test_concurrency.py.
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
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/agent"
	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/model"
)

const providerText = "429 quota exceeded for project, key AIzaSyA0123456789abcdef"

var turnFields = []string{
	"requestId", "conversationId", "snapshotId", "model", "promptTokens", "completionTokens",
	"tokensEstimated", "toolCalls", "tools", "iterations", "citations", "latencyMs", "truncated",
}

type turnCall struct {
	question, snapshotID, language string
	history                        []model.Message
}

// turnAgent notes each run in the fake backend's steps, so the order is one log.
type turnAgent struct {
	backend *fakeBackend
	answer  agent.Answer
	err     error
	hold    time.Duration

	mu     sync.Mutex
	calls  []turnCall
	events []string // enter:<question> and exit:<question>
}

func (a *turnAgent) Answer(_ context.Context, question, snapshotID string, history []model.Message, language string) (agent.Answer, error) {
	a.backend.note("agent")
	a.mu.Lock()
	a.calls = append(a.calls, turnCall{question, snapshotID, language, slices.Clone(history)})
	a.events = append(a.events, "enter:"+question)
	a.mu.Unlock()
	time.Sleep(a.hold)
	a.mu.Lock()
	a.events = append(a.events, "exit:"+question)
	a.mu.Unlock()
	return a.answer, a.err
}

func defaultAnswer() agent.Answer {
	return agent.Answer{
		Text:       "fact_orders is one row per order.",
		Citations:  []string{"ordering/fact_orders"},
		ToolCalls:  []agent.CallRecord{{Name: "get_tables", Args: json.RawMessage(`{"ids":["ordering/fact_orders"]}`)}},
		Iterations: 2, Model: "gemini-2.5-flash", LatencyMS: 3412,
		Usage:        map[string]int{"input_tokens": 900, "output_tokens": 120},
		PromptTokens: 900, CompletionTokens: 120,
	}
}

type turnEnv struct {
	f   *fakeBackend
	a   *turnAgent
	h   http.Handler
	log *logs
}

func newTurnEnv(t *testing.T, f *fakeBackend, tune func(*config.Settings)) *turnEnv {
	t.Helper()
	if f == nil {
		f = &fakeBackend{}
	}
	f.enabled = true
	settings := chatSettings()
	settings.MaxQuestionChars = 4000
	settings.AnswerTimeout = time.Minute
	settings.MaxConversationTurns = 50
	settings.MaxConcurrentTurns = 4
	settings.TurnAdmissionWait = time.Second
	if tune != nil {
		tune(settings)
	}
	a := &turnAgent{backend: f, answer: defaultAnswer()}
	l := &logs{}
	h := httpapi.New(httpapi.Deps{
		Settings: settings,
		Log:      slog.New(slog.NewJSONHandler(l, nil)),
		Backend:  f,
		Agent:    a,
		Clock:    newClock().now,
	}).Handler()
	return &turnEnv{f: f, a: a, h: h, log: l}
}

func (e *turnEnv) turn(cid, body string) *httptest.ResponseRecorder {
	return post(e.h, "/api/chat/conversations/"+cid+"/turn", body)
}

func (e *turnEnv) ask(question string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]string{"question": question})
	return e.turn("conv-1", string(b))
}

func (e *turnEnv) roles() []string {
	var out []string
	for _, m := range e.f.appended {
		out = append(out, m.role)
	}
	return out
}

func (e *turnEnv) logLine(t *testing.T, msg string) map[string]any {
	t.Helper()
	for _, line := range e.log.lines(t) {
		if line["msg"] == msg {
			return line
		}
	}
	t.Fatalf("no %q log", msg)
	return nil
}

func assistantTurns(n int) []model.Message {
	var out []model.Message
	for i := range n {
		out = append(out,
			model.Message{Ordinal: 2 * i, Role: "user", Content: fmt.Sprintf("q%d", i)},
			model.Message{Ordinal: 2*i + 1, Role: "assistant", Content: fmt.Sprintf("a%d", i)})
	}
	return out
}

func TestTurnStepsRunInOrder(t *testing.T) {
	e := newTurnEnv(t, &fakeBackend{title: new(string)}, nil)
	if rec := e.ask("What is fact_orders?"); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if want := []string{"get", "append:user", "agent", "append:assistant", "patch"}; !reflect.DeepEqual(e.f.steps, want) {
		t.Errorf("steps %v", e.f.steps)
	}
}

func TestTurnSnapshotComesFromTheConversation(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.ask("the grain of fact_orders?")
	if e.a.calls[0].snapshotID != "5b0c1a52-3d8e-4f7a-9c21-6e4d8b2f1a90" {
		t.Errorf("snapshot %q", e.a.calls[0].snapshotID)
	}
}

func TestTurnRequestShape(t *testing.T) {
	for body, field := range map[string]string{
		`{"question":"q","snapshotId":"other"}`: "snapshotId",
		`{"language":"EN"}`:                     "question",
	} {
		e := newTurnEnv(t, nil, nil)
		rec := e.turn("conv-1", body)
		if rec.Code != http.StatusBadRequest || firstField(t, rec)["field"] != field {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
		if len(e.a.calls) != 0 || len(e.f.steps) != 0 {
			t.Errorf("%s: ran %v", body, e.f.steps)
		}
	}
}

func TestTurnProviderFailureLeavesTheQuestionStored(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.a.err = errors.New(providerText)
	rec := e.ask("the grain of fact_orders?")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d", rec.Code)
	}
	if !reflect.DeepEqual(e.roles(), []string{"user"}) || e.f.appended[0].content != "the grain of fact_orders?" {
		t.Errorf("appended %+v", e.f.appended)
	}
	body := decoded(t, rec)
	if body["error"] != httpapi.MsgProviderFailed || strings.Contains(rec.Body.String(), "quota") ||
		strings.Contains(rec.Body.String(), "AIza") {
		t.Errorf("body %s", rec.Body)
	}
}

func TestTurnBackendFailureIsNotReportedAsTheModel(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.a.err = &apiclient.Error{Status: 500, Message: "neo4j is down"}
	rec := e.ask("q")
	if rec.Code != http.StatusBadGateway || decoded(t, rec)["error"] == httpapi.MsgProviderFailed {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestTurnLostAnswerIsLogged(t *testing.T) {
	e := newTurnEnv(t, &fakeBackend{appendErr: &apiclient.Error{Status: 500, Message: "messages table is on fire"}}, nil)
	if rec := e.ask("what is the grain?"); rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d", rec.Code)
	}
	if e.f.appended[0].role != "user" || e.f.appended[0].content != "what is the grain?" {
		t.Errorf("appended %+v", e.f.appended)
	}
	line := e.logLine(t, "the answer could not be stored")
	usage := map[string]any{"input_tokens": float64(900), "output_tokens": float64(120)}
	if line["answer"] != "fact_orders is one row per order." || line["conversation_id"] != "conv-1" ||
		!reflect.DeepEqual(line["usage"], usage) || line["request_id"] == "" {
		t.Errorf("log %v", line)
	}
	if slices.Contains(e.f.steps, "patch") {
		t.Error("titled a turn whose answer was lost")
	}
}

func TestTurnHistoryReachesTheAgentInOrder(t *testing.T) {
	e := newTurnEnv(t, &fakeBackend{messages: []model.Message{
		{Ordinal: 0, Role: "user", Content: "what is fact_orders?"},
		{Ordinal: 1, Role: "assistant", Content: "a fact table."},
	}}, nil)
	e.ask("and what joins to it?")
	h := e.a.calls[0].history
	if len(h) != 2 || h[0].Role != "user" || h[1].Role != "assistant" || h[0].Ordinal != 0 || h[1].Ordinal != 1 {
		t.Errorf("history %+v", h)
	}
}

func TestTurnNewQuestionIsNotInTheHistory(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.ask("and what joins to it?")
	call := e.a.calls[0]
	if call.question != "and what joins to it?" {
		t.Errorf("question %q", call.question)
	}
	for _, m := range call.history {
		if m.Content == call.question {
			t.Error("the new question is in its own history")
		}
	}
}

func TestTurnStoredMeta(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.ask("q")
	if !reflect.DeepEqual(e.roles(), []string{"user", "assistant"}) {
		t.Fatalf("roles %v", e.roles())
	}
	stored := e.f.appended[1]
	if !reflect.DeepEqual(stored.citations, []string{"ordering/fact_orders"}) {
		t.Errorf("citations %v", stored.citations)
	}
	// Round-tripped, as the backend would store it.
	raw, _ := json.Marshal(stored.meta)
	var meta map[string]any
	_ = json.Unmarshal(raw, &meta)
	for _, k := range turnFields {
		if _, ok := meta[k]; !ok {
			t.Errorf("meta has no %s", k)
		}
	}
	calls, _ := meta["toolCalls"].([]any)
	if len(calls) != 1 || calls[0].(map[string]any)["name"] != "get_tables" {
		t.Errorf("toolCalls %v", meta["toolCalls"])
	}
	usage, _ := meta["usage"].(map[string]any)
	if usage["input_tokens"] != float64(900) || meta["outcome"] != "answered" || meta["model"] != "gemini-2.5-flash" ||
		meta["iterations"] != float64(2) || meta["latencyMs"] != float64(3412) || meta["conversationId"] != "conv-1" {
		t.Errorf("meta %v", meta)
	}
}

func TestTurnResponse(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	rec := e.ask("q")
	body := decoded(t, rec)
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"assistantMessage", "conversationId", "latencyMs", "model", "toolCalls", "truncated", "userMessage"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("keys %v", keys)
	}
	user, assistant := body["userMessage"].(map[string]any), body["assistantMessage"].(map[string]any)
	if body["conversationId"] != "conv-1" || user["role"] != "user" || assistant["role"] != "assistant" ||
		user["ordinal"] != float64(0) || assistant["ordinal"] != float64(1) {
		t.Errorf("body %v", body)
	}
	if !reflect.DeepEqual(assistant["citations"], []any{"ordering/fact_orders"}) ||
		!reflect.DeepEqual(user["citations"], []any{}) || !reflect.DeepEqual(user["meta"], map[string]any{}) {
		t.Errorf("messages %v %v", user, assistant)
	}
	if body["toolCalls"].([]any)[0].(map[string]any)["name"] != "get_tables" || body["truncated"] != false ||
		body["latencyMs"] != float64(3412) || body["model"] != "gemini-2.5-flash" {
		t.Errorf("body %v", body)
	}
}

func TestTurnEmptyToolCallsAreAList(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.a.answer = agent.Answer{Text: "ok"}
	body := decoded(t, e.ask("q"))
	if !reflect.DeepEqual(body["toolCalls"], []any{}) {
		t.Errorf("toolCalls %v", body["toolCalls"])
	}
}

func TestTurnBadQuestionTouchesNothing(t *testing.T) {
	for question, want := range map[string][]string{
		"   ":                     {"question"},
		strings.Repeat("x", 4001): {"4000", "4001"},
	} {
		e := newTurnEnv(t, nil, nil)
		rec := e.ask(question)
		detail, _ := decoded(t, rec)["detail"].(string)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status %d", rec.Code)
		}
		for _, w := range want {
			if !strings.Contains(detail, w) {
				t.Errorf("detail %q lacks %s", detail, w)
			}
		}
		if len(e.f.convUsers) != 0 {
			t.Errorf("the backend saw %v", e.f.steps)
		}
	}
}

func TestTurnUnknownConversationIs404(t *testing.T) {
	e := newTurnEnv(t, &fakeBackend{convErr: &apiclient.Error{Status: 404, Message: "conversation not found"}}, nil)
	rec := e.turn("nope", `{"question":"hi"}`)
	if rec.Code != http.StatusNotFound || len(e.f.appended) != 0 {
		t.Errorf("%d, appended %v", rec.Code, e.f.appended)
	}
}

func TestTurnUnknownLanguageFallsBack(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	if rec := e.turn("conv-1", `{"question":"q","language":"KL"}`); rec.Code != http.StatusOK || e.a.calls[0].language != "EN" {
		t.Errorf("%d %+v", rec.Code, e.a.calls)
	}
}

func TestTurnConversationLimit(t *testing.T) {
	e := newTurnEnv(t, &fakeBackend{messages: assistantTurns(50)}, nil)
	rec := e.ask("one more?")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d", rec.Code)
	}
	if d := decoded(t, rec)["detail"]; d != "this conversation has reached its limit of 50 turns; start a new conversation to keep asking" {
		t.Errorf("detail %q", d)
	}
	if len(e.a.calls) != 0 || len(e.f.appended) != 0 {
		t.Error("the turn ran")
	}

	if rec := newTurnEnv(t, &fakeBackend{messages: assistantTurns(49)}, nil).ask("last one?"); rec.Code != http.StatusOK {
		t.Errorf("49 turns: %d", rec.Code)
	}
	tuned := newTurnEnv(t, &fakeBackend{messages: assistantTurns(2)}, func(s *config.Settings) { s.MaxConversationTurns = 2 })
	if rec := tuned.ask("third?"); rec.Code != http.StatusConflict {
		t.Errorf("limit 2: %d", rec.Code)
	}
}

func TestTurnLogLineCarriesEveryField(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.ask("q")
	line := e.logLine(t, "turn answered")
	for _, k := range turnFields {
		if _, ok := line[k]; !ok {
			t.Errorf("no %s", k)
		}
	}
	if line["conversationId"] != "conv-1" || line["snapshotId"] != "5b0c1a52-3d8e-4f7a-9c21-6e4d8b2f1a90" ||
		line["promptTokens"] != float64(900) || line["completionTokens"] != float64(120) ||
		line["toolCalls"] != float64(1) || !reflect.DeepEqual(line["tools"], []any{"get_tables"}) || line["requestId"] == "" {
		t.Errorf("line %v", line)
	}
}

func TestTurnEstimatedCountsAreMarked(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.a.answer = agent.Answer{Text: "ok", PromptTokens: 400, TokensEstimated: true}
	e.ask("q")
	if line := e.logLine(t, "turn answered"); line["tokensEstimated"] != true {
		t.Errorf("tokensEstimated %v", line["tokensEstimated"])
	}
}

func TestTurnFailureWritesACostLine(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.a.err = errors.New(providerText)
	e.ask("q")
	line := e.logLine(t, "turn failed")
	if line["outcome"] != "failed" || line["conversationId"] != "conv-1" || line["error"] == "" {
		t.Errorf("line %v", line)
	}
	raw, _ := json.Marshal(line)
	if strings.Contains(string(raw), providerText) {
		t.Error("the provider's text is in the cost line")
	}
}

func TestTurnProviderFailureLogIsRedacted(t *testing.T) {
	question := "what is the grain of fact_orders?"
	e := newTurnEnv(t, nil, nil)
	e.a.err = errors.New("400 invalid request: " + question + " key AIzaSyA0123456789abcdef")
	e.ask(question)
	reason, _ := e.logLine(t, "language model call failed")["reason"].(string)
	if !strings.Contains(reason, "400 invalid request: [question]") || strings.Contains(reason, question) ||
		strings.Contains(reason, "AIza") {
		t.Errorf("reason %q", reason)
	}
}

func TestTurnSameConversationRunsOneAtATime(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.a.hold = 30 * time.Millisecond
	var wg sync.WaitGroup
	for _, q := range []string{"one", "two"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rec := e.ask(q); rec.Code != http.StatusOK {
				t.Errorf("%s: %d", q, rec.Code)
			}
		}()
	}
	wg.Wait()
	ev := e.a.events
	if len(ev) != 4 || !strings.HasPrefix(ev[0], "enter:") || ev[1] != "exit:"+strings.TrimPrefix(ev[0], "enter:") {
		t.Errorf("events %v", ev)
	}
}

func TestTurnDifferentConversationsOverlap(t *testing.T) {
	e := newTurnEnv(t, nil, nil)
	e.a.hold = 50 * time.Millisecond
	var wg sync.WaitGroup
	for _, cid := range []string{"conv-1", "conv-2"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.turn(cid, `{"question":"`+cid+`"}`)
		}()
	}
	wg.Wait()
	ev := e.a.events
	if len(ev) != 4 || !strings.HasPrefix(ev[1], "enter:") {
		t.Errorf("events %v: the second did not start while the first ran", ev)
	}
}
