package httpapi

import (
	"net/http"

	"urara-vision/backend/internal/chat/llm"
)

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz bodies are not error bodies: no requestId, as in Python.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	info := llm.Describe(*s.settings)
	if s.backend.Health(r.Context()) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "backend": "ok", "llm": info})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"status":  "unready",
		"backend": "unreachable",
		"llm":     info,
		"reason":  "backend is not reachable or not ready",
	})
}
