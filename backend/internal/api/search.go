// Endpoints that answer questions about a snapshot rather than returning part
// of the graph: search, diagnostics and the upstream models.
package api

import (
	"net/http"
)

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	hits, err := s.pg.Search(r.Context(), sid, q.Get("q"), atoiDefault(q.Get("limit"), 50))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	diags, err := s.pg.ListDiagnostics(r.Context(), sid, r.URL.Query().Get("severity"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"diagnostics": diags})
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	sources, err := s.pg.ListSourceTables(r.Context(), sid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
}
