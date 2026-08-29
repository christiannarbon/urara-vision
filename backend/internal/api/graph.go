// Graph endpoints: the filtered overview, one table's neighbourhood, the join
// paths between two tables, and lineage in either direction.
package api

import (
	"net/http"

	neostore "urara-vision/backend/internal/store/neo4j"
)

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	opt := neostore.GraphOptions{
		Domains:         splitCSV(q.Get("domain")),
		Kinds:           splitCSV(q.Get("kind")),
		IncludeSources:  q.Get("sources") == "true",
		CrossDomainOnly: q.Get("crossDomainOnly") == "true",
	}
	g, err := s.graphs.GetGraph(r.Context(), sid, opt)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleNeighborhood(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	id := q.Get("table")
	if id == "" {
		s.badRequest(w, "query parameter \"table\" is required")
		return
	}
	depth := atoiDefault(q.Get("depth"), 1)
	g, err := s.graphs.Neighborhood(r.Context(), sid, id, depth, q.Get("sources") == "true")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handlePaths(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" || to == "" {
		s.badRequest(w, "query parameters \"from\" and \"to\" are required")
		return
	}
	paths, err := s.graphs.FindPaths(r.Context(), sid, from, to,
		atoiDefault(q.Get("maxDepth"), 4), atoiDefault(q.Get("limit"), 10))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"paths": paths})
}

func (s *Server) handleLineage(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		s.badRequest(w, "query parameter \"id\" is required")
		return
	}
	ctx := r.Context()

	switch q.Get("direction") {
	case "downstream":
		entries, err := s.graphs.Downstream(ctx, sid, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"direction": "downstream", "entries": entries})
	default:
		entries, err := s.graphs.Upstream(ctx, sid, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"direction": "upstream", "entries": entries})
	}
}
