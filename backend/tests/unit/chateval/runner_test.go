package chateval_test

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"urara-vision/backend/internal/chateval"
)

// fakeBackend ingests any set as snapshot "sid-<name>" and records DELETEs.
type fakeBackend struct {
	mu      sync.Mutex
	deleted []string
	// conflict answers ingest with 409 for these sets.
	conflict map[string]bool
	listed   []map[string]string
	version  string   // in the 409 body; "1" when empty
	paths    []string // escaped paths of GETs under /api/v1/projects/
}

func (b *fakeBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "POST" && r.URL.Path == "/api/v1/ingest":
		var body struct{ Name string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if b.conflict[body.Name] {
			w.WriteHeader(http.StatusConflict)
			version := b.version
			if version == "" {
				version = "1"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"project": body.Name, "version": version})
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"snapshot": map[string]string{"id": "sid-" + body.Name}})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v1/projects/"):
		b.mu.Lock()
		b.paths = append(b.paths, r.URL.EscapedPath())
		b.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "existing-" + strings.Split(r.URL.Path, "/")[4]})
	case r.Method == "GET" && r.URL.Path == "/api/v1/snapshots":
		_ = json.NewEncoder(w).Encode(map[string]any{"snapshots": b.listed})
	case r.Method == "DELETE":
		b.mu.Lock()
		b.deleted = append(b.deleted, strings.TrimPrefix(r.URL.Path, "/api/v1/snapshots/"))
		b.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func setDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		_ = os.MkdirAll(filepath.Join(dir, n), 0o755)
		_ = os.WriteFile(filepath.Join(dir, n, "a.md"), []byte("# a"), 0o600)
	}
	return dir
}

func ingester(t *testing.T, b *fakeBackend, names ...string) *chateval.Ingester {
	t.Helper()
	return ingesterTo(t, b, nil, names...)
}

func ingesterTo(t *testing.T, b *fakeBackend, out io.Writer, names ...string) *chateval.Ingester {
	t.Helper()
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)
	in := &chateval.Ingester{BackendURL: srv.URL, Label: "eval-test", ActingUser: "admin", Dirs: []string{setDir(t, names...)}, Client: srv.Client(), Out: out}
	for _, n := range names {
		if _, err := in.Ingest(context.Background(), n); err != nil {
			t.Fatal(err)
		}
	}
	return in
}

// chatServer serves /readyz and hands /api/chat/answer to answer.
func chatServer(t *testing.T, answer http.HandlerFunc) *chateval.Chat {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" {
			return
		}
		answer(w, r)
	}))
	t.Cleanup(srv.Close)
	return &chateval.Chat{URL: srv.URL, UserID: "admin", Client: srv.Client()}
}

func ok(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{"text": "answer", "citations": []string{}, "toolCalls": []any{}, "model": "m", "latencyMs": 5})
}

func questions(n int) []chateval.Question {
	qs := make([]chateval.Question, n)
	for i := range qs {
		qs[i] = chateval.Question{ID: string(rune('a' + i)), Set: "s", Category: "lookup"}
	}
	return qs
}

func run(t *testing.T, ctx context.Context, chat *chateval.Chat, qs []chateval.Question, concurrency int) ([]chateval.Result, error) {
	t.Helper()
	return chateval.RunAll(ctx, chateval.RunOptions{
		Chat: chat, Questions: qs, SnapshotIDs: map[string]string{"s": "sid-s"}, Repeat: 1, Concurrency: concurrency,
	})
}

func TestA429IsRetriedAfterRetryAfter(t *testing.T) {
	var calls atomic.Int32
	chat := chatServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		ok(w)
	})
	results, err := run(t, context.Background(), chat, questions(1), 1)
	if err != nil || len(results) != 1 || results[0].Error != "" || results[0].Scores == nil || calls.Load() != 3 {
		t.Errorf("results %+v, err %v, calls %d", results, err, calls.Load())
	}
}

