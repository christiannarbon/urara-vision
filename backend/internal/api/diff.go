// The version diff: what changed in a project's model between two versions.
package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/diff"
	"urara-vision/backend/internal/model"
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

	var sids [2]string
	for i, v := range versions {
		sn, err := s.pg.GetVersion(r.Context(), slug, v)
		if err != nil {
			s.failVersion(w, r, slug, fmt.Sprintf("version %s not found", v), err)
			return
		}
		sids[i] = sn.ID
	}

	// Loaded one after the other: errgroup is not a direct dependency.
	from, err := s.pg.LoadModel(r.Context(), sids[0])
	if err != nil {
		s.fail(w, r, err)
		return
	}
	to := from
	if sids[1] != sids[0] {
		if to, err = s.pg.LoadModel(r.Context(), sids[1]); err != nil {
			s.fail(w, r, err)
			return
		}
	}

	ref := func(m *model.Model) versionRef {
		return versionRef{Version: m.Snapshot.Project.Project.Version, SnapshotID: m.Snapshot.ID}
	}
	writeJSON(w, http.StatusOK, diffResponse{
		Project: slug,
		From:    ref(from),
		To:      ref(to),
		Result:  diff.Compare(from, to),
	})
}
