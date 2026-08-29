// Full-text search over tables and their columns.
package postgres

import (
	"context"
	_ "embed"
	"strings"

	"urara-vision/backend/internal/model"
)

// SearchHit is one full-text search result.
type SearchHit struct {
	TableID   string          `json:"tableId"`
	Name      string          `json:"name"`
	DomainID  string          `json:"domainId"`
	Kind      model.TableKind `json:"kind"`
	Grain     string          `json:"grain"`
	Rank      float32         `json:"rank"`
	MatchedOn []string        `json:"matchedOn,omitempty"`
}

// Search runs a full-text query over tables and their columns. The query is
// treated as a prefix search so partial table names match as the user types.
func (s *Store) Search(ctx context.Context, sid, query string, limit int) ([]SearchHit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []SearchHit{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	tsq := toPrefixQuery(q)

	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.name, t.domain_id, t.kind, t.grain,
		       ts_rank(t.search, to_tsquery('english', $2)) AS rank,
		       coalesce((
		           SELECT array_agg(DISTINCT c.name)
		           FROM columns c
		           WHERE c.snapshot_id = t.snapshot_id AND c.table_id = t.id
		             AND c.name ILIKE '%' || $3 || '%'
		       ), '{}') AS matched
		FROM tables t
		WHERE t.snapshot_id = $1 AND t.search @@ to_tsquery('english', $2)
		ORDER BY rank DESC, t.name
		LIMIT $4`, sid, tsq, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SearchHit{}
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.TableID, &h.Name, &h.DomainID, &h.Kind, &h.Grain, &h.Rank, &h.MatchedOn); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// toPrefixQuery converts free text into a safe tsquery where each term is a
// prefix match, so "alph ident" matches "alpha_identifier".
func toPrefixQuery(q string) string {
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_')
	})
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		if f == "" {
			continue
		}
		terms = append(terms, f+":*")
	}
	if len(terms) == 0 {
		// No usable characters; match nothing rather than erroring.
		return "zzzznomatchzzzz"
	}
	return strings.Join(terms, " & ")
}
