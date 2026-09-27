package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"runtime/debug"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/reqctx"
)

// What a caller is told: which side failed, and nothing else.
const (
	MsgNotFound           = "not found"
	MsgBackendUnavailable = "the model store is unavailable"
	MsgProviderFailed     = "the language model did not answer"
	MsgInternal           = "internal error"
	MsgChatTurnedOff      = "chat is turned off"
	MsgNotSignedIn        = "not signed in"
	MsgNotAllowed         = "not allowed"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// WriteError writes every error response in the service, adding requestId to
// the body and the header.
func WriteError(w http.ResponseWriter, r *http.Request, status int, fields map[string]any) {
	id := reqctx.RequestID(r.Context())
	body := maps.Clone(fields)
	if body == nil {
		body = map[string]any{}
	}
	body["requestId"] = id
	w.Header().Set(requestIDHeader, id)
	writeJSON(w, status, body)
}

// RenderBackendError maps a backend client error onto a response. The
// backend's own message is logged, never returned.
func (s *Server) RenderBackendError(w http.ResponseWriter, r *http.Request, err error) {
	id := reqctx.RequestID(r.Context())
	var apiErr *apiclient.Error
	switch {
	case errors.Is(err, apiclient.ErrNotFound):
		WriteError(w, r, http.StatusNotFound, map[string]any{"error": MsgNotFound})
	case errors.Is(err, apiclient.ErrForbidden):
		WriteError(w, r, http.StatusForbidden, map[string]any{"error": MsgNotAllowed})
	case errors.Is(err, apiclient.ErrRejected):
		// A 4xx means this service built a bad request.
		s.log.Error("the backend refused a request built here", "request_id", id, "backend_error", err.Error())
		WriteError(w, r, http.StatusInternalServerError, map[string]any{"error": MsgInternal})
	case errors.As(err, &apiErr):
		s.log.Error("backend call failed", "request_id", id, "backend_error", err.Error())
		WriteError(w, r, http.StatusBadGateway, map[string]any{"error": MsgBackendUnavailable})
	default:
		// A response that did not decode, as pydantic's ValidationError in Python.
		s.log.Error("a backend response did not match this service's models", "request_id", id, "error", err.Error())
		WriteError(w, r, http.StatusInternalServerError, map[string]any{"error": MsgInternal})
	}
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			s.log.Error("unhandled exception",
				"request_id", reqctx.RequestID(r.Context()),
				"error", fmt.Sprint(rec), "stack", string(debug.Stack()))
			WriteError(w, r, http.StatusInternalServerError, map[string]any{"error": MsgInternal})
		}()
		next.ServeHTTP(w, r)
	})
}
