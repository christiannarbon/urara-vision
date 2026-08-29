// Snapshot-level endpoints: listing ingests, reading one, and deleting one.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	snaps, err := s.pg.ListSnapshots(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": snaps})
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	snap, err := s.pg.GetSnapshot(r.Context(), sid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleDeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "sid")
	if err := s.pg.DeleteSnapshot(r.Context(), sid); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.graphs.DeleteSnapshot(r.Context(), sid); err != nil {
		// Postgres is the record of truth; a stale graph projection is
		// recoverable, so report success but log loudly.
		s.log.Error("failed to delete graph projection", "snapshot", sid, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}
