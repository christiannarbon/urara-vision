package tools_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"urara-vision/backend/internal/chat/tools"
)

func problems(t *testing.T, name, args string) []tools.Problem {
	t.Helper()
	err := tools.Validate(name, json.RawMessage(args))
	if err == nil {
		t.Fatalf("%s %s: accepted", name, args)
	}
	var argsErr *tools.ArgsError
	if !errors.As(err, &argsErr) {
		t.Fatalf("%s %s: %v is not an ArgsError", name, args, err)
	}
	return argsErr.Problems
}

func idsArg(n int) string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%q", fmt.Sprintf("d/t%d", i))
	}
	return `{"ids":[` + strings.Join(ids, ",") + `]}`
}

// Out of range is an error the model is told about, not a 400 it must interpret.
func TestValidateEnforcesEveryBound(t *testing.T) {
	for _, c := range []struct{ name, args, reason string }{
		{"get_tables", `{"ids":[]}`, "ids: must have between 1 and 8 items, got 0"},
		{"get_tables", idsArg(9), "ids: must have between 1 and 8 items, got 9"},
		{"get_tables", `{}`, "ids: required"},
		{"get_tables", `{"ids":"d/t"}`, "ids: must be a list of strings"},
		{"get_tables", `{"ids":["d/t",3]}`, "ids[1]: must be a string"},
		{"search_model", `{"query":"x","limit":51}`, "limit: must be between 1 and 50, got 51"},
		{"search_model", `{"query":"x","limit":500}`, "limit: must be between 1 and 50, got 500"},
		{"search_model", `{"query":"x","limit":0}`, "limit: must be between 1 and 50, got 0"},
		{"search_model", `{"query":"x","limit":2.5}`, "limit: must be a whole number"},
		{"search_model", `{"query":"x","limit":"5"}`, "limit: must be a whole number"},
		{"search_model", `{"query":"x","limit":5.0}`, "limit: must be written as a whole number, e.g. 5"},
		{"search_model", `{"query":null}`, "query: must be a string"},
		{"search_model", `{"query":"x","limt":5}`, "limt: not an argument of search_model"},
		{"get_neighbourhood", `{"table_id":"d/t","depth":0}`, "depth: must be between 1 and 3, got 0"},
		{"get_neighbourhood", `{"table_id":"d/t","depth":4}`, "depth: must be between 1 and 3, got 4"},
		{"find_join_paths", `{"from_table":"a/x","to_table":"b/y","max_depth":7}`, "max_depth: must be between 1 and 6, got 7"},
		{"find_join_paths", `{"from_table":"a/x"}`, "to_table: required"},
		{"get_lineage", `{"table_id":"d/t","direction":"sideways"}`, `direction: must be 'upstream' or 'downstream', got "sideways"`},
		{"list_diagnostics", `{"severity":"fatal"}`, `severity: must be 'error', 'warning' or 'info', got "fatal"`},
		{"list_domains", `{"x":1}`, "x: not an argument of list_domains"},
		{"list_domains", `[]`, "arguments must be a JSON object"},
	} {
		err := tools.Validate(c.name, json.RawMessage(c.args))
		if err == nil || err.Error() != c.reason {
			t.Errorf("%s %s: %v, want %q", c.name, c.args, err, c.reason)
		}
	}
}

func TestValidatePassesValidArguments(t *testing.T) {
	for _, c := range []struct{ name, args string }{
		{"get_tables", idsArg(8)},
		{"get_tables", idsArg(1)},
		{"search_model", `{"query":"x"}`},
		{"search_model", `{"query":"x","limit":50}`},
		{"get_neighbourhood", `{"table_id":"d/t","depth":3}`},
		{"find_join_paths", `{"from_table":"a/x","to_table":"b/y","max_depth":6}`},
		{"get_lineage", `{"table_id":"d/t","direction":"downstream"}`},
		{"list_tables", `{"domain":null}`},
		{"list_tables", `{"domain":"ordering"}`},
		{"list_diagnostics", `{"severity":null}`},
		{"list_diagnostics", `{"severity":"info"}`},
		{"list_domains", ``},
		{"list_source_models", `{}`},
	} {
		if err := tools.Validate(c.name, json.RawMessage(c.args)); err != nil {
			t.Errorf("%s %s: %v", c.name, c.args, err)
		}
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	p := problems(t, "find_join_paths", `{"max_depth":9,"zzz":1}`)
	var got []string
	for _, x := range p {
		got = append(got, x.Reason)
	}
	want := []string{"from_table: required", "to_table: required", "max_depth: must be between 1 and 6, got 9", "zzz: not an argument of find_join_paths"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reasons = %q", got)
	}
}

// The /debug/tool 400 carries pydantic's error shape.
func TestValidateProblemsArePydanticShaped(t *testing.T) {
	for _, c := range []struct {
		name, args string
		want       map[string]any
	}{
		{"search_model", `{"query":"x","limit":500}`, map[string]any{
			"type": "less_than_equal", "loc": []any{"limit"}, "msg": "Input should be less than or equal to 50",
			"input": 500.0, "ctx": map[string]any{"le": 50.0},
		}},
		{"get_neighbourhood", `{"table_id":"d/t","depth":0}`, map[string]any{
			"type": "greater_than_equal", "loc": []any{"depth"}, "msg": "Input should be greater than or equal to 1",
			"input": 0.0, "ctx": map[string]any{"ge": 1.0},
		}},
		{"get_tables", `{"ids":[]}`, map[string]any{
			"type": "too_short", "loc": []any{"ids"}, "msg": "List should have at least 1 item after validation, not 0",
			"input": []any{}, "ctx": map[string]any{"field_type": "List", "min_length": 1.0, "actual_length": 0.0},
		}},
		{"get_lineage", `{"table_id":"d/t","direction":"sideways"}`, map[string]any{
			"type": "literal_error", "loc": []any{"direction"}, "msg": "Input should be 'upstream' or 'downstream'",
			"input": "sideways", "ctx": map[string]any{"expected": "'upstream' or 'downstream'"},
		}},
		{"search_model", `{}`, map[string]any{
			"type": "missing", "loc": []any{"query"}, "msg": "Field required", "input": map[string]any{},
		}},
		{"search_model", `{"query":"x","limt":5}`, map[string]any{
			"type": "extra_forbidden", "loc": []any{"limt"}, "msg": "Extra inputs are not permitted", "input": 5.0,
		}},
	} {
		b, _ := json.Marshal(problems(t, c.name, c.args)[0])
		var got map[string]any
		_ = json.Unmarshal(b, &got)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %s:\n got %v\nwant %v", c.name, c.args, got, c.want)
		}
	}
}

