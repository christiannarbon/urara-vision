package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/httpapi"
)

// Never shown to a caller.
const secret = "prompt fragment and AIza-shaped-credential"

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return m
}

func unreachableError(t *testing.T) error {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	_, err := apiclient.New(srv.URL, "", time.Second).Domains(t.Context(), "s")
	return err
}

func TestBackendErrorsMapToStatuses(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		msg    string
	}{
		{"not found", &apiclient.Error{Status: 404, Message: secret}, 404, httpapi.MsgNotFound},
		{"forbidden", &apiclient.Error{Status: 403, Message: secret}, 403, httpapi.MsgNotAllowed},
		{"rejected", &apiclient.Error{Status: 400, Message: secret}, 500, httpapi.MsgInternal},
		{"server error", &apiclient.Error{Status: 500, Message: secret}, 502, httpapi.MsgBackendUnavailable},
		{"unreachable", unreachableError(t), 502, httpapi.MsgBackendUnavailable},
		{"undecodable response", errors.New(secret), 500, httpapi.MsgInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, l := newServerWith(t, 1<<20)
			h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				s.RenderBackendError(w, r, tc.err)
			}))
			rec := get(h, "/x", "trace-err")

			want := map[string]any{"error": tc.msg, "requestId": "trace-err"}
			if rec.Code != tc.status || !reflect.DeepEqual(decode(t, rec), want) {
				t.Errorf("%d %s, want %d %v", rec.Code, rec.Body, tc.status, want)
			}
			if strings.Contains(rec.Body.String(), secret) {
				t.Error("the backend's message reached the caller")
			}
			if tc.status >= 500 && !strings.Contains(l.buf.String(), "trace-err") {
				t.Error("a 5xx must be logged with the request ID")
			}
		})
	}
}

func TestAPanicIs500WithTheRequestID(t *testing.T) {
	h, l := wrapped(t, func(http.ResponseWriter, *http.Request) { panic(secret) })
	rec := get(h, "/boom", "trace-err")

	want := map[string]any{"error": "internal error", "requestId": "trace-err"}
	if rec.Code != 500 || !reflect.DeepEqual(decode(t, rec), want) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("X-Request-Id") != "trace-err" {
		t.Errorf("X-Request-Id = %q", rec.Header().Get("X-Request-Id"))
	}
	var logged bool
	for _, m := range l.lines(t) {
		if m["msg"] == "unhandled exception" && strings.Contains(m["stack"].(string), "goroutine") {
			logged = true
		}
	}
	if !logged {
		t.Error("the panic was not logged with its stack")
	}
}

func decodeInto(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	s, _ := newServerWith(t, 1<<20)
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var dst struct {
			Question string `json:"question"`
			Limit    int    `json:"limit"`
		}
		if s.DecodeJSON(w, r, &dst) {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	return serve(h, httptest.NewRequest("POST", "/echo", strings.NewReader(body)))
}

func wantFields(t *testing.T, rec *httptest.ResponseRecorder, field string) {
	t.Helper()
	body := decode(t, rec)
	fields, _ := body["fields"].([]any)
	if rec.Code != 400 || body["error"] != "the request is invalid" || body["requestId"] == "" || len(fields) != 1 {
		t.Fatalf("%d %v", rec.Code, body)
	}
	f := fields[0].(map[string]any)
	if f["field"] != field || f["location"] != "body" || f["reason"] == "" {
		t.Errorf("field = %v, want %q in body", f, field)
	}
}

func TestDecodeJSON(t *testing.T) {
	if rec := decodeInto(t, `{"question":"hi"}`); rec.Code != http.StatusNoContent {
		t.Errorf("a valid body: %d %s", rec.Code, rec.Body)
	}
	wantFields(t, decodeInto(t, `{"quesiton":"typo"}`), "quesiton")
	wantFields(t, decodeInto(t, `{"question":"hi","limit":"x"}`), "limit")
	wantFields(t, decodeInto(t, ``), "body")
	wantFields(t, decodeInto(t, `{not json`), "body")
}

func TestFieldErrorMatchesTheGoldenShape(t *testing.T) {
	h, _ := wrapped(t, func(w http.ResponseWriter, r *http.Request) {
		httpapi.FieldError(w, r, httpapi.Required("snapshotId"), httpapi.Required("question"))
	})
	assertGolden(t, "errors/validation-missing.json", get(h, "/x", ""))
}

func TestUnknownFieldMatchesTheGolden(t *testing.T) {
	s, _ := newServerWith(t, 1<<20)
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var dst struct {
			SnapshotID string `json:"snapshotId"`
			Question   string `json:"question"`
		}
		s.DecodeJSON(w, r, &dst)
	}))
	rec := serve(h, httptest.NewRequest("POST", "/x", strings.NewReader(`{"snapshotId":"s","question":"hi","bogus":1}`)))
	assertGolden(t, "errors/validation-extra.json", rec)
}

