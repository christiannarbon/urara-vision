package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/model"
)

type createConversationRequest struct {
	SnapshotID      string `json:"snapshotId"`
	SnapshotIDSnake string `json:"snapshot_id"` // pydantic's populate_by_name
	Title           string `json:"title"`
}

// conversationResponse is Python's ConversationResponse.
type conversationResponse struct {
	ID         string            `json:"id"`
	SnapshotID string            `json:"snapshotId"`
	Title      string            `json:"title"`
	CreatedAt  *time.Time        `json:"createdAt"`
	UpdatedAt  *time.Time        `json:"updatedAt"`
	Messages   []messageResponse `json:"messages"`
}

// messageResponse is Python's MessageResponse.
type messageResponse struct {
	Ordinal   int            `json:"ordinal"`
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Citations []string       `json:"citations"`
	Meta      map[string]any `json:"meta"`
	CreatedAt *time.Time     `json:"createdAt"`
}

// conversationJSON never leaves messages null.
func conversationJSON(c model.Conversation) conversationResponse {
	messages := make([]messageResponse, len(c.Messages))
	for i, m := range c.Messages {
		messages[i] = messageJSON(m)
	}
	return conversationResponse{
		ID: c.ID, SnapshotID: c.SnapshotID, Title: c.Title,
		CreatedAt: timeJSON(c.CreatedAt), UpdatedAt: timeJSON(c.UpdatedAt), Messages: messages,
	}
}

// messageJSON never leaves citations or meta null.
func messageJSON(m model.Message) messageResponse {
	out := messageResponse{
		Ordinal: m.Ordinal, Role: m.Role, Content: m.Content,
		Citations: m.Citations, Meta: m.Meta, CreatedAt: timeJSON(m.CreatedAt),
	}
	if out.Citations == nil {
		out.Citations = []string{}
	}
	if out.Meta == nil {
		out.Meta = map[string]any{}
	}
	return out
}

// timeJSON is null for a missing time, as Python's datetime | None.
func timeJSON(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	var body createConversationRequest
	if !s.DecodeJSON(w, r, &body) {
		return
	}
	sid := body.SnapshotID
	if sid == "" {
		sid = body.SnapshotIDSnake
	}
	if sid == "" {
		FieldError(w, r, Required("snapshotId"))
		return
	}
	// "latest" goes through as is; the backend resolves it.
	conv, err := s.backend.CreateConversation(r.Context(), sid, body.Title)
	if err != nil {
		s.RenderBackendError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, conversationJSON(conv))
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var problems []FieldProblem
	switch snapshot, ok := q["snapshot"]; {
	case !ok:
		problems = append(problems, FieldProblem{Field: "snapshot", Location: "query", Reason: "Field required"})
	case snapshot[0] == "":
		problems = append(problems, FieldProblem{Field: "snapshot", Location: "query", Reason: "String should have at least 1 character"})
	}
	limit := 0 // 0: the backend's default
	if raw, ok := q["limit"]; ok {
		n, err := strconv.Atoi(raw[0])
		switch {
		case err != nil:
			problems = append(problems, FieldProblem{Field: "limit", Location: "query", Reason: "Input should be a valid integer, unable to parse string as an integer"})
		case n < 1:
			problems = append(problems, FieldProblem{Field: "limit", Location: "query", Reason: "Input should be greater than or equal to 1"})
		default:
			limit = n
		}
	}
	if len(problems) > 0 {
		FieldError(w, r, problems...)
		return
	}

	convs, err := s.backend.ListConversations(r.Context(), q.Get("snapshot"), limit)
	if err != nil {
		s.RenderBackendError(w, r, err)
		return
	}
	out := make([]conversationResponse, len(convs))
	for i, c := range convs {
		out[i] = conversationJSON(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	conv, err := s.backend.GetConversation(r.Context(), chi.URLParam(r, "cid"))
	if err != nil {
		s.RenderBackendError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, conversationJSON(conv))
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.DeleteConversation(r.Context(), chi.URLParam(r, "cid")); err != nil {
		s.RenderBackendError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
