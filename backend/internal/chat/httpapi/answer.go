package httpapi

import "net/http"

// chatAnswer is /debug/answer behind a turn slot. The slot is taken before the
// question is cleaned, as in Python.
func (s *Server) chatAnswer(w http.ResponseWriter, r *http.Request) {
	var body answerRequest
	if !s.decodeAnswer(w, r, &body) {
		return
	}
	release, err := s.limiter.Acquire(r.Context())
	if err != nil {
		s.RenderTurnError(w, r, err)
		return
	}
	defer release()
	s.answer(w, r, body)
}
