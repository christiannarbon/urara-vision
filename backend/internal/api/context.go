// The agent-facing catalogue endpoint: one call that says what a snapshot
// contains, so a model can go straight to the right table instead of probing
// the other endpoints to find out what exists.
//
// The response is injected into the system prompt on every turn, which decides
// its shape. It is a catalogue and not a dump: prose is truncated, and past a
// configured table count the list is dropped entirely rather than trimmed,
// because a silently shortened catalogue would read as a complete one.
//
// The response types are unexported structs here rather than in internal/model
// because they are a query result, not a stored record --
// internal/store/neo4j/types.go makes the same split for the same reason.
package api

import (
	"net/http"
	"time"
	"unicode/utf8"

	"urara-vision/backend/internal/model"
)

// Prose limits, in runes. A domain description is a paragraph of orientation
// and a grain is a single sentence, so these keep the whole catalogue within a
// budget a prompt can carry without losing what either field is saying.
const (
	maxContextDescriptionRunes = 400
	maxContextGrainRunes       = 200
)

type contextResponse struct {
	Snapshot    contextSnapshot `json:"snapshot"`
	Domains     []contextDomain `json:"domains"`
	Tables      []contextTable  `json:"tables"`
	Diagnostics map[string]int  `json:"diagnostics"`
	Truncated   bool            `json:"truncated"`
}

type contextSnapshot struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	CreatedAt time.Time         `json:"createdAt"`
	Project   model.ProjectMeta `json:"project"`
	Stats     model.Stats       `json:"stats"`
}

type contextDomain struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	TableCount  int    `json:"tableCount"`
}

type contextTable struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	DomainID    string          `json:"domainId"`
	Kind        model.TableKind `json:"kind"`
	Grain       string          `json:"grain"`
	ColumnCount int             `json:"columnCount"`
	Conformed   bool            `json:"conformed,omitempty"`
}

// handleContext returns the whole catalogue of a snapshot in one response.
func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	snap, err := s.pg.GetSnapshot(ctx, sid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	domains, err := s.pg.ListDomains(ctx, sid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tables, err := s.pg.ListTables(ctx, sid, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	diags, err := s.pg.ListDiagnostics(ctx, sid, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}

	resp := contextResponse{
		Snapshot: contextSnapshot{
			ID:        snap.ID,
			Name:      snap.Name,
			CreatedAt: snap.CreatedAt,
			Project:   snap.Project,
			Stats:     snap.Stats,
		},
		Domains: make([]contextDomain, 0, len(domains)),
		Tables:  []contextTable{},
		// All three severities are always present, at zero if need be, so a
		// consumer never has to tell an absent key from a count of none.
		Diagnostics: map[string]int{
			model.SeverityError:   0,
			model.SeverityWarning: 0,
			model.SeverityInfo:    0,
		},
	}

	for _, d := range domains {
		resp.Domains = append(resp.Domains, contextDomain{
			ID:          d.ID,
			Title:       d.Title,
			Description: truncateRunes(d.Description, maxContextDescriptionRunes),
			TableCount:  d.TableCount,
		})
	}

	// Past the cap the table list is dropped rather than shortened: the agent
	// reads this as the full catalogue, and a partial one would send it looking
	// for tables it had simply not been shown. Domains stay whole -- they are
	// few, and they are what is left to navigate by.
	if len(tables) > s.cfg.MaxContextTables {
		resp.Truncated = true
	} else {
		resp.Tables = make([]contextTable, 0, len(tables))
		for _, t := range tables {
			resp.Tables = append(resp.Tables, contextTable{
				ID:          t.ID,
				Name:        t.Name,
				DomainID:    t.DomainID,
				Kind:        t.Kind,
				Grain:       truncateRunes(t.Grain, maxContextGrainRunes),
				ColumnCount: t.ColumnCount,
				Conformed:   t.Conformed,
			})
		}
	}

	for _, d := range diags {
		if _, known := resp.Diagnostics[d.Severity]; known {
			resp.Diagnostics[d.Severity]++
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// truncateRunes shortens s to at most n runes, marking that it was cut.
//
// Runes rather than bytes: the documentation this reads is bilingual, and a
// byte slice through a multi-byte character produces mojibake rather than a
// shorter string.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	if n <= 0 {
		return "…"
	}
	return string([]rune(s)[:n]) + "…"
}
