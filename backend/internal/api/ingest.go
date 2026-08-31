// The ingest endpoint: parse an uploaded documentation directory into a new
// snapshot, store it, and project it.
//
// The upload has to carry a projectmeta.toml, and it is checked before any
// document is read: a directory that will not say what project it is stays
// unparsed rather than becoming a nameless snapshot.
//
// Both stores have to end up holding the same snapshot, so a failed projection
// rolls the stored snapshot back rather than leaving an entry that lists but
// cannot be drawn.
package api

import (
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/parser"
	"urara-vision/backend/internal/projectmeta"
)

// handleIngest parses an uploaded documentation directory into a new snapshot.
// It accepts either a JSON body or multipart/form-data, where each part's field
// name is the file's path relative to the selected directory. Either shape must
// include the directory's projectmeta.toml.
func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		mediaType = "application/json"
	}

	var up *upload

	switch {
	case strings.HasPrefix(mediaType, "multipart/"):
		up, err = s.readMultipart(r)
	default:
		up, err = s.readJSON(r)
	}
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}

	// The manifest comes first: it is what the directory says it is, and
	// nothing is parsed without it.
	if !up.metaFound {
		msg := fmt.Sprintf("%s is required at the root of the selected directory", projectmeta.FileName)
		if len(up.nestedMeta) > 0 {
			msg += fmt.Sprintf("; one was found at %q, which is not the root", up.nestedMeta[0])
		}
		s.badRequest(w, msg)
		return
	}
	meta, err := projectmeta.Parse(up.meta)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}

	files := up.files
	if len(files) == 0 {
		s.badRequest(w, "no markdown files were supplied; select a directory containing .md documents")
		return
	}
	if len(files) > s.cfg.MaxFiles {
		s.badRequest(w, fmt.Sprintf("too many files: %d exceeds the limit of %d", len(files), s.cfg.MaxFiles))
		return
	}

	name := up.name
	if strings.TrimSpace(name) == "" {
		name = defaultName(up.sourceLabel)
	}

	snapshotID := uuid.NewString()
	started := time.Now()

	m := graph.Build(snapshotID, name, up.sourceLabel, meta, parser.Parse(files))
	m.Snapshot.CreatedAt = time.Now().UTC()
	edges := graph.Edges(m)

	if err := s.pg.SaveSnapshot(ctx, m); err != nil {
		s.fail(w, r, fmt.Errorf("save snapshot: %w", err))
		return
	}
	if err := s.graphs.Project(ctx, m, edges); err != nil {
		// The snapshot is stored but unqueryable as a graph. Roll it back so
		// the user does not see a half-working entry in the snapshot list.
		if delErr := s.pg.DeleteSnapshot(ctx, snapshotID); delErr != nil {
			s.log.Error("rollback after failed projection", "snapshot", snapshotID, "error", delErr)
		}
		s.fail(w, r, fmt.Errorf("project graph: %w", err))
		return
	}

	s.log.Info("ingested snapshot",
		"snapshot", snapshotID,
		"project", meta.Project.Name,
		"version", meta.Project.Version,
		"files", len(files),
		"tables", m.Snapshot.Stats.Tables,
		"edges", len(edges),
		"duration_ms", time.Since(started).Milliseconds())

	writeJSON(w, http.StatusCreated, map[string]any{
		"snapshot":    m.Snapshot,
		"edges":       len(edges),
		"diagnostics": m.Diagnostics,
	})
}
