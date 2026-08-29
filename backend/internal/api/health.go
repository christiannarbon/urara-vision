// Liveness and readiness. Readiness names the dependency that is down, so a
// failing deploy points at the container to look at.
package api

import (
	"net/http"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]string{"postgres": "ok", "graph": "ok"}
	code := http.StatusOK

	if err := s.pg.Ping(ctx); err != nil {
		out["postgres"] = err.Error()
		code = http.StatusServiceUnavailable
	}
	if err := s.graphs.Ping(ctx); err != nil {
		out["graph"] = err.Error()
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, out)
}
