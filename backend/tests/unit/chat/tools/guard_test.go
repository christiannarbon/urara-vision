// Ported from chat/tests/unit/test_langchain_tools.py.
package tools_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/tools"
)

// probe is a real tool name, so validation applies, with a Run a test controls.
func probe(name string, run func(ctx context.Context, args json.RawMessage) (any, error)) tools.Spec {
	return tools.Spec{Name: name, Description: "a probe", Run: run}
}

func failing(err error) tools.Spec {
	return probe("get_neighbourhood", func(context.Context, json.RawMessage) (any, error) { return nil, err })
}

type logLines struct{ bytes.Buffer }

func (l *logLines) toolCall(t *testing.T) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(l.String()), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil && m["msg"] == "tool call" {
			return m
		}
	}
	t.Fatalf("no tool call log in %q", l.String())
	return nil
}

func guarded(spec tools.Spec) (func(context.Context, json.RawMessage) (any, error), *logLines) {
	l := &logLines{}
	log := slog.New(slog.NewJSONHandler(l, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return tools.Guarded(spec, log), l
}

func advice(t *testing.T, spec tools.Spec, args string) string {
	t.Helper()
	run, _ := guarded(spec)
	out, err := run(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("err = %v, want advice", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("result = %v, want advice text", out)
	}
	return s
}

func TestGuardedASuccessfulCallReturnsTheRealResult(t *testing.T) {
	want := map[string]any{"items": []any{1}, "truncated": false, "total": 1}
	run, _ := guarded(probe("get_neighbourhood", func(context.Context, json.RawMessage) (any, error) { return want, nil }))
	out, err := run(context.Background(), json.RawMessage(`{"table_id":"d/t"}`))
	if err != nil || !reflect.DeepEqual(out, want) {
		t.Errorf("%v, %v", out, err)
	}
}

func TestGuardedInvalidArgumentsBecomeAdvice(t *testing.T) {
	called := false
	spec := probe("search_model", func(context.Context, json.RawMessage) (any, error) { called = true; return nil, nil })
	got := advice(t, spec, `{"query":"x","limit":500}`)
	want := "Invalid arguments for search_model: limit: must be between 1 and 50, got 500. Fix them and call the tool again."
	if got != want || called {
		t.Errorf("advice = %q, tool called %v", got, called)
	}
}

func TestGuardedNotFoundNamesTheIDsAndTheToolThatFixesIt(t *testing.T) {
	notFound := &apiclient.Error{Status: 404, Message: "not found"}
	got := advice(t, failing(notFound), `{"table_id":"ordering/nope"}`)
	if got != "No table with id 'ordering/nope' in this model. Call search_model to find the right ID." {
		t.Errorf("advice = %q", got)
	}

	batch := probe("get_tables", func(context.Context, json.RawMessage) (any, error) { return nil, notFound })
	if got := advice(t, batch, `{"ids":["a/one","b/two"]}`); !strings.Contains(got, "'a/one', 'b/two'") {
		t.Errorf("batch advice = %q", got)
	}

	paths := probe("find_join_paths", func(context.Context, json.RawMessage) (any, error) { return nil, notFound })
	if got := advice(t, paths, `{"from_table":"a/x","to_table":"b/y"}`); !strings.Contains(got, "'a/x', 'b/y'") {
		t.Errorf("paths advice = %q", got)
	}

	none := probe("list_domains", func(context.Context, json.RawMessage) (any, error) { return nil, notFound })
	if got := advice(t, none, `{}`); !strings.HasPrefix(got, "That was not found in this model.") {
		t.Errorf("no-ID advice = %q", got)
	}
}

func TestGuardedABackendErrorCarriesTheReason(t *testing.T) {
	for _, err := range []error{
		&apiclient.Error{Status: 500, Message: "the index is rebuilding"},
		&apiclient.Error{Status: 400, Message: "bad query"},
	} {
		want := "The model store could not answer that: " + err.(*apiclient.Error).Message + "."
		if got := advice(t, failing(err), `{"table_id":"d/t"}`); got != want {
			t.Errorf("advice = %q", got)
		}
	}
}

// An unreachable backend is not fixed by ending the turn either.
func TestGuardedAnUnreachableBackendIsRecoverable(t *testing.T) {
	c := apiclient.New("http://127.0.0.1:1", "", 0)
	spec := probe("list_domains", func(ctx context.Context, _ json.RawMessage) (any, error) { return c.Domains(ctx, "s") })
	if got := advice(t, spec, `{}`); !strings.Contains(got, "could not answer") || !strings.Contains(got, "unreachable") {
		t.Errorf("advice = %q", got)
	}
}

// The backend client's own timeout, with the turn still live.
func TestGuardedATimeoutSuggestsNarrowing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { time.Sleep(200 * time.Millisecond) }))
	defer srv.Close()
	c := apiclient.New(srv.URL, "", 20*time.Millisecond)
	spec := probe("list_domains", func(ctx context.Context, _ json.RawMessage) (any, error) { return c.Domains(ctx, "s") })
	if got := advice(t, spec, `{}`); got != "That lookup timed out. Try a narrower query." {
		t.Errorf("advice = %q", got)
	}
}

