// Shared request and response plumbing: resolving the snapshot a request is
// about, mapping a store error to a status code, and reading query parameters.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"urara-vision/backend/internal/store/postgres"
)

// resolveSnapshot reads the snapshot ID from the path, translating the alias
// "latest" into the most recent ingest so the UI can deep-link without knowing
// an ID.
func (s *Server) resolveSnapshot(w http.ResponseWriter, r *http.Request) (string, bool) {
	sid := chi.URLParam(r, "sid")
	if sid != "latest" {
		// Verify it exists, so a typo returns 404 rather than an empty graph
		// that reads as a real snapshot with nothing in it.
		if _, err := s.pg.GetSnapshot(r.Context(), sid); err != nil {
			if errors.Is(err, postgres.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot not found"})
				return "", false
			}
			s.fail(w, r, err)
			return "", false
		}
		return sid, true
	}
	id, err := s.pg.LatestSnapshotID(r.Context())
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "no snapshots have been ingested yet",
			})
			return "", false
		}
		s.fail(w, r, err)
		return "", false
	}
	return id, true
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, postgres.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	s.log.Error("request failed",
		"path", r.URL.Path,
		"request_id", middleware.GetReqID(r.Context()),
		"error", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

func (s *Server) badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already written; nothing useful remains but a log.
		slog.Error("encode response", "error", err)
	}
}

func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func atoiDefault(v string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}
