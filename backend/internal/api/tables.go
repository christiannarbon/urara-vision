// Domain and table endpoints, from the lists the sidebar renders to the full
// detail one pane shows.
package api

import (
	"net/http"
)

func (s *Server) handleListDomains(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	domains, err := s.pg.ListDomains(r.Context(), sid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": domains})
}

func (s *Server) handleListTables(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	tables, err := s.pg.ListTables(r.Context(), sid, r.URL.Query().Get("domain"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
}

// handleGetTable returns one table's full detail. Table IDs contain a slash
// ("domain_one/fact_primary"), so they travel as a query parameter rather than a path
// segment.
func (s *Server) handleGetTable(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		s.badRequest(w, "query parameter \"id\" is required")
		return
	}
	ctx := r.Context()

	table, err := s.pg.GetTable(ctx, sid, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	incoming, err := s.pg.IncomingRelationships(ctx, sid, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	upstream, err := s.graphs.Upstream(ctx, sid, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	siblings, err := s.graphs.SiblingsBySource(ctx, sid, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"table":    table,
		"incoming": incoming,
		"upstream": upstream,
		"siblings": siblings,
	})
}