func TestValidateRefusesAnUnknownTool(t *testing.T) {
	if err := tools.Validate("nope", nil); err == nil {
		t.Error("accepted")
	}
}

// Validation covers exactly the schema each tool shows the model.
func TestValidateCoversEverySchemaProperty(t *testing.T) {
	for _, s := range tools.Build(&fakeBackend{}, "snap-1") {
		props, _ := decoded(t, s.Schema)["properties"].(map[string]any)
		for name := range props {
			if err := tools.Validate(s.Name, json.RawMessage(`{"`+name+`":{"not":"valid"}}`)); err == nil ||
				strings.Contains(err.Error(), "not an argument") {
				t.Errorf("%s.%s is not validated: %v", s.Name, name, err)
			}
		}
	}
}

// The model is shown the schemas; validation must enforce exactly what they say.
func TestValidateAgreesWithEverySchema(t *testing.T) {
	for _, s := range tools.Build(&fakeBackend{}, "s") {
		schema := decoded(t, s.Schema)
		props, _ := schema["properties"].(map[string]any)
		var required []string
		for _, r := range asList(schema["required"]) {
			required = append(required, r.(string))
		}
		// args fills every required property with a valid value, then applies set.
		args := func(set map[string]any, omit string) json.RawMessage {
			m := map[string]any{}
			for _, name := range required {
				if name == omit {
					continue
				}
				if props[name].(map[string]any)["type"] == "array" {
					m[name] = []any{"d/t"}
				} else {
					m[name] = "d/t"
				}
			}
			for k, v := range set {
				m[k] = v
			}
			b, _ := json.Marshal(m)
			return b
		}
		check := func(what string, raw json.RawMessage, pass bool) {
			t.Helper()
			if err := tools.Validate(s.Name, raw); (err == nil) != pass {
				t.Errorf("%s %s %s: %v, want pass %v", s.Name, what, raw, err, pass)
			}
		}

		check("all required", args(nil, ""), true)
		for _, name := range required {
			err := tools.Validate(s.Name, args(nil, name))
			if err == nil || !strings.Contains(err.Error(), name+": required") {
				t.Errorf("%s without %s: %v", s.Name, name, err)
			}
		}
		for name, p := range props {
			prop := p.(map[string]any)
			if lo, ok := prop["minimum"].(float64); ok {
				check(name+" minimum", args(map[string]any{name: lo}, ""), true)
				check(name+" below minimum", args(map[string]any{name: lo - 1}, ""), false)
			}
			if hi, ok := prop["maximum"].(float64); ok {
				check(name+" maximum", args(map[string]any{name: hi}, ""), true)
				check(name+" above maximum", args(map[string]any{name: hi + 1}, ""), false)
			}
			list := func(n float64) []any {
				out := make([]any, int(n))
				for i := range out {
					out[i] = fmt.Sprintf("d/t%d", i)
				}
				return out
			}
			if lo, ok := prop["minItems"].(float64); ok {
				check(name+" minItems", args(map[string]any{name: list(lo)}, ""), true)
				check(name+" below minItems", args(map[string]any{name: list(lo - 1)}, ""), false)
			}
			if hi, ok := prop["maxItems"].(float64); ok {
				check(name+" maxItems", args(map[string]any{name: list(hi)}, ""), true)
				check(name+" above maxItems", args(map[string]any{name: list(hi + 1)}, ""), false)
			}
			for _, v := range asList(prop["enum"]) {
				check(name+" enum", args(map[string]any{name: v}, ""), true)
			}
			if prop["enum"] != nil {
				check(name+" outside enum", args(map[string]any{name: "zzz"}, ""), false)
			}
		}
	}
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// 5.0 has no fractional part, so it is not int_from_float.
func TestValidateNamesIntegralFloatsAsIntType(t *testing.T) {
	for args, want := range map[string]string{`{"query":"x","limit":5.0}`: "int_type", `{"query":"x","limit":2.5}`: "int_from_float"} {
		if p := problems(t, "search_model", args); p[0].Type != want {
			t.Errorf("%s: %s, want %s", args, p[0].Type, want)
		}
	}
}
