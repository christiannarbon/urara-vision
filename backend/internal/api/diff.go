// The version diff: what changed in a project's model between two versions.
package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/diff"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
)

type versionRef struct {
	Version    string `json:"version"`
	SnapshotID string `json:"snapshotId"`
}

type diffResponse struct {
	Project string     `json:"project"`
	From    versionRef `json:"from"`
	To      versionRef `json:"to"`
	diff.Result
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "project")
	versions := [2]string{r.URL.Query().Get("from"), r.URL.Query().Get("to")}
	for i, name := range [2]string{"from", "to"} {
		if versions[i] == "" {
			s.badRequest(w, name+" is required")
			return
		}
	}

	var models [2]*model.Model
	for i, v := range versions {
		sn, err := s.pg.GetVersion(r.Context(), slug, v)
		if err != nil {
			s.failDiffVersion(w, r, slug, v, err)
			return
		}
		// Loaded one after the other: errgroup is not a direct dependency.
		m, err := s.pg.LoadModel(r.Context(), sn.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		models[i] = m
	}

	ref := func(m *model.Model) versionRef {
		return versionRef{Version: m.Snapshot.Project.Project.Version, SnapshotID: m.Snapshot.ID}
	}
	writeJSON(w, http.StatusOK, diffResponse{
		Project: slug,
		From:    ref(models[0]),
		To:      ref(models[1]),
		Result:  diff.Compare(models[0], models[1]),
	})
}

// failDiffVersion is failVersion with the version named, since two are in play.
func (s *Server) failDiffVersion(w http.ResponseWriter, r *http.Request, slug, v string, err error) {
	if !errors.Is(err, postgres.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if _, perr := s.pg.GetProject(r.Context(), slug); perr != nil {
		s.failProject(w, r, perr)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("version %s not found", v)})
}
