package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Problem is one broken rule, shaped as a pydantic error for /debug/tool.
type Problem struct {
	Type   string         `json:"type"`
	Loc    []any          `json:"loc"`
	Msg    string         `json:"msg"`
	Input  any            `json:"input"`
	Ctx    map[string]any `json:"ctx,omitempty"`
	Reason string         `json:"-"` // what the model is told
}

// ArgsError lists every problem with one call's arguments.
type ArgsError struct {
	Problems []Problem
}

func (e *ArgsError) Error() string {
	reasons := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		reasons[i] = p.Reason
	}
	return strings.Join(reasons, "; ")
}

type kind int

const (
	kindString kind = iota
	kindInt
	kindStrings
)

type field struct {
	name     string
	kind     kind
	required bool
	nullable bool
	min, max int      // int value, or list length
	enum     []string // string only
}

var fields = map[string][]field{
	"list_domains": nil,
	"list_tables":  {{name: "domain", nullable: true}},
	"get_tables":   {{name: "ids", kind: kindStrings, required: true, min: 1, max: 8}},
	"search_model": {
		{name: "query", required: true},
		{name: "limit", kind: kindInt, min: 1, max: 50},
	},
	"get_neighbourhood": {
		{name: "table_id", required: true},
		{name: "depth", kind: kindInt, min: 1, max: 3},
	},
	"find_join_paths": {
		{name: "from_table", required: true},
		{name: "to_table", required: true},
		{name: "max_depth", kind: kindInt, min: 1, max: 6},
	},
	"get_lineage": {
		{name: "table_id", required: true},
		{name: "direction", enum: []string{"upstream", "downstream"}},
	},
	"list_diagnostics":   {{name: "severity", nullable: true, enum: []string{"error", "warning", "info"}}},
	"list_source_models": nil,
}

// Validate checks args against the tool's schema by hand, reporting every
// problem. Numbers must be integer literals: 5.0 is refused, unlike pydantic.
func Validate(name string, args json.RawMessage) error {
	spec, ok := fields[name]
	if !ok {
		return fmt.Errorf("unknown tool %q", name)
	}
	var obj map[string]json.RawMessage
	var whole any = map[string]any{}
	if len(bytes.TrimSpace(args)) > 0 {
		whole = decodeAny(args)
		if err := json.Unmarshal(args, &obj); err != nil || obj == nil {
			return &ArgsError{Problems: []Problem{{
				Type: "dict_type", Loc: []any{}, Msg: "Input should be a valid dictionary",
				Input: whole, Reason: "arguments must be a JSON object",
			}}}
		}
	}

	var problems []Problem
	known := map[string]bool{}
	for _, f := range spec {
		known[f.name] = true
		raw, present := obj[f.name]
		if !present || (f.nullable && isNull(raw)) {
			if present || !f.required {
				continue
			}
			problems = append(problems, Problem{
				Type: "missing", Loc: []any{f.name}, Msg: "Field required", Input: whole,
				Reason: f.name + ": required",
			})
			continue
		}
		problems = append(problems, f.check(raw)...)
	}

	var extra []string
	for key := range obj {
		if !known[key] {
			extra = append(extra, key)
		}
	}
	slices.Sort(extra)
	for _, key := range extra {
		problems = append(problems, Problem{
			Type: "extra_forbidden", Loc: []any{key}, Msg: "Extra inputs are not permitted", Input: decodeAny(obj[key]),
			Reason: fmt.Sprintf("%s: not an argument of %s", key, name),
		})
	}
	if len(problems) > 0 {
		return &ArgsError{Problems: problems}
	}
	return nil
}

