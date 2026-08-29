//go:build integration

// Integration tests for the Neo4j projection: writing a snapshot's graph and
// removing it again.
//
// Run with `make test-integration`; without a URI they skip.
package neo4j_test

import (
	"testing"

	neostore "urara-vision/backend/internal/store/neo4j"
	"urara-vision/backend/tests/integration/harness"
)

// nodeIDs collects a graph's node IDs for set-style assertions.
func nodeIDs(g *neostore.Graph) map[string]neostore.Node {
	m := map[string]neostore.Node{}
	for _, n := range g.Nodes {
		m[n.ID] = n
	}
	return m
}

func TestEnsureConstraintsIsIdempotent(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t) // constraints already ensured once
	if err := gs.EnsureConstraints(ctx); err != nil {
		t.Fatalf("second EnsureConstraints: %v", err)
	}
}

// TestProjectAndReadBackGraph: every table becomes a node and every deduped
// edge becomes a JOINS relationship.
func TestProjectAndReadBackGraph(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, edges := harness.ProjectedModel(t, ctx, gs)

	g, err := gs.GetGraph(ctx, m.Snapshot.ID, neostore.GraphOptions{})
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	if len(g.Nodes) != len(m.Tables) {
		t.Errorf("nodes = %d, want %d", len(g.Nodes), len(m.Tables))
	}
	if len(g.Links) != len(edges) {
		t.Errorf("links = %d, want %d", len(g.Links), len(edges))
	}

	byID := nodeIDs(g)
	fact, ok := byID["domain_one/fact_primary"]
	if !ok {
		t.Fatalf("fact_primary missing from %v", byID)
	}
	if fact.Type != "table" || fact.Kind != "fact" {
		t.Errorf("node = %+v, want a fact table", fact)
	}
	if fact.ColumnCount != 3 {
		t.Errorf("columnCount = %d, want 3", fact.ColumnCount)
	}
	// The fact joins two dimensions, so its degree is 2. Degree is what sizes
	// the node in the UI, and a wrong count is visible.
	if fact.Degree != 2 {
		t.Errorf("degree = %d, want 2", fact.Degree)
	}
	if dim := byID["domain_two/dim_beta"]; !dim.Conformed {
		t.Error("dim_beta was projected unconformed")
	}

	for _, l := range g.Links {
		if l.Type != "joins" {
			t.Errorf("link %s has type %q", l.ID, l.Type)
		}
		if l.FromColumn == "" || l.ToColumn == "" {
			t.Errorf("link %s lost its join columns", l.ID)
		}
	}
}

// TestProjectReplacesTheSameSnapshot: re-ingesting must not double the graph.
func TestProjectReplacesTheSameSnapshot(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, edges := harness.ProjectedModel(t, ctx, gs)

	if err := gs.Project(ctx, m, edges); err != nil {
		t.Fatalf("second Project: %v", err)
	}
	g, err := gs.GetGraph(ctx, m.Snapshot.ID, neostore.GraphOptions{})
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	if len(g.Nodes) != len(m.Tables) {
		t.Errorf("nodes = %d after reprojection, want %d", len(g.Nodes), len(m.Tables))
	}
	if len(g.Links) != len(edges) {
		t.Errorf("links = %d after reprojection, want %d", len(g.Links), len(edges))
	}
}

// TestGraphIsScopedToItsSnapshot: two projections of the same model must not
// see each other's nodes, which is what makes snapshot comparison possible.
func TestGraphIsScopedToItsSnapshot(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	a, _ := harness.ProjectedModel(t, ctx, gs)
	b, _ := harness.ProjectedModel(t, ctx, gs)

	ga, err := gs.GetGraph(ctx, a.Snapshot.ID, neostore.GraphOptions{})
	if err != nil {
		t.Fatalf("GetGraph(a): %v", err)
	}
	gb, err := gs.GetGraph(ctx, b.Snapshot.ID, neostore.GraphOptions{})
	if err != nil {
		t.Fatalf("GetGraph(b): %v", err)
	}
	if len(ga.Nodes) != len(gb.Nodes) || len(ga.Nodes) != len(a.Tables) {
		t.Errorf("nodes = %d and %d, want %d each; the snapshots are not isolated",
			len(ga.Nodes), len(gb.Nodes), len(a.Tables))
	}
}

// TestDeleteSnapshotRemovesTheWholeProjection, including the source nodes,
// which hang off the snapshot rather than off any one table.
func TestDeleteSnapshotRemovesTheWholeProjection(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	if err := gs.DeleteSnapshot(ctx, m.Snapshot.ID); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	g, err := gs.GetGraph(ctx, m.Snapshot.ID, neostore.GraphOptions{IncludeSources: true})
	if err != nil {
		t.Fatalf("GetGraph after delete: %v", err)
	}
	if len(g.Nodes) != 0 || len(g.Links) != 0 {
		t.Errorf("graph = %d nodes / %d links after delete, want empty", len(g.Nodes), len(g.Links))
	}

	// Deleting an absent projection is a no-op, so the rollback path in ingest
	// can call it unconditionally.
	if err := gs.DeleteSnapshot(ctx, m.Snapshot.ID); err != nil {
		t.Errorf("second DeleteSnapshot = %v, want nil", err)
	}
}
