package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
)

// FieldProblem is one entry of a 400's "fields", as Python's _fields renders it.
type FieldProblem struct {
	Field    string `json:"field"`
	Location string `json:"location,omitempty"`
	Reason   string `json:"reason"`
}

// FieldError writes the 400 for a request that does not fit its schema.
func FieldError(w http.ResponseWriter, r *http.Request, problems ...FieldProblem) {
	WriteError(w, r, http.StatusBadRequest, map[string]any{
		"error":  "the request is invalid",
		"fields": problems,
	})
}

// Required is the problem for an absent body field.
func Required(field string) FieldProblem {
	return FieldProblem{Field: field, Location: "body", Reason: "Field required"}
}

func extraField(field string) FieldProblem {
	return FieldProblem{Field: field, Location: "body", Reason: "Extra inputs are not permitted"}
}

var malformedBody = FieldProblem{Field: "body", Location: "body", Reason: "JSON decode error"}

// DecodeJSON decodes the body into dst, refusing unknown fields. On failure it
// has written the response and returns false.
func (s *Server) DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			s.writeTooLarge(w, r)
		} else {
			FieldError(w, r, malformedBody)
		}
		return false
	}
	if extra := inexactKeys(raw, dst); len(extra) > 0 {
		problems := make([]FieldProblem, len(extra))
		for i, key := range extra {
			problems[i] = extraField(key)
		}
		FieldError(w, r, problems...)
		return false
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	err = dec.Decode(dst)
	if err == nil {
		// Only whitespace may follow the value, as FastAPI requires.
		if dec.Decode(&struct{}{}) == io.EOF {
			return true
		}
		FieldError(w, r, malformedBody)
		return false
	}

	var typeErr *json.UnmarshalTypeError
	switch {
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		FieldError(w, r, extraField(strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)))
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			field = "body"
		}
		FieldError(w, r, FieldProblem{Field: field, Location: "body", Reason: "Input should be a valid " + jsonKind(typeErr.Type)})
	case errors.Is(err, io.EOF):
		FieldError(w, r, Required("body"))
	default:
		FieldError(w, r, malformedBody)
	}
	return false
}

// inexactKeys lists top-level keys that are not exactly one of dst's JSON
// names. encoding/json folds case; pydantic does not.
func inexactKeys(raw []byte, dst any) []string {
	t := reflect.TypeOf(dst)
	if t == nil || t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct {
		return nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	names := map[string]bool{}
	for i := range t.Elem().NumField() {
		f := t.Elem().Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch {
		case name == "-" || !f.IsExported():
		case name == "":
			names[f.Name] = true
		default:
			names[name] = true
		}
	}
	var extra []string
	for key := range obj {
		if !names[key] {
			extra = append(extra, key)
		}
	}
	slices.Sort(extra)
	return extra
}

// jsonKind names a Go type in JSON's words, so no Go type reaches a caller.
func jsonKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		return "list"
	default:
		return "object"
	}
}
