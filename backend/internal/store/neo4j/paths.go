// "How do I join these two tables?" -- the question a model diagram is
// usually consulted for.
package neo4j

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// FindPaths returns the shortest join paths between two tables, which answers
// "how do I join these?" -- the question a model diagram is usually
// consulted for.
func (s *Store) FindPaths(ctx context.Context, sid, from, to string, maxDepth, limit int) ([]JoinPath, error) {
	if maxDepth < 1 || maxDepth > 6 {
		maxDepth = 4
	}
	if limit <= 0 || limit > 25 {
		limit = 10
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() { _ = session.Close(ctx) }()

	q := fmt.Sprintf(`
		MATCH (a:Table {snapshotId: $sid, id: $from}), (b:Table {snapshotId: $sid, id: $to})
		MATCH p = allShortestPaths((a)-[:JOINS*1..%d]-(b))
		RETURN [n IN nodes(p) | n.id] AS tables,
		       [r IN relationships(p) | {
		           from: startNode(r).id, to: endNode(r).id,
		           fromColumn: r.fromColumn, toColumn: r.toColumn, cardinality: r.cardinality
		       }] AS hops,
		       length(p) AS len
		ORDER BY len
		LIMIT $limit`, maxDepth)

	res, err := session.Run(ctx, q, map[string]any{"sid": sid, "from": from, "to": to, "limit": limit})
	if err != nil {
		return nil, fmt.Errorf("find paths: %w", err)
	}

	out := []JoinPath{}
	for res.Next(ctx) {
		r := res.Record()
		jp := JoinPath{Length: integer(r, "len"), Tables: strSlice(r, "tables")}
		if raw, ok := r.Get("hops"); ok {
			if list, ok := raw.([]any); ok {
				for _, item := range list {
					mp, ok := item.(map[string]any)
					if !ok {
						continue
					}
					jp.Hops = append(jp.Hops, PathHop{
						From:        mapStr(mp, "from"),
						To:          mapStr(mp, "to"),
						FromColumn:  mapStr(mp, "fromColumn"),
						ToColumn:    mapStr(mp, "toColumn"),
						Cardinality: mapStr(mp, "cardinality"),
					})
				}
			}
		}
		out = append(out, jp)
	}
	return out, res.Err()
}
