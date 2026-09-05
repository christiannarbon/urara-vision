// Domain and table endpoints, from the lists the sidebar renders to the full
// detail one pane shows.
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"urara-vision/backend/internal/store/postgres"
)

// maxBatchTables caps how many tables one /tables/detail call may ask for. A
// tool loop batches the handful it is reasoning about; a larger request is a
// sign of a caller that should be paging the table list instead.
const maxBatchTables = 8

func (s *Server) handleListDomains(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	domains, err := s.pg.ListDomains(r.Context(), sid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": domains})
}

func (s *Server) handleListTables(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	tables, err := s.pg.ListTables(r.Context(), sid, r.URL.Query().Get("domain"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
}

// handleGetTable returns one table's full detail. Table IDs contain a slash
// ("domain_one/fact_primary"), so they travel as a query parameter rather than a path
// segment.
func (s *Server) handleGetTable(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		s.badRequest(w, "query parameter \"id\" is required")
		return
	}

	detail, err := s.tableDetail(r.Context(), sid, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// tableDetail assembles one table's full response: the document itself, the
// relationships pointing at it, and its lineage. Both the single and batch
// handlers go through here so there is one definition of what a table detail
// is.
func (s *Server) tableDetail(ctx context.Context, sid, id string) (any, error) {
	table, err := s.pg.GetTable(ctx, sid, id)
	if err != nil {
		return nil, err
	}
	incoming, err := s.pg.IncomingRelationships(ctx, sid, id)
	if err != nil {
		return nil, err
	}
	upstream, err := s.graphs.Upstream(ctx, sid, id)
	if err != nil {
		return nil, err
	}
	siblings, err := s.graphs.SiblingsBySource(ctx, sid, id)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"table":    table,
		"incoming": incoming,
		"upstream": upstream,
		"siblings": siblings,
	}, nil
}

// handleTablesDetail returns several table details in one call. In a tool loop
// four tables would otherwise cost four round trips and four model turns; each
// entry here is byte-identical to what /table?id= returns for the same ID.
func (s *Server) handleTablesDetail(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.resolveSnapshot(w, r)
	if !ok {
		return
	}
	ids := splitCSV(r.URL.Query().Get("ids"))
	if len(ids) == 0 {
		s.badRequest(w, "query parameter \"ids\" is required")
		return
	}
	if len(ids) > maxBatchTables {
		s.badRequest(w, fmt.Sprintf("at most %d ids may be requested at once, got %d", maxBatchTables, len(ids)))
		return
	}

	// Both are initialised rather than left nil so the JSON carries [] and not
	// null, which a caller would have to guard separately.
	tables := []any{}
	missing := []string{}

	// Sequentially: these are eight primary-key lookups, and a concurrent
	// version would need a bounded pool and error aggregation to buy a saving
	// too small to measure.
	for _, id := range ids {
		detail, err := s.tableDetail(r.Context(), sid, id)
		if err != nil {
			// A wrong guess at an ID is an answer, not a failure: the agent
			// gets the tables that exist plus the list that did not, rather
			// than a failed call it has to reason its way out of.
			if errors.Is(err, postgres.ErrNotFound) {
				missing = append(missing, id)
				continue
			}
			s.fail(w, r, err)
			return
		}
		tables = append(tables, detail)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tables":  tables,
		"missing": missing,
	})
}
