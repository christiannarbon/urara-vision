package httpapi

import (
	"net/http"

	"urara-vision/backend/internal/chat/agent"
)

type answerRequest struct {
	SnapshotID      string  `json:"snapshotId"`
	SnapshotIDSnake string  `json:"snapshot_id"` // pydantic's populate_by_name
	Question        *string `json:"question"`
	Language        string  `json:"language"`
}

// answerResponse is Python's AnswerResponse.
type answerResponse struct {
	Text       string             `json:"text"`
	Citations  []string           `json:"citations"`
	ToolCalls  []agent.CallRecord `json:"toolCalls"`
	Iterations int                `json:"iterations"`
	Truncated  bool               `json:"truncated"`
	Model      string             `json:"model"`
	LatencyMS  int                `json:"latencyMs"`
	Usage      map[string]int     `json:"usage"`
}

func toResponse(a agent.Answer) answerResponse {
	a = withEmptyLists(a)
	return answerResponse{
		Text: a.Text, Citations: a.Citations, ToolCalls: a.ToolCalls, Iterations: a.Iterations,
		Truncated: a.Truncated, Model: a.Model, LatencyMS: a.LatencyMS, Usage: a.Usage,
	}
}

// withEmptyLists never leaves citations, tool calls or usage null: callers index into them.
func withEmptyLists(a agent.Answer) agent.Answer {
	if a.Citations == nil {
		a.Citations = []string{}
	}
	if a.ToolCalls == nil {
		a.ToolCalls = []agent.CallRecord{}
	}
	if a.Usage == nil {
		a.Usage = map[string]int{}
	}
	return a
}

// debugAnswer is one stateless turn through the whole pipeline, with no limiter.
func (s *Server) debugAnswer(w http.ResponseWriter, r *http.Request) {
	var body answerRequest
	if s.decodeAnswer(w, r, &body) {
		s.answer(w, r, body)
	}
}

// decodeAnswer checks the body's shape. On failure it has written the response.
func (s *Server) decodeAnswer(w http.ResponseWriter, r *http.Request, body *answerRequest) bool {
	if !s.DecodeJSON(w, r, body) {
		return false
	}
	if body.SnapshotID == "" {
		body.SnapshotID = body.SnapshotIDSnake
	}
	var missing []FieldProblem
	if body.SnapshotID == "" {
		missing = append(missing, Required("snapshotId"))
	}
	if body.Question == nil {
		missing = append(missing, Required("question"))
	}
	if len(missing) > 0 {
		FieldError(w, r, missing...)
		return false
	}
	return true
}

// answer runs one turn for a decoded body, persisting nothing.
func (s *Server) answer(w http.ResponseWriter, r *http.Request, body answerRequest) {
	// Cleaned before resolving, so a bad question costs no backend call.
	question, bad := cleanQuestion(*body.Question, s.settings.MaxQuestionChars)
	if bad != nil {
		bad.write(w, r)
		return
	}
	snapshotID, err := s.tools.ResolveSnapshot(r.Context(), body.SnapshotID)
	if err != nil {
		s.RenderBackendError(w, r, err)
		return
	}
	answer, err := s.runTurn(r.Context(), question, snapshotID, nil, normaliseLanguage(body.Language), "")
	if err != nil {
		s.RenderTurnError(w, r, err)
		return
	}
	s.logTurn(turnRecord(r.Context(), answer, snapshotID, ""))
	writeJSON(w, http.StatusOK, toResponse(answer))
}
