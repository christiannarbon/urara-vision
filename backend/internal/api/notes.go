// Sticky notes: route permissions are in the table; authorship is checked here.
package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/notes"
	"urara-vision/backend/internal/store/postgres"
)

type createNoteRequest struct {
	AnchorKind string `json:"anchorKind"`
	AnchorID   string `json:"anchorId"`
	Body       string `json:"body"`
	ParentID   string `json:"parentId"`
}

const msgResolveReply = "replies cannot be resolved"

type patchNoteRequest struct {
	Body     *string `json:"body"`
	Resolved *bool   `json:"resolved"`
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	if q.Get("anchorKind") == "" || q.Get("anchorId") == "" {
		s.badRequest(w, `query parameters "anchorKind" and "anchorId" are required`)
		return
	}
	kind, err := notes.ParseKind(q.Get("anchorKind"))
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	list, err := s.pg.ListNotes(r.Context(), sid, kind, q.Get("anchorId"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": list})
}

func (s *Server) handleNoteCounts(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	counts, err := s.pg.CountNotes(r.Context(), sid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"counts": counts})
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	// Service and anonymous callers have no name to sign a note with.
	p, _ := auth.PrincipalFrom(r.Context())
	if p.Kind != auth.KindUser {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
		return
	}
	var req createNoteRequest
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	body, err := notes.CheckBody(req.Body)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}

	n := model.Note{SnapshotID: sid, Body: body, AuthorID: p.UserID, AuthorName: authorName(p)}
	if req.ParentID != "" {
		// The store copies the parent's anchor; any in the request is ignored.
		n.ParentID = req.ParentID
	} else {
		kind, err := notes.ParseKind(req.AnchorKind)
		if err != nil {
			s.badRequest(w, err.Error())
			return
		}
		if req.AnchorID == "" {
			s.badRequest(w, `"anchorId" is required`)
			return
		}
		exists, err := s.pg.AnchorExists(r.Context(), sid, kind, req.AnchorID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !exists {
			s.badRequest(w, fmt.Sprintf("%s %s does not exist in this version", kind, req.AnchorID))
			return
		}
		n.AnchorKind, n.AnchorID = string(kind), req.AnchorID
	}

	created, err := s.pg.CreateNote(r.Context(), n)
	if err != nil {
		s.failNote(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handlePatchNote(w http.ResponseWriter, r *http.Request) {
	var req patchNoteRequest
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	if (req.Body == nil) == (req.Resolved == nil) {
		s.badRequest(w, `exactly one of "body" or "resolved" is required`)
		return
	}
	n, err := s.pg.GetNote(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.failNote(w, r, err)
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())

	var out *model.Note
	if req.Body != nil {
		if !canChangeNote(p, n) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
			return
		}
		body, err := notes.CheckBody(*req.Body)
		if err != nil {
			s.badRequest(w, err.Error())
			return
		}
		out, err = s.pg.UpdateNoteBody(r.Context(), n.ID, body)
		if err != nil {
			s.failNote(w, r, err)
			return
		}
	} else {
		if n.ParentID != "" {
			s.badRequest(w, msgResolveReply)
			return
		}
		out, err = s.pg.SetNoteResolved(r.Context(), n.ID, *req.Resolved, authorName(p))
		if errors.Is(err, postgres.ErrReplyDepth) {
			s.badRequest(w, msgResolveReply)
			return
		}
		if err != nil {
			s.failNote(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	n, err := s.pg.GetNote(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.failNote(w, r, err)
		return
	}
	if p, _ := auth.PrincipalFrom(r.Context()); !canChangeNote(p, n) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
		return
	}
	if err := s.pg.DeleteNote(r.Context(), n.ID); err != nil {
		s.failNote(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// canChangeNote: the author, or anyone with note.moderate.
func canChangeNote(p auth.Principal, n *model.Note) bool {
	return (p.Kind == auth.KindUser && n.AuthorID == p.UserID) || p.Can(auth.PermNoteModerate)
}

func authorName(p auth.Principal) string {
	if strings.TrimSpace(p.DisplayName) != "" {
		return p.DisplayName
	}
	return p.Username
}

func (s *Server) failNote(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, postgres.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "note not found"})
	case errors.Is(err, postgres.ErrReplyDepth):
		s.badRequest(w, "replies cannot be replied to")
	default:
		s.fail(w, r, err)
	}
}
