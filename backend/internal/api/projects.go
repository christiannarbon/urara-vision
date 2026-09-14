// Project endpoints: listing projects, reading one, and deleting one with its snapshots.
package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/store/postgres"
)

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.pg.ListProjects(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.pg.GetProject(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		s.failProject(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "project")
	ids, err := s.pg.DeleteProject(r.Context(), slug)
	if err != nil {
		s.failProject(w, r, err)
		return
	}
	for _, sid := range ids {
		// As for a snapshot: Postgres is the record of truth, so a stale graph
		// projection is logged rather than failing the request.
		if err := s.graphs.DeleteSnapshot(r.Context(), sid); err != nil {
			s.log.Error("failed to delete graph projection", "project", slug, "snapshot", sid, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) failProject(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, postgres.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	s.fail(w, r, err)
}
