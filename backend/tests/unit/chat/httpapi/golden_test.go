package httpapi_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
)

// The same placeholders capture.sh writes.
var (
	uuidPattern    = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	timePattern    = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}`)
	latencyPattern = regexp.MustCompile(`(?i)latency|duration`)
)

func normalise(v any) any {
	switch v := v.(type) {
	case string:
		s := uuidPattern.ReplaceAllString(v, "<uuid>")
		if timePattern.MatchString(s) {
			return "<time>"
		}
		return s
	case []any:
		for i := range v {
			v[i] = normalise(v[i])
		}
		return v
	case map[string]any:
		for k, x := range v {
			switch _, isNumber := x.(float64); {
			case k == "requestId":
				v[k] = "<request-id>"
			case isNumber && latencyPattern.MatchString(k):
				v[k] = "<number>"
			default:
				v[k] = normalise(x)
			}
		}
		// Validation reasons are pydantic's wording; field and location must match.
		if fields, ok := v["fields"].([]any); ok {
			for _, f := range fields {
				if m, ok := f.(map[string]any); ok {
					delete(m, "reason")
				}
			}
		}
		return v
	}
	return v
}

type golden struct {
	Status  int             `json:"status"`
	Headers map[string]bool `json:"headers"`
	Body    any             `json:"body"`
}

// assertGolden compares a response with testdata/golden/<name>, as decoded
// JSON after capture.sh's normalisation.
func assertGolden(t *testing.T, name string, rec *httptest.ResponseRecorder) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	var want golden
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	want.Body = normalise(want.Body)

	got := golden{
		Status: rec.Code,
		Headers: map[string]bool{
			"Retry-After":  rec.Header().Get("Retry-After") != "",
			"X-Request-Id": rec.Header().Get("X-Request-Id") != "",
		},
	}
	if rec.Body.Len() > 0 && json.Unmarshal(rec.Body.Bytes(), &got.Body) != nil {
		got.Body = nil
	}
	got.Body = normalise(got.Body)

	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("%s does not match\n got: %s\nwant: %s", name, gotJSON, wantJSON)
	}
}

func TestGoldenTooLarge(t *testing.T) {
	h, _ := newServer(t, 1048576)
	req := httptest.NewRequest("POST", "/api/chat/answer", bodyOf(1048577))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assertGolden(t, "errors/too-large.json", rec)
}
