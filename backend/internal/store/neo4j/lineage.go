// Lineage queries over the DERIVED_FROM edges: what a table draws from, what
// reads a source model, and which tables share a source.
package neo4j

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Upstream returns the source models a table draws from.
func (s *Store) Upstream(ctx context.Context, sid, tableID string) ([]LineageEntry, error) {
	return s.lineage(ctx, `
		MATCH (t:Table {snapshotId: $sid, id: $id})-[d:DERIVED_FROM]->(s:Source)
		RETURN s.id AS id, s.name AS label, s.dataset AS dataset, '' AS domainId,
		       d.columns AS columns, d.columnCount AS columnCount
		ORDER BY columnCount DESC, id`, sid, tableID)
}

// Downstream returns the tables fed by a source model. Passing a table ID
// instead returns the tables that share a source with it.
func (s *Store) Downstream(ctx context.Context, sid, sourceID string) ([]LineageEntry, error) {
	return s.lineage(ctx, `
		MATCH (t:Table {snapshotId: $sid})-[d:DERIVED_FROM]->(s:Source {snapshotId: $sid, id: $id})
		RETURN t.id AS id, t.name AS label, '' AS dataset, t.domainId AS domainId,
		       d.columns AS columns, d.columnCount AS columnCount
		ORDER BY columnCount DESC, id`, sid, sourceID)
}

// SiblingsBySource returns tables that read from at least one of the same
// source models as the given table -- a practical proxy for "what else breaks
// if this upstream model changes".
func (s *Store) SiblingsBySource(ctx context.Context, sid, tableID string) ([]LineageEntry, error) {
	return s.lineage(ctx, `
		MATCH (t:Table {snapshotId: $sid, id: $id})-[:DERIVED_FROM]->(s:Source)<-[d:DERIVED_FROM]-(o:Table)
		WHERE o.id <> t.id
		WITH o, collect(DISTINCT s.id) AS shared, sum(d.columnCount) AS cc
		RETURN o.id AS id, o.name AS label, '' AS dataset, o.domainId AS domainId,
		       shared AS columns, cc AS columnCount
		ORDER BY size(shared) DESC, id`, sid, tableID)
}

func (s *Store) lineage(ctx context.Context, cypher, sid, id string) ([]LineageEntry, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() { _ = session.Close(ctx) }()

	res, err := session.Run(ctx, cypher, map[string]any{"sid": sid, "id": id})
	if err != nil {
		return nil, fmt.Errorf("lineage query: %w", err)
	}
	out := []LineageEntry{}
	for res.Next(ctx) {
		r := res.Record()
		out = append(out, LineageEntry{
			ID:          str(r, "id"),
			Label:       str(r, "label"),
			Dataset:     str(r, "dataset"),
			DomainID:    str(r, "domainId"),
			Columns:     strSlice(r, "columns"),
			ColumnCount: integer(r, "columnCount"),
		})
	}
	return out, res.Err()
}
