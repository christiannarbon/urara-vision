package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

// DecodeJSON decodes the body into dst, refusing unknown fields. On failure it
// has written the response and returns false.
func (s *Server) DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil {
		return true
	}

	var tooLarge *http.MaxBytesError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooLarge):
		s.writeTooLarge(w, r)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		FieldError(w, r, FieldProblem{Field: field, Location: "body", Reason: "Extra inputs are not permitted"})
	case errors.As(err, &typeErr):
		FieldError(w, r, FieldProblem{
			Field: typeErr.Field, Location: "body",
			Reason: fmt.Sprintf("Input should be a valid %s", typeErr.Type),
		})
	case errors.Is(err, io.EOF):
		FieldError(w, r, FieldProblem{Field: "body", Location: "body", Reason: "Field required"})
	default:
		FieldError(w, r, FieldProblem{Field: "body", Location: "body", Reason: "JSON decode error"})
	}
	return false
}
