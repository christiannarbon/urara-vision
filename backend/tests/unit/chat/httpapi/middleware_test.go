// Ported from chat/tests/unit/test_middleware.py and the body-limit case in test_routes.py.
package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/chat/reqctx"
)

// logs collects JSON log lines at debug and above.
type logs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logs) lines(t *testing.T) []map[string]any {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(l.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

// requestLines are the middleware's "request" lines.
func (l *logs) requestLines(t *testing.T) []map[string]any {
	var out []map[string]any
	for _, m := range l.lines(t) {
		if m["msg"] == "request" {
			out = append(out, m)
		}
	}
	return out
}

func newServerWith(t *testing.T, maxBytes int64) (*httpapi.Server, *logs) {
	t.Helper()
	l := &logs{}
	log := slog.New(slog.NewJSONHandler(l, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return httpapi.New(httpapi.Deps{Settings: &config.Settings{MaxRequestBytes: maxBytes}, Log: log}), l
}

func newServer(t *testing.T, maxBytes int64) (http.Handler, *logs) {
	s, l := newServerWith(t, maxBytes)
	return s.Handler(), l
}

// wrapped runs h under the service's middleware.
func wrapped(t *testing.T, h http.HandlerFunc) (http.Handler, *logs) {
	s, l := newServerWith(t, 1<<20)
	return s.Wrap(h), l
}

func bodyOf(n int) io.Reader {
	return strings.NewReader(`{"q":"` + strings.Repeat("x", n-8) + `"}`)
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func get(h http.Handler, path, requestID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if requestID != "" {
		req.Header.Set("X-Request-Id", requestID)
	}
	return serve(h, req)
}

// echoID answers with the request ID the handler saw.
func echoID(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{"requestId": reqctx.RequestID(r.Context())})
}

func seen(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return body["requestId"]
}

func TestAGeneratedIDComesBackAndMatchesWhatTheHandlerSaw(t *testing.T) {
	h, _ := wrapped(t, echoID)
	rec := get(h, "/ok", "")
	id := rec.Header().Get("X-Request-Id")
	if len(id) != 32 || seen(t, rec) != id {
		t.Errorf("header %q, handler saw %q", id, seen(t, rec))
	}
}

func TestAnInboundIDIsEchoedUnchanged(t *testing.T) {
	h, _ := wrapped(t, echoID)
	rec := get(h, "/ok", "my-trace-1")
	if rec.Header().Get("X-Request-Id") != "my-trace-1" || seen(t, rec) != "my-trace-1" {
		t.Errorf("header %q, handler saw %q", rec.Header().Get("X-Request-Id"), seen(t, rec))
	}
}

func TestALongInboundIDIsTruncated(t *testing.T) {
	h, _ := wrapped(t, echoID)
	rec := get(h, "/ok", strings.Repeat("a", 200))
	if got := rec.Header().Get("X-Request-Id"); got != strings.Repeat("a", 64) {
		t.Errorf("header = %q (%d)", got, len(got))
	}
}

func TestANewlineCannotForgeALogLine(t *testing.T) {
	h, l := wrapped(t, echoID)
	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header["X-Request-Id"] = []string{"abc\n{\"level\":\"error\",\"msg\":\"forged\"}"}
	rec := serve(h, req)

	id := rec.Header().Get("X-Request-Id")
	if !strings.HasPrefix(id, "abc") || strings.ContainsAny(id, "\n{\"") {
		t.Errorf("header = %q", id)
	}
	for _, m := range l.lines(t) {
		if m["msg"] == "forged" {
			t.Error("a forged line reached the log")
		}
	}
}

func TestSanitiseRequestID(t *testing.T) {
	if got := httpapi.SanitiseRequestID("a\r\n\tb c\x00d"); got != "abcd" {
		t.Errorf("control characters and spaces kept: %q", got)
	}
	if got := httpapi.SanitiseRequestID("chi-host/abc-000001:x_y.z"); got != "chi-host/abc-000001:x_y.z" {
		t.Errorf("allowed characters dropped: %q", got)
	}
	for _, raw := range []string{"", "\n\n\n", "{}\"'"} {
		if got := httpapi.SanitiseRequestID(raw); len(got) != 32 {
			t.Errorf("SanitiseRequestID(%q) = %q, want a fresh ID", raw, got)
		}
	}
}

func TestAnAbsentHeaderGetsAFreshIDPerRequest(t *testing.T) {
	h, _ := wrapped(t, echoID)
	first := get(h, "/ok", "").Header().Get("X-Request-Id")
	second := get(h, "/ok", "").Header().Get("X-Request-Id")
	if first == "" || first == second {
		t.Errorf("ids %q and %q", first, second)
	}
}

func TestTheIDIsSentOnce(t *testing.T) {
	h, _ := wrapped(t, func(w http.ResponseWriter, r *http.Request) {
		httpapi.WriteError(w, r, http.StatusNotFound, map[string]any{"error": httpapi.MsgNotFound})
	})
	rec := get(h, "/boom", "trace-dup")
	if got := rec.Header().Values("X-Request-Id"); len(got) != 1 || got[0] != "trace-dup" {
		t.Errorf("X-Request-Id = %v", got)
	}
}

func TestTheIDReachesTheBackend(t *testing.T) {
	var forwarded string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = r.Header.Get("X-Request-Id")
		_, _ = io.WriteString(w, `{"domains":[]}`)
	}))
	t.Cleanup(backend.Close)
	client := apiclient.New(backend.URL, "", time.Second)

	h, _ := wrapped(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := client.Domains(r.Context(), "snap-1"); err != nil {
			t.Error(err)
		}
	})
	get(h, "/backend", "trace-out")
	if forwarded != "trace-out" {
		t.Errorf("backend saw X-Request-Id %q", forwarded)
	}
	if _, err := client.Domains(context.Background(), "snap-1"); err != nil || forwarded != "" {
		t.Errorf("outside a request the header must be absent, got %q (%v)", forwarded, err)
	}
}

