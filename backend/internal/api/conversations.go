// Chat conversation endpoints: starting a thread about a snapshot, reading it
// back, and appending turns to it.
//
// A conversation is addressed by its own ID rather than under a snapshot,
// because the snapshot it is about is decided once, when it is created. That is
// also where the "latest" alias is resolved: a thread is stored against the
// concrete ID it was started on, so re-ingesting cannot silently change what an
// existing transcript is talking about.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
)

// maxConversationBody caps a chat request body. Turns are prose, not uploads,
// so this is generous for anything a client should be sending and still small
// enough that a runaway body is refused rather than buffered.
const maxConversationBody = 1 << 20 // 1 MiB

type createConversationRequest struct {
	SnapshotID string `json:"snapshotId"`
	Title      string `json:"title"`
}

type appendMessageRequest struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Citations []string       `json:"citations"`
	Meta      map[string]any `json:"meta"`
}

// decodeBody reads a JSON request body into dst.
//
// Unknown fields are refused rather than ignored: a client that misspells a
// field should be told so, instead of watching the value it sent quietly fail
// to take effect.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxConversationBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

// handleCreateConversation starts a thread about one snapshot.
func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	var req createConversationRequest
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	if strings.TrimSpace(req.SnapshotID) == "" {
		s.badRequest(w, "\"snapshotId\" is required")
		return
	}

	// Resolved here and stored concrete: a thread holding the literal "latest"
	// would change subject on the next ingest, and its earlier answers would
	// then cite tables from a different model.
	sid, err := s.resolveSnapshotID(r.Context(), req.SnapshotID)
	if err != nil {
		s.failSnapshot(w, r, err)
		return
	}

	conv, err := s.pg.CreateConversation(r.Context(), sid, req.Title)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, conv)
}

// handleListConversations lists the threads about one snapshot, newest first
// and without their transcripts.
func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	snapshot := r.URL.Query().Get("snapshot")
	if strings.TrimSpace(snapshot) == "" {
		s.badRequest(w, "query parameter \"snapshot\" is required")
		return
	}
	sid, err := s.resolveSnapshotID(r.Context(), snapshot)
	if err != nil {
		s.failSnapshot(w, r, err)
		return
	}

	convs, err := s.pg.ListConversations(r.Context(), sid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": convs})
}

// handleGetConversation returns one thread with its full transcript.
func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	conv, err := s.pg.GetConversation(r.Context(), chi.URLParam(r, "cid"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, conv)
}

// handleDeleteConversation removes a thread and its messages.
func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	if err := s.pg.DeleteConversation(r.Context(), chi.URLParam(r, "cid")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAppendMessage adds one turn to a conversation. The ordinal is the
// store's to assign, so the stored message is returned rather than the one that
// was sent.
func (s *Server) handleAppendMessage(w http.ResponseWriter, r *http.Request) {
	var req appendMessageRequest
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	// The role is checked here rather than by a database constraint, so a bad
	// one is a message the caller can act on instead of a driver error.
	if !model.ValidRole(req.Role) {
		s.badRequest(w, fmt.Sprintf("%q is not a valid role; expected one of %q, %q or %q",
			req.Role, model.RoleUser, model.RoleAssistant, model.RoleSystem))
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		s.badRequest(w, "\"content\" is required")
		return
	}

	stored, err := s.pg.AppendMessage(r.Context(), chi.URLParam(r, "cid"), model.Message{
		Role:      req.Role,
		Content:   req.Content,
		Citations: req.Citations,
		Meta:      req.Meta,
	})
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, stored)
}
