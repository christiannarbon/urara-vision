// Writing a snapshot's graph.
//
// A projection replaces whatever was stored under the same snapshot ID, so a
// re-ingest is idempotent rather than additive.
package neo4j

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
)

// batchSize keeps UNWIND payloads small enough to stream comfortably.
const batchSize = 500

// Project writes a snapshot's graph, replacing anything previously stored under
// the same snapshot ID.
func (s *Store) Project(ctx context.Context, m *model.Model, edges []graph.Edge) error {
	sid := m.Snapshot.ID
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() { _ = session.Close(ctx) }()

	if err := s.DeleteSnapshot(ctx, sid); err != nil {
		return err
	}

	// Domains.
	domains := make([]map[string]any, 0, len(m.Domains))
	for _, d := range m.Domains {
		domains = append(domains, map[string]any{
			"id": d.ID, "title": d.Title, "description": d.Description, "tableCount": d.TableCount,
		})
	}
	if err := runBatched(ctx, session, `
		UNWIND $rows AS row
		MERGE (d:Domain {snapshotId: $sid, id: row.id})
		SET d.title = row.title, d.description = row.description, d.tableCount = row.tableCount`,
		sid, domains); err != nil {
		return fmt.Errorf("project domains: %w", err)
	}

	// Tables, linked to their domain.
	tables := make([]map[string]any, 0, len(m.Tables))
	for _, t := range m.Tables {
		tables = append(tables, map[string]any{
			"id": t.ID, "name": t.Name, "domainId": t.DomainID, "kind": string(t.Kind),
			"grain": t.Grain, "conformed": t.Conformed, "columnCount": len(t.Columns),
		})
	}
	if err := runBatched(ctx, session, `
		UNWIND $rows AS row
		MERGE (t:Table {snapshotId: $sid, id: row.id})
		SET t.name = row.name, t.domainId = row.domainId, t.kind = row.kind,
		    t.grain = row.grain, t.conformed = row.conformed, t.columnCount = row.columnCount
		WITH t, row
		MATCH (d:Domain {snapshotId: $sid, id: row.domainId})
		MERGE (t)-[:IN_DOMAIN]->(d)`,
		sid, tables); err != nil {
		return fmt.Errorf("project tables: %w", err)
	}

	// Joins.
	joins := make([]map[string]any, 0, len(edges))
	for _, e := range edges {
		joins = append(joins, map[string]any{
			"id": e.ID, "from": e.From, "to": e.To,
			"fromColumn": e.FromColumn, "toColumn": e.ToColumn,
			"cardinality": e.Cardinality, "resolution": string(e.Resolution),
			"crossDomain": e.CrossDomain, "declaredBy": toAnySlice(e.DeclaredBy),
		})
	}
	if err := runBatched(ctx, session, `
		UNWIND $rows AS row
		MATCH (a:Table {snapshotId: $sid, id: row.from})
		MATCH (b:Table {snapshotId: $sid, id: row.to})
		MERGE (a)-[j:JOINS {id: row.id}]->(b)
		SET j.fromColumn = row.fromColumn, j.toColumn = row.toColumn,
		    j.cardinality = row.cardinality, j.resolution = row.resolution,
		    j.crossDomain = row.crossDomain, j.declaredBy = row.declaredBy`,
		sid, joins); err != nil {
		return fmt.Errorf("project joins: %w", err)
	}

	// Conformed instances of the same table name link to each other's group.
	conformed := map[string][]string{}
	for _, t := range m.Tables {
		if t.Conformed {
			conformed[t.Name] = append(conformed[t.Name], t.ID)
		}
	}
	groups := make([]map[string]any, 0, len(conformed))
	for name, ids := range conformed {
		groups = append(groups, map[string]any{"name": name, "ids": toAnySlice(ids)})
	}
	if err := runBatched(ctx, session, `
		UNWIND $rows AS row
		MERGE (g:Conformed {snapshotId: $sid, name: row.name})
		WITH g, row
		UNWIND row.ids AS tid
		MATCH (t:Table {snapshotId: $sid, id: tid})
		MERGE (t)-[:CONFORMS_TO]->(g)`,
		sid, groups); err != nil {
		return fmt.Errorf("project conformed groups: %w", err)
	}

	// Source tables and the columns derived from them.
	sources := make([]map[string]any, 0, len(m.SourceTables))
	for _, st := range m.SourceTables {
		sources = append(sources, map[string]any{
			"id": st.ID, "dataset": st.Dataset, "name": st.Name, "refs": st.Refs,
		})
	}
	if err := runBatched(ctx, session, `
		UNWIND $rows AS row
		MERGE (s:Source {snapshotId: $sid, id: row.id})
		SET s.dataset = row.dataset, s.name = row.name, s.refs = row.refs`,
		sid, sources); err != nil {
		return fmt.Errorf("project sources: %w", err)
	}

	// One DERIVED_FROM edge per (table, source), carrying the columns it feeds.
	type dfKey struct{ table, source string }
	df := map[dfKey][]string{}
	for _, t := range m.Tables {
		for _, l := range t.ColumnLineage {
			if l.SourceTable == "" {
				continue
			}
			k := dfKey{t.ID, l.SourceTable}
			df[k] = append(df[k], l.Column)
		}
	}
	derived := make([]map[string]any, 0, len(df))
	for k, cols := range df {
		derived = append(derived, map[string]any{
			"table": k.table, "source": k.source,
			"columns": toAnySlice(cols), "columnCount": len(cols),
		})
	}
	if err := runBatched(ctx, session, `
		UNWIND $rows AS row
		MATCH (t:Table {snapshotId: $sid, id: row.table})
		MATCH (s:Source {snapshotId: $sid, id: row.source})
		MERGE (t)-[d:DERIVED_FROM]->(s)
		SET d.columns = row.columns, d.columnCount = row.columnCount`,
		sid, derived); err != nil {
		return fmt.Errorf("project lineage: %w", err)
	}

	return nil
}

// runBatched executes a Cypher statement over rows in chunks.
func runBatched(ctx context.Context, session neo4j.SessionWithContext, cypher, sid string, rows []map[string]any) error {
	if len(rows) == 0 {
		return nil
	}
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			return tx.Run(ctx, cypher, map[string]any{"sid": sid, "rows": chunk})
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// DeleteSnapshot removes every node belonging to a snapshot.
func (s *Store) DeleteSnapshot(ctx context.Context, sid string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() { _ = session.Close(ctx) }()

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		return tx.Run(ctx, `
			MATCH (n)
			WHERE n.snapshotId = $sid AND (n:Table OR n:Domain OR n:Source OR n:Conformed)
			DETACH DELETE n`, map[string]any{"sid": sid})
	})
	if err != nil {
		return fmt.Errorf("delete snapshot from neo4j: %w", err)
	}
	return nil
}