func TestSix429sAreRecordedAndTheRunContinues(t *testing.T) {
	var calls atomic.Int32
	chat := chatServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"busy"}`))
	})
	results, err := run(t, context.Background(), chat, questions(2), 1)
	if err != nil || len(results) != 2 || calls.Load() != 12 {
		t.Fatalf("results %d, err %v, calls %d", len(results), err, calls.Load())
	}
	for _, r := range results {
		if !strings.HasPrefix(r.Error, `429 {"error":"busy"}`) || r.Scores != nil {
			t.Errorf("%s: error %q", r.ID, r.Error)
		}
	}
	if s := chateval.Summarise(results); len(s.Errors) != 2 {
		t.Errorf("summary errors %v", s.Errors)
	}
}

func TestAnInterruptedRunStillCleansUp(t *testing.T) {
	b := &fakeBackend{}
	in := ingester(t, b, "s", "t")
	ctx, cancel := context.WithCancel(context.Background())
	chat := chatServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Read the body first: the server only notices a hang-up after it.
		_, _ = io.Copy(io.Discard, r.Body)
		cancel()
		<-r.Context().Done()
	})
	if _, err := run(t, ctx, chat, questions(3), 2); err == nil {
		t.Fatal("no error after cancel")
	}
	cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if failed := in.Cleanup(cleanup); len(failed) != 0 {
		t.Errorf("failed %v", failed)
	}
	slices.Sort(b.deleted)
	if want := []string{"sid-s", "sid-t"}; !slices.Equal(b.deleted, want) {
		t.Errorf("deleted %v, want %v", b.deleted, want)
	}
}

func TestAReusedVersionIsNotDeleted(t *testing.T) {
	b := &fakeBackend{conflict: map[string]bool{"old": true}}
	var out strings.Builder
	in := ingesterTo(t, b, &out, "old", "new")
	if in.IDs["old"] != "existing-old" {
		t.Errorf("ids %v", in.IDs)
	}
	if got := out.String(); !strings.Contains(got, "old 1 already ingested") || strings.Contains(got, "new") {
		t.Errorf("output %q", got)
	}
	_ = in.Cleanup(context.Background())
	if !slices.Equal(b.deleted, []string{"sid-new"}) {
		t.Errorf("deleted %v", b.deleted)
	}
}

func TestAReusedVersionPathIsEscaped(t *testing.T) {
	b := &fakeBackend{conflict: map[string]bool{"old": true}, version: "1.0/rc"}
	ingester(t, b, "old")
	if len(b.paths) != 1 || !strings.HasSuffix(b.paths[0], "/versions/1.0%2Frc") {
		t.Errorf("paths %v", b.paths)
	}
}

func TestCleanupSweepsTheRunLabel(t *testing.T) {
	b := &fakeBackend{listed: []map[string]string{
		{"id": "stray", "sourceLabel": "eval-test"},
		{"id": "other", "sourceLabel": "someone-else"},
	}}
	in := ingester(t, b, "s")
	if failed := in.Cleanup(context.Background()); len(failed) != 0 {
		t.Errorf("failed %v", failed)
	}
	slices.Sort(b.deleted)
	if want := []string{"sid-s", "stray"}; !slices.Equal(b.deleted, want) {
		t.Errorf("deleted %v, want %v", b.deleted, want)
	}
}

func TestConcurrencyIsCapped(t *testing.T) {
	var inFlight, peak atomic.Int32
	chat := chatServer(t, func(w http.ResponseWriter, _ *http.Request) {
		n := inFlight.Add(1)
		// Wait for a partner, so running one at a time cannot pass.
		for deadline := time.Now().Add(2 * time.Second); inFlight.Load() < 2 && time.Now().Before(deadline); {
			time.Sleep(time.Millisecond)
		}
		n = inFlight.Load()
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		ok(w)
	})
	results, err := run(t, context.Background(), chat, questions(6), 2)
	if err != nil || len(results) != 6 || peak.Load() != 2 {
		t.Errorf("results %d, err %v, peak %d", len(results), err, peak.Load())
	}
	for i, r := range results {
		if r.ID != string(rune('a'+i)) {
			t.Errorf("result %d is %s: not in question order", i, r.ID)
		}
	}
}

func TestAWrongModelStopsTheRun(t *testing.T) {
	chat := chatServer(t, func(w http.ResponseWriter, _ *http.Request) { ok(w) })
	chat.ExpectModel = "other"
	if _, err := run(t, context.Background(), chat, questions(2), 1); err == nil || !strings.Contains(err.Error(), `answered with "m"`) {
		t.Errorf("err = %v", err)
	}
}

// Keys run_eval.py wrote; a local Python results file is checked too when present.
var pythonKeys = map[string][]string{
	"":        {"startedAt", "wallSeconds", "args", "thresholds", "summary", "results"},
	"args":    {"set", "category", "id", "model", "repeat", "concurrency", "out", "chat_url", "backend_url"},
	"summary": {"categories", "overall", "refusal_accuracy", "violations", "errors", "per_question", "tokens_in", "tokens_out", "mean_wall_ms"},
	"result":  {"id", "set", "category", "language", "question", "run", "answer", "citations", "tool_calls", "model", "usage", "latency_ms", "wall_ms", "error", "scores"},
	"scores":  {"recall", "precision", "tools", "substr", "violations", "refusal_ok"},
}

func keysOf(t *testing.T, raw []byte) map[string][]string {
	t.Helper()
	var f map[string]any
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	sorted := func(m any) []string { return slices.Sorted(maps.Keys(m.(map[string]any))) }
	results := f["results"].([]any)
	var scored map[string]any
	for _, r := range results {
		if s, ok := r.(map[string]any)["scores"].(map[string]any); ok {
			scored = s
			break
		}
	}
	return map[string][]string{
		"": sorted(f), "args": sorted(f["args"]), "summary": sorted(f["summary"]),
		"result": sorted(results[0]), "scores": sorted(scored),
	}
}

func TestResultsFileHasPythonsKeys(t *testing.T) {
	q := chateval.Question{ID: "q", Category: "lookup", ExpectCitations: []string{"a/b"}}
	s := chateval.Score(q, chateval.AnswerResponse{Citations: []string{"a/b"}})
	results := []chateval.Result{{ID: "q", Category: "lookup", Run: 1, Scores: &s}}
	started := time.Date(2026, 10, 5, 1, 2, 3, 0, time.UTC)
	path := chateval.ResultsFile(t.TempDir(), started)
	if filepath.Base(path) != "20261005T010203Z.json" {
		t.Errorf("file name %s", filepath.Base(path))
	}
	err := chateval.WriteResults(path, chateval.Summarise(results), results, chateval.Meta{StartedAt: started, Wall: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	got := keysOf(t, raw)

	want := map[string][]string{}
	for k, v := range pythonKeys {
		want[k] = slices.Sorted(slices.Values(v))
	}
	if pyFiles, _ := filepath.Glob(filepath.Join(evalDir, "results", "*.json")); len(pyFiles) > 0 {
		for _, f := range pyFiles {
			if raw, err := os.ReadFile(f); err == nil && strings.Contains(string(raw), `"refusal_ok"`) {
				if py := keysOf(t, raw); !maps.EqualFunc(py, want, slices.Equal) {
					t.Errorf("%s keys %v differ from the expected list %v", f, py, want)
				}
				break
			}
		}
	}
	if !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("keys %v, want %v", got, want)
	}
}
