// The focused query: one table and what it touches, within a hop limit.
package neo4j

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Neighborhood returns the subgraph within depth hops of a table, following
// joins in both directions. This is what the UI shows when a table is focused.
func (s *Store) Neighborhood(ctx context.Context, sid, tableID string, depth int, includeSources bool) (*Graph, error) {
	if depth < 1 {
		depth = 1
	}
	if depth > 4 {
		depth = 4
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() { _ = session.Close(ctx) }()

	g := &Graph{Nodes: []Node{}, Links: []Link{}}
	params := map[string]any{"sid": sid, "id": tableID}

	// Variable-length patterns cannot take a parameter for their bound, so the
	// depth is interpolated after being clamped to 1..4 above.
	nodeQ := fmt.Sprintf(`
		MATCH (root:Table {snapshotId: $sid, id: $id})
		OPTIONAL MATCH path = (root)-[:JOINS*1..%d]-(n:Table)
		WITH collect(DISTINCT n) + collect(DISTINCT root) AS ns
		UNWIND ns AS t
		WITH DISTINCT t WHERE t IS NOT NULL
		OPTIONAL MATCH (t)-[j:JOINS]-(:Table)
		RETURN t.id AS id, t.name AS name, t.domainId AS domainId, t.kind AS kind,
		       t.grain AS grain, t.conformed AS conformed, t.columnCount AS columnCount,
		       count(j) AS degree
		ORDER BY id`, depth)

	res, err := session.Run(ctx, nodeQ, params)
	if err != nil {
		return nil, fmt.Errorf("neighborhood nodes: %w", err)
	}
	ids := map[string]bool{}
	for res.Next(ctx) {
		r := res.Record()
		id := str(r, "id")
		ids[id] = true
		g.Nodes = append(g.Nodes, Node{
			ID: id, Label: str(r, "name"), Type: "table",
			DomainID: str(r, "domainId"), Kind: str(r, "kind"), Grain: str(r, "grain"),
			Conformed: boolean(r, "conformed"), ColumnCount: integer(r, "columnCount"),
			Degree: integer(r, "degree"),
		})
	}
	if err := res.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return g, nil
	}

	linkQ := `
		UNWIND $ids AS aid
		MATCH (a:Table {snapshotId: $sid, id: aid})-[j:JOINS]->(b:Table {snapshotId: $sid})
		WHERE b.id IN $ids
		RETURN j.id AS id, a.id AS source, b.id AS target, j.fromColumn AS fromColumn,
		       j.toColumn AS toColumn, j.cardinality AS cardinality,
		       j.resolution AS resolution, j.crossDomain AS crossDomain
		ORDER BY id`
	idList := make([]any, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	res, err = session.Run(ctx, linkQ, map[string]any{"sid": sid, "ids": idList})
	if err != nil {
		return nil, fmt.Errorf("neighborhood links: %w", err)
	}
	for res.Next(ctx) {
		r := res.Record()
		g.Links = append(g.Links, Link{
			ID: str(r, "id"), Source: str(r, "source"), Target: str(r, "target"),
			Type: "joins", FromColumn: str(r, "fromColumn"), ToColumn: str(r, "toColumn"),
			Cardinality: str(r, "cardinality"), Resolution: str(r, "resolution"),
			CrossDomain: boolean(r, "crossDomain"),
		})
	}
	if err := res.Err(); err != nil {
		return nil, err
	}

	if includeSources {
		res, err = session.Run(ctx, `
			MATCH (t:Table {snapshotId: $sid})-[d:DERIVED_FROM]->(s:Source {snapshotId: $sid})
			WHERE t.id IN $ids
			RETURN s.id AS sid_, s.dataset AS dataset, s.name AS name, s.refs AS refs,
			       t.id AS table, d.columns AS columns, d.columnCount AS columnCount`,
			map[string]any{"sid": sid, "ids": idList})
		if err != nil {
			return nil, fmt.Errorf("neighborhood sources: %w", err)
		}
		seen := map[string]bool{}
		for res.Next(ctx) {
			r := res.Record()
			id := str(r, "sid_")
			if !seen[id] {
				seen[id] = true
				g.Nodes = append(g.Nodes, Node{
					ID: id, Label: str(r, "name"), Type: "source",
					Dataset: str(r, "dataset"), Refs: integer(r, "refs"),
				})
			}
			g.Links = append(g.Links, Link{
				ID: str(r, "table") + "->" + id, Source: str(r, "table"), Target: id,
				Type: "derived_from", Columns: strSlice(r, "columns"), ColumnCount: integer(r, "columnCount"),
			})
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
	}

	return g, nil
}
