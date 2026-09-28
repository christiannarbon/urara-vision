// Python and Go chat services compared request by request (20.1).
package parity_test

import (
	"reflect"
	"regexp"
	"testing"
)

// capture.sh's placeholders (16.1), plus validation reason text dropped.
var (
	uuidPattern    = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	timePattern    = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}`)
	latencyPattern = regexp.MustCompile(`(?i)latency|duration`)
)

// normalise rewrites v in place and returns it.
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

func TestNormalise(t *testing.T) {
	cases := []struct {
		name    string
		in, out any
	}{
		{"uuid", "0b6f3c1e-5a2d-4c8e-9f10-2b3c4d5e6f70", "<uuid>"},
		{"uuid inside a string", "snapshot 0b6f3c1e-5a2d-4c8e-9f10-2b3c4d5e6f70 not found", "snapshot <uuid> not found"},
		{"uppercase is not a uuid", "0B6F3C1E-5A2D-4C8E-9F10-2B3C4D5E6F70", "0B6F3C1E-5A2D-4C8E-9F10-2B3C4D5E6F70"},
		{"rfc 3339", "2026-09-28T10:15:00Z", "<time>"},
		{"rfc 3339 with offset and fraction", "2026-09-28T10:15:00.123456+09:00", "<time>"},
		{"a date alone is kept", "2026-09-28", "2026-09-28"},
		{"request id", map[string]any{"requestId": "abc123"}, map[string]any{"requestId": "<request-id>"}},
		{"latency number", map[string]any{"latencyMs": 12.5, "totalDuration": 3.0}, map[string]any{"latencyMs": "<number>", "totalDuration": "<number>"}},
		{"latency that is not a number", map[string]any{"latency": "slow"}, map[string]any{"latency": "slow"}},
		{"other numbers kept", map[string]any{"turns": 2.0}, map[string]any{"turns": 2.0}},
		{
			"validation reason dropped",
			map[string]any{"fields": []any{map[string]any{"field": "bogus", "location": "body", "reason": "Extra inputs are not permitted"}}},
			map[string]any{"fields": []any{map[string]any{"field": "bogus", "location": "body"}}},
		},
		{
			"nested",
			[]any{map[string]any{"id": "0b6f3c1e-5a2d-4c8e-9f10-2b3c4d5e6f70", "at": "2026-09-28T10:15:00Z"}},
			[]any{map[string]any{"id": "<uuid>", "at": "<time>"}},
		},
		{"null", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalise(c.in); !reflect.DeepEqual(got, c.out) {
				t.Errorf("got %#v, want %#v", got, c.out)
			}
		})
	}
}