func TestGuardedForbiddenEndsTheTurn(t *testing.T) {
	forbidden := &apiclient.Error{Status: 403, Message: "forbidden"}
	run, _ := guarded(failing(forbidden))
	out, err := run(context.Background(), json.RawMessage(`{"table_id":"d/t"}`))
	if !errors.Is(err, apiclient.ErrForbidden) || out != nil {
		t.Errorf("%v, %v", out, err)
	}
}

// A finished turn is not the model's to recover from, even when the call timed out.
func TestGuardedADoneParentEndsTheTurn(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(cause)
		run, _ := guarded(failing(cause))
		if out, err := run(ctx, json.RawMessage(`{"table_id":"d/t"}`)); err == nil || out != nil {
			t.Errorf("%v: %v, %v", cause, out, err)
		}
	}
}

// An unexpected error is a bug, not advice.
func TestGuardedUnexpectedErrorsPropagate(t *testing.T) {
	bug := errors.New("a real bug")
	run, _ := guarded(failing(bug))
	if _, err := run(context.Background(), json.RawMessage(`{"table_id":"d/t"}`)); !errors.Is(err, bug) {
		t.Errorf("err = %v", err)
	}
}

func TestGuardedCallsAreLoggedAtDebug(t *testing.T) {
	run, l := guarded(probe("get_neighbourhood", func(context.Context, json.RawMessage) (any, error) { return "ok", nil }))
	_, _ = run(context.Background(), json.RawMessage(`{"table_id":"d/t"}`))
	line := l.toolCall(t)
	if line["level"] != "DEBUG" || line["tool"] != "get_neighbourhood" || line["returned_error"] != false || line["error"] != nil ||
		!reflect.DeepEqual(line["tool_args"], map[string]any{"table_id": "d/t"}) {
		t.Errorf("log = %v", line)
	}
	if d, ok := line["duration_ms"].(float64); !ok || d < 0 {
		t.Errorf("duration_ms = %v", line["duration_ms"])
	}
}

// Advice looks like a success from outside, so the log marks it.
func TestGuardedAReturnedErrorIsMarkedInTheLog(t *testing.T) {
	run, l := guarded(failing(&apiclient.Error{Status: 404, Message: "nope"}))
	_, _ = run(context.Background(), json.RawMessage(`{"table_id":"d/t"}`))
	line := l.toolCall(t)
	if e, _ := line["error"].(string); line["returned_error"] != true || !strings.Contains(e, "search_model") {
		t.Errorf("log = %v", line)
	}
}

// Every real tool, guarded, over an empty backend.
func TestGuardedEveryRealToolRuns(t *testing.T) {
	args := map[string]string{
		"get_tables": `{"ids":["d/t"]}`, "search_model": `{"query":"x"}`, "get_neighbourhood": `{"table_id":"d/t"}`,
		"find_join_paths": `{"from_table":"d/t","to_table":"d/u"}`, "get_lineage": `{"table_id":"d/t"}`,
	}
	for _, s := range tools.Build(&fakeBackend{}, "snap-1") {
		run, _ := guarded(s)
		out, err := run(context.Background(), json.RawMessage(args[s.Name]))
		if _, isMap := out.(map[string]any); err != nil || !isMap {
			t.Errorf("%s: %v, %v", s.Name, out, err)
		}
	}
}
