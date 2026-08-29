// The ingest endpoint: parse an uploaded documentation directory into a new
// snapshot, store it, and project it.
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
)

// handleIngest parses an uploaded documentation directory into a new snapshot.
// It accepts either a JSON body or multipart/form-data, where each part's field
// name is the file's path relative to the selected directory.
func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		mediaType = "application/json"
	}

	var (
		files       []parser.File
		name        string
		sourceLabel string
	)

	switch {
	case strings.HasPrefix(mediaType, "multipart/"):
		files, name, sourceLabel, err = s.readMultipart(r)
	default:
		files, name, sourceLabel, err = s.readJSON(r)
	}
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}

	if len(files) == 0 {
		s.badRequest(w, "no markdown files were supplied; select a directory containing .md documents")
		return
	}
	if len(files) > s.cfg.MaxFiles {
		s.badRequest(w, fmt.Sprintf("too many files: %d exceeds the limit of %d", len(files), s.cfg.MaxFiles))
		return
	}

	if strings.TrimSpace(name) == "" {
		name = defaultName(sourceLabel)
	}

	snapshotID := uuid.NewString()
	started := time.Now()

	m := graph.Build(snapshotID, name, sourceLabel, parser.Parse(files))
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
