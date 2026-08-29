// The overview query: every table that survives the filters, and the joins
// between the survivors.
package neo4j

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// GetGraph returns the join graph for a snapshot.
func (s *Store) GetGraph(ctx context.Context, sid string, opt GraphOptions) (*Graph, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() { _ = session.Close(ctx) }()

	g := &Graph{Nodes: []Node{}, Links: []Link{}}

	params := map[string]any{
		"sid":     sid,
		"domains": toAnySlice(opt.Domains),
		"kinds":   toAnySlice(opt.Kinds),
	}

	// Nodes first, so the frontend can render tables that have no joins.
	nodeQ := `
		MATCH (t:Table {snapshotId: $sid})
		WHERE (size($domains) = 0 OR t.domainId IN $domains)
		  AND (size($kinds) = 0 OR t.kind IN $kinds)
		OPTIONAL MATCH (t)-[j:JOINS]-(:Table)
		RETURN t.id AS id, t.name AS name, t.domainId AS domainId, t.kind AS kind,
		       t.grain AS grain, t.conformed AS conformed, t.columnCount AS columnCount,
		       count(j) AS degree
		ORDER BY id`
	res, err := session.Run(ctx, nodeQ, params)
	if err != nil {
		return nil, fmt.Errorf("graph nodes: %w", err)
	}
	for res.Next(ctx) {
		r := res.Record()
		g.Nodes = append(g.Nodes, Node{
			ID:          str(r, "id"),
			Label:       str(r, "name"),
			Type:        "table",
			DomainID:    str(r, "domainId"),
			Kind:        str(r, "kind"),
			Grain:       str(r, "grain"),
			Conformed:   boolean(r, "conformed"),
			ColumnCount: integer(r, "columnCount"),
			Degree:      integer(r, "degree"),
		})
	}
	if err := res.Err(); err != nil {
		return nil, err
	}

	// Joins between the tables that survived the filter.
	linkQ := `
		MATCH (a:Table {snapshotId: $sid})-[j:JOINS]->(b:Table {snapshotId: $sid})
		WHERE (size($domains) = 0 OR (a.domainId IN $domains AND b.domainId IN $domains))
		  AND (size($kinds) = 0 OR (a.kind IN $kinds AND b.kind IN $kinds))
		  AND ($crossOnly = false OR j.crossDomain = true)
		RETURN j.id AS id, a.id AS source, b.id AS target, j.fromColumn AS fromColumn,
		       j.toColumn AS toColumn, j.cardinality AS cardinality,
		       j.resolution AS resolution, j.crossDomain AS crossDomain
		ORDER BY id`
	params["crossOnly"] = opt.CrossDomainOnly
	res, err = session.Run(ctx, linkQ, params)
	if err != nil {
		return nil, fmt.Errorf("graph links: %w", err)
	}
	for res.Next(ctx) {
		r := res.Record()
		g.Links = append(g.Links, Link{
			ID:          str(r, "id"),
			Source:      str(r, "source"),
			Target:      str(r, "target"),
			Type:        "joins",
			FromColumn:  str(r, "fromColumn"),
			ToColumn:    str(r, "toColumn"),
			Cardinality: str(r, "cardinality"),
			Resolution:  str(r, "resolution"),
			CrossDomain: boolean(r, "crossDomain"),
		})
	}
	if err := res.Err(); err != nil {
		return nil, err
	}

	if opt.IncludeSources {
		srcQ := `
			MATCH (t:Table {snapshotId: $sid})-[d:DERIVED_FROM]->(s:Source {snapshotId: $sid})
			WHERE (size($domains) = 0 OR t.domainId IN $domains)
			  AND (size($kinds) = 0 OR t.kind IN $kinds)
			RETURN s.id AS sid_, s.dataset AS dataset, s.name AS name, s.refs AS refs,
			       t.id AS table, d.columns AS columns, d.columnCount AS columnCount
			ORDER BY sid_, table`
		res, err = session.Run(ctx, srcQ, params)
		if err != nil {
			return nil, fmt.Errorf("graph sources: %w", err)
		}
		seen := map[string]bool{}
		for res.Next(ctx) {
			r := res.Record()
			id := str(r, "sid_")
			if !seen[id] {
				seen[id] = true
				g.Nodes = append(g.Nodes, Node{
					ID:      id,
					Label:   str(r, "name"),
					Type:    "source",
					Dataset: str(r, "dataset"),
					Refs:    integer(r, "refs"),
				})
			}
			g.Links = append(g.Links, Link{
				ID:          str(r, "table") + "->" + id,
				Source:      str(r, "table"),
				Target:      id,
				Type:        "derived_from",
				Columns:     strSlice(r, "columns"),
				ColumnCount: integer(r, "columnCount"),
			})
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
	}

	return g, nil
}