func TestADeclaredOversizedBodyIs413WithTheID(t *testing.T) {
	h, _ := newServer(t, 200)
	for _, id := range []string{"", "trace-413"} {
		req := httptest.NewRequest("POST", "/debug/answer", bodyOf(500))
		if id != "" {
			req.Header.Set("X-Request-Id", id)
		}
		rec := serve(h, req)

		var body map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != 413 || !strings.Contains(body["detail"], "200") {
			t.Fatalf("status %d body %v", rec.Code, body)
		}
		if header := rec.Header().Get("X-Request-Id"); header == "" || body["requestId"] != header || (id != "" && header != id) {
			t.Errorf("header %q body requestId %q", header, body["requestId"])
		}
	}
}

// Without a Content-Length the read itself is bounded.
func TestAnUndeclaredOversizedBodyIsStill413(t *testing.T) {
	s, _ := newServerWith(t, 200)
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if s.DecodeJSON(w, r, &v) {
			t.Error("an oversized body decoded")
		}
	}))
	req := httptest.NewRequest("POST", "/x", bodyOf(500))
	req.ContentLength = -1
	rec := serve(h, req)
	if rec.Code != 413 || rec.Header().Get("X-Request-Id") == "" {
		t.Errorf("status %d, body %s", rec.Code, rec.Body)
	}
}

func TestABodyUnderTheLimitReachesTheHandler(t *testing.T) {
	s, _ := newServerWith(t, 200)
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if s.DecodeJSON(w, r, &v) {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	if rec := serve(h, httptest.NewRequest("POST", "/x", bodyOf(100))); rec.Code != http.StatusNoContent {
		t.Errorf("status %d, body %s", rec.Code, rec.Body)
	}
}

func TestOneLogLinePerRequestCarryingTheID(t *testing.T) {
	h, l := wrapped(t, echoID)
	get(h, "/ok", "trace-log")

	lines := l.requestLines(t)
	if len(lines) != 1 {
		t.Fatalf("%d request lines", len(lines))
	}
	m := lines[0]
	if m["request_id"] != "trace-log" || m["method"] != "GET" || m["path"] != "/ok" ||
		m["status"] != float64(200) || m["level"] != "INFO" {
		t.Errorf("line = %v", m)
	}
	if d, ok := m["duration_ms"].(float64); !ok || d < 0 {
		t.Errorf("duration_ms = %v", m["duration_ms"])
	}
}

func TestProbesAreQuietUnlessTheyFail(t *testing.T) {
	status := http.StatusOK
	h, l := wrapped(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })

	get(h, "/healthz", "")
	status = http.StatusServiceUnavailable
	get(h, "/readyz", "")

	lines := l.requestLines(t)
	if len(lines) != 2 || lines[0]["level"] != "DEBUG" || lines[1]["level"] != "INFO" {
		t.Errorf("lines = %v", lines)
	}
	if rec := get(h, "/healthz", ""); rec.Header().Get("X-Request-Id") == "" {
		t.Error("a probe still carries the request ID")
	}
}

func TestTheLineIsWrittenWhenTheHandlerPanics(t *testing.T) {
	h, l := wrapped(t, func(http.ResponseWriter, *http.Request) { panic("boom") })
	get(h, "/boom", "")
	lines := l.requestLines(t)
	if len(lines) != 1 || lines[0]["status"] != float64(500) {
		t.Errorf("lines = %v", lines)
	}
}

func TestTrailingDataPastTheLimitIs413(t *testing.T) {
	s, _ := newServerWith(t, 200)
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		s.DecodeJSON(w, r, &v)
	}))
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"q":"x"}`+strings.Repeat(" ", 300)+"x"))
	req.ContentLength = -1
	if rec := serve(h, req); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status %d, body %s", rec.Code, rec.Body)
	}
}

func TestAHandlerThatWritesNothingStillCarriesTheID(t *testing.T) {
	h, _ := wrapped(t, func(http.ResponseWriter, *http.Request) {})
	rec := get(h, "/quiet", "")
	if rec.Code != http.StatusOK || rec.Header().Get("X-Request-Id") == "" {
		t.Errorf("status %d, X-Request-Id %q", rec.Code, rec.Header().Get("X-Request-Id"))
	}
}