func TestAnUnknownRouteIs404WithTheID(t *testing.T) {
	h, _ := newServer(t, 1<<20)
	rec := get(h, "/nope", "trace-404")
	want := map[string]any{"detail": "Not Found", "requestId": "trace-404"}
	if rec.Code != 404 || !reflect.DeepEqual(decode(t, rec), want) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestDecodeJSONRefusesTrailingDataAndHidesGoTypes(t *testing.T) {
	wantFields(t, decodeInto(t, `{"question":"hi"} x`), "body")
	wantFields(t, decodeInto(t, `{"question":"hi"}{"question":"b"}`), "body")

	rec := decodeInto(t, `[]`)
	wantFields(t, rec, "body")
	if strings.Contains(rec.Body.String(), "struct") {
		t.Errorf("a Go type reached the caller: %s", rec.Body)
	}

	rec = decodeInto(t, `{"question":"hi","limit":"x"}`)
	wantFields(t, rec, "limit")
	if reason := decode(t, rec)["fields"].([]any)[0].(map[string]any)["reason"]; reason != "Input should be a valid number" {
		t.Errorf("reason = %q", reason)
	}

	if rec := decodeInto(t, "{\"question\":\"hi\"}\n"); rec.Code != http.StatusNoContent {
		t.Errorf("trailing whitespace: %d %s", rec.Code, rec.Body)
	}
}

// Pydantic matches keys exactly; encoding/json alone would fold case.
func TestAKeyInTheWrongCaseIsAnExtraField(t *testing.T) {
	rec := decodeInto(t, `{"Question":"hi"}`)
	wantFields(t, rec, "Question")
	if reason := decode(t, rec)["fields"].([]any)[0].(map[string]any)["reason"]; reason != "Extra inputs are not permitted" {
		t.Errorf("reason = %q", reason)
	}
}

// Once headers are out, a panic can only be logged.
func TestAPanicAfterAPartialWriteDoesNotWriteAgain(t *testing.T) {
	h, l := wrapped(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"partial":`))
		panic("late")
	})
	rec := get(h, "/late", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "internal error") {
		t.Errorf("status %d, body %q", rec.Code, rec.Body)
	}
	if !strings.Contains(l.buf.String(), "unhandled exception") {
		t.Error("the panic was not logged")
	}
}

func TestACallerHangingUpIsNotABackendFault(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := apiclient.New(srv.URL, "", time.Second).Domains(ctx, "s")

	s, l := newServerWith(t, 1<<20)
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.RenderBackendError(w, r, err)
	}))
	get(h, "/x", "trace-gone")
	for _, m := range l.lines(t) {
		if m["request_id"] == "trace-gone" && m["msg"] != "request" && m["level"] != "INFO" {
			t.Errorf("logged %v at %v, want INFO", m["msg"], m["level"])
		}
	}
	if !strings.Contains(l.buf.String(), "request cancelled by the caller") {
		t.Errorf("log = %s", l.buf.String())
	}
}
