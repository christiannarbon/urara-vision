// Chat conversation endpoints: starting a thread about a snapshot, reading it
// back, renaming it, and appending turns to it.
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
	"unicode/utf8"

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

// maxConversationTitleRunes caps a title. Counted in runes rather than bytes so
// the limit a caller is told about is the one they can count in what they sent.
const maxConversationTitleRunes = 200

// patchConversationRequest is the whole of what a conversation can be changed
// to. It has one field on purpose: with DisallowUnknownFields, anything else --
// "snapshotId" above all -- is a 400 rather than a silently ignored edit.
type patchConversationRequest struct {
	Title string `json:"title"`
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
	// One JSON value and nothing else. A second object after the first is a
	// client bug, and accepting it would silently discard whatever it meant.
	if dec.More() {
		return errors.New("invalid JSON body: unexpected data after the top-level object")
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

	convs, err := s.pg.ListConversations(r.Context(), sid, listLimit(r.URL.Query().Get("limit")))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": convs})
}

// How many threads a listing returns, and the most it will return however it is
// asked. There is no existing cap to copy: atoiDefault supplies a default and
// nothing else, so ?limit=1000000 is honoured everywhere it is used. Clamped
// rather than refused, because a caller asking for more than exists is not
// making a mistake worth a 400 -- they are asking for everything, and this is
// everything.
const (
	defaultConversationLimit = 50
	maxConversationLimit     = 200
)

// listLimit reads the limit parameter, defaulting and clamping it.
func listLimit(raw string) int {
	limit := atoiDefault(raw, defaultConversationLimit)
	if limit < 1 {
		return defaultConversationLimit
	}
	return min(limit, maxConversationLimit)
}

// failConversation maps a store error onto a response, naming the conversation
// rather than leaving a bare "not found" the caller has to interpret.
func (s *Server) failConversation(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, postgres.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
		return
	}
	s.fail(w, r, err)
}

// handleGetConversation returns one thread with its full transcript.
func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	conv, err := s.pg.GetConversation(r.Context(), chi.URLParam(r, "cid"))
	if err != nil {
		s.failConversation(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, conv)
}

// handlePatchConversation sets a conversation's title, which is the only thing
// about a thread that can change.
//
// The snapshot in particular is not patchable. Resolving it once, when the
// thread is created, is what stops a transcript changing subject under a later
// ingest; an editable snapshot ID would hand that back.
func (s *Server) handlePatchConversation(w http.ResponseWriter, r *http.Request) {
	var req patchConversationRequest
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	// Refused rather than truncated: a caller whose title came back shorter
	// than they sent it, under a 200 saying all was well, has no way to notice.
	if n := utf8.RuneCountInString(req.Title); n > maxConversationTitleRunes {
		s.badRequest(w, fmt.Sprintf("\"title\" is %d characters; the limit is %d",
			n, maxConversationTitleRunes))
		return
	}

	// An empty title is a legitimate edit -- it clears one -- so it is not
	// checked for, unlike a message's content.
	conv, err := s.pg.UpdateConversationTitle(r.Context(), chi.URLParam(r, "cid"), req.Title)
	if err != nil {
		s.failConversation(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, conv)
}

// handleDeleteConversation removes a thread and its messages.
func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	if err := s.pg.DeleteConversation(r.Context(), chi.URLParam(r, "cid")); err != nil {
		s.failConversation(w, r, err)
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
		s.failConversation(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, stored)
}
