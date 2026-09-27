package httpapi

import (
	"net/http"
	"strings"

	"urara-vision/backend/internal/chat/config"
)

// ModelInfo describes the configured model from settings alone, as Python's
// describe_model. Phase 17 replaces it with the provider's own description.
func ModelInfo(s *config.Settings) map[string]string {
	info := map[string]string{"provider": s.LLMProvider, "model": s.LLMModel}
	if strings.HasPrefix(s.LLMProvider, "vertex") {
		info["location"] = s.VertexLocation
	}
	return info
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz bodies are not error bodies: no requestId, as in Python.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if s.backend.Health(r.Context()) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "backend": "ok", "llm": s.modelInfo})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"status":  "unready",
		"backend": "unreachable",
		"llm":     s.modelInfo,
		"reason":  "backend is not reachable or not ready",
	})
}
