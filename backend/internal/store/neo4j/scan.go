// Conversions between Go values and the driver's untyped ones.
//
// The driver returns every value as an any, and a query that stops returning a
// column starts returning nil rather than failing. Reading through these keeps
// a missing or retyped column a zero value instead of a panic. The same applies
// going the other way: Cypher parameters have to be []any, not []string.
package neo4j

import (
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func str(r *neo4j.Record, key string) string {
	v, ok := r.Get(key)
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func integer(r *neo4j.Record, key string) int {
	v, ok := r.Get(key)
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

func boolean(r *neo4j.Record, key string) bool {
	v, ok := r.Get(key)
	if !ok || v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

func strSlice(r *neo4j.Record, key string) []string {
	v, ok := r.Get(key)
	if !ok || v == nil {
		return []string{}
	}
	list, ok := v.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mapStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func toAnySlice(v []string) []any {
	out := make([]any, len(v))
	for i, s := range v {
		out[i] = s
	}
	return out
}
