// Shared request and response plumbing: resolving the snapshot a request is
// about, mapping a store error to a status code, and reading query parameters.
package api

import (
	"context"
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

// errNoSnapshots is the "latest" alias asked for on an empty database. It is
// distinct from ErrNotFound because the two deserve different messages: one
// means the caller's ID is wrong, the other means there is nothing to name yet.
var errNoSnapshots = errors.New("no snapshots have been ingested yet")

// resolveSnapshotID turns the "latest" alias into a concrete snapshot ID and
// verifies that any other ID exists, so a typo is a 404 rather than an empty
// result that reads as a real but empty snapshot.
//
// It takes the ID rather than the request because the snapshot does not always
// arrive in the path: a conversation names one in its body or query string.
func (s *Server) resolveSnapshotID(ctx context.Context, sid string) (string, error) {
	if sid != "latest" {
		if _, err := s.pg.GetSnapshot(ctx, sid); err != nil {
			return "", err
		}
		return sid, nil
	}
	id, err := s.pg.LatestSnapshotID(ctx)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return "", errNoSnapshots
		}
		return "", err
	}
	return id, nil
}

// failSnapshot maps a resolveSnapshotID error onto a response.
func (s *Server) failSnapshot(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errNoSnapshots):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errNoSnapshots.Error()})
	case errors.Is(err, postgres.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot not found"})
	default:
		s.fail(w, r, err)
	}
}

// resolveSnapshot reads the snapshot ID from the path, translating the alias
// "latest" into the most recent ingest so the UI can deep-link without knowing
// an ID.
func (s *Server) resolveSnapshot(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := s.resolveSnapshotID(r.Context(), chi.URLParam(r, "sid"))
	if err != nil {
		s.failSnapshot(w, r, err)
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
