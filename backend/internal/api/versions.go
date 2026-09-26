// Version endpoints: a project's snapshots, addressed by version label.
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/store/postgres"
)

// versionParam decodes {version}; chi can hand it over still encoded. It is
// encoded only when chi routed on RawPath, so a literal '%' is not decoded twice.
func versionParam(r *http.Request) (string, error) {
	v := chi.URLParam(r, "version")
	if r.URL.RawPath == "" {
		return v, nil
	}
	d, err := url.PathUnescape(v)
	if err != nil {
		return "", fmt.Errorf("invalid version %q", v)
	}
	return d, nil
}

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := s.pg.ListVersions(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		s.failProject(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

func (s *Server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	version, err := versionParam(r)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	slug := chi.URLParam(r, "project")
	sn, err := s.pg.GetVersion(r.Context(), slug, version)
	if err != nil {
		s.failVersion(w, r, slug, err)
		return
	}
	writeJSON(w, http.StatusOK, sn)
}

func (s *Server) handleDeleteVersion(w http.ResponseWriter, r *http.Request) {
	version, err := versionParam(r)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	// Deleting "whatever is newest" by alias is too easy to do by accident.
	if version == "latest" {
		s.badRequest(w, "latest cannot be deleted; name the version")
		return
	}
	slug := chi.URLParam(r, "project")
	sid, _, err := s.pg.DeleteVersion(r.Context(), slug, version)
	if err != nil {
		s.failVersion(w, r, slug, err)
		return
	}
	// As for a project: the rows are gone, so the projection is cleared even if
	// the client hung up, and a failure is logged rather than returned.
	if err := s.graphs.DeleteSnapshot(context.WithoutCancel(r.Context()), sid); err != nil {
		s.log.Error("failed to delete graph projection", "project", slug, "snapshot", sid, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// failVersion tells a missing project apart from a missing version.
func (s *Server) failVersion(w http.ResponseWriter, r *http.Request, slug string, err error) {
	if !errors.Is(err, postgres.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if _, perr := s.pg.GetProject(r.Context(), slug); perr != nil {
		s.failProject(w, r, perr)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "version not found"})
}