func (f field) check(raw json.RawMessage) []Problem {
	input := decodeAny(raw)
	switch f.kind {
	case kindInt:
		var n int
		// Unmarshal takes null into an int as a no-op, so it is refused here.
		if err := json.Unmarshal(raw, &n); err != nil || isNull(raw) {
			p := Problem{Type: "int_type", Msg: "Input should be a valid integer", Reason: f.name + ": must be a whole number"}
			if v, isNum := input.(float64); isNum && v != math.Trunc(v) {
				p.Type, p.Msg = "int_from_float", "Input should be a valid integer, got a number with a fractional part"
			} else if isNum {
				// 5.0 is whole but tools decode into int, so it is still refused.
				p.Reason = fmt.Sprintf("%s: must be written as a whole number, e.g. %d", f.name, int64(v))
			}
			p.Loc, p.Input = []any{f.name}, input
			return []Problem{p}
		}
		between := fmt.Sprintf("%s: must be between %d and %d, got %d", f.name, f.min, f.max, n)
		if n < f.min {
			return []Problem{{Type: "greater_than_equal", Loc: []any{f.name}, Input: input, Reason: between,
				Msg: fmt.Sprintf("Input should be greater than or equal to %d", f.min), Ctx: map[string]any{"ge": f.min}}}
		}
		if n > f.max {
			return []Problem{{Type: "less_than_equal", Loc: []any{f.name}, Input: input, Reason: between,
				Msg: fmt.Sprintf("Input should be less than or equal to %d", f.max), Ctx: map[string]any{"le": f.max}}}
		}
	case kindStrings:
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil || items == nil {
			return []Problem{{Type: "list_type", Loc: []any{f.name}, Msg: "Input should be a valid list", Input: input,
				Reason: f.name + ": must be a list of strings"}}
		}
		var problems []Problem
		for i, item := range items {
			var s string
			if json.Unmarshal(item, &s) != nil || isNull(item) {
				problems = append(problems, Problem{Type: "string_type", Loc: []any{f.name, i}, Msg: "Input should be a valid string",
					Input: decodeAny(item), Reason: fmt.Sprintf("%s[%d]: must be a string", f.name, i)})
			}
		}
		if len(problems) > 0 {
			return problems
		}
		between := fmt.Sprintf("%s: must have between %d and %d items, got %d", f.name, f.min, f.max, len(items))
		ctx := map[string]any{"field_type": "List", "actual_length": len(items)}
		if len(items) < f.min {
			ctx["min_length"] = f.min
			return []Problem{{Type: "too_short", Loc: []any{f.name}, Input: input, Ctx: ctx, Reason: between,
				Msg: fmt.Sprintf("List should have at least %d item%s after validation, not %d", f.min, plural(f.min), len(items))}}
		}
		if len(items) > f.max {
			ctx["max_length"] = f.max
			return []Problem{{Type: "too_long", Loc: []any{f.name}, Input: input, Ctx: ctx, Reason: between,
				Msg: fmt.Sprintf("List should have at most %d item%s after validation, not %d", f.max, plural(f.max), len(items))}}
		}
	default:
		var s string
		if json.Unmarshal(raw, &s) != nil || isNull(raw) {
			return []Problem{{Type: "string_type", Loc: []any{f.name}, Msg: "Input should be a valid string", Input: input,
				Reason: f.name + ": must be a string"}}
		}
		if len(f.enum) > 0 && !slices.Contains(f.enum, s) {
			expected := quotedList(f.enum)
			return []Problem{{Type: "literal_error", Loc: []any{f.name}, Input: input,
				Msg: "Input should be " + expected, Ctx: map[string]any{"expected": expected},
				Reason: fmt.Sprintf("%s: must be %s, got %q", f.name, expected, s)}}
		}
	}
	return nil
}

// quotedList renders pydantic's "'a', 'b' or 'c'".
func quotedList(values []string) string {
	q := make([]string, len(values))
	for i, v := range values {
		q[i] = "'" + v + "'"
	}
	if len(q) == 1 {
		return q[0]
	}
	return strings.Join(q[:len(q)-1], ", ") + " or " + q[len(q)-1]
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func isNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

func decodeAny(raw json.RawMessage) any {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	return v
}
