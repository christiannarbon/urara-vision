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
	r := answerResponse{
		Text: a.Text, Citations: a.Citations, ToolCalls: a.ToolCalls, Iterations: a.Iterations,
		Truncated: a.Truncated, Model: a.Model, LatencyMS: a.LatencyMS, Usage: a.Usage,
	}
	// Never null: callers index into these.
	if r.Citations == nil {
		r.Citations = []string{}
	}
	if r.ToolCalls == nil {
		r.ToolCalls = []agent.CallRecord{}
	}
	if r.Usage == nil {
		r.Usage = map[string]int{}
	}
	return r
}

// debugAnswer is one stateless turn through the whole pipeline.
func (s *Server) debugAnswer(w http.ResponseWriter, r *http.Request) {
	var body answerRequest
	if !s.DecodeJSON(w, r, &body) {
		return
	}
	sid := body.SnapshotID
	if sid == "" {
		sid = body.SnapshotIDSnake
	}
	var missing []FieldProblem
	if sid == "" {
		missing = append(missing, Required("snapshotId"))
	}
	if body.Question == nil {
		missing = append(missing, Required("question"))
	}
	if len(missing) > 0 {
		FieldError(w, r, missing...)
		return
	}

	// Cleaned before resolving, so a bad question costs no backend call.
	question, bad := cleanQuestion(*body.Question, s.settings.MaxQuestionChars)
	if bad != nil {
		bad.write(w, r)
		return
	}
	snapshotID, err := s.tools.ResolveSnapshot(r.Context(), sid)
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
