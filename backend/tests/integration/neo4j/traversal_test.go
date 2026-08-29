//go:build integration

// The traversal queries: one table's neighbourhood at a hop limit, and the join
// paths between two tables.
package neo4j_test

import (
	"testing"

	"urara-vision/backend/tests/integration/harness"
)

// TestNeighborhood is the focused view: one table and what it touches.
func TestNeighborhood(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	// Depth 1 from a dimension reaches the fact that joins it, and no further.
	g, err := gs.Neighborhood(ctx, m.Snapshot.ID, "domain_one/dim_alpha", 1, false)
	if err != nil {
		t.Fatalf("Neighborhood: %v", err)
	}
	ids := nodeIDs(g)
	if _, ok := ids["domain_one/dim_alpha"]; !ok {
		t.Error("the focused table is missing from its own neighbourhood")
	}
	if _, ok := ids["domain_one/fact_primary"]; !ok {
		t.Errorf("the joining fact is missing at depth 1: %v", ids)
	}
	if _, ok := ids["domain_two/dim_beta"]; ok {
		t.Error("dim_beta is two hops away and should not appear at depth 1")
	}

	// Depth 2 reaches the other dimension through the fact.
	g2, err := gs.Neighborhood(ctx, m.Snapshot.ID, "domain_one/dim_alpha", 2, false)
	if err != nil {
		t.Fatalf("Neighborhood(depth 2): %v", err)
	}
	if _, ok := nodeIDs(g2)["domain_two/dim_beta"]; !ok {
		t.Errorf("dim_beta should be reachable at depth 2: %v", nodeIDs(g2))
	}
}

// TestNeighborhoodOfAnUnknownTable returns an empty graph rather than failing,
// since a stale deep link is a normal thing to receive.
func TestNeighborhoodOfAnUnknownTable(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	g, err := gs.Neighborhood(ctx, m.Snapshot.ID, "nope/nope", 1, false)
	if err != nil {
		t.Fatalf("Neighborhood: %v", err)
	}
	if len(g.Nodes) != 0 || len(g.Links) != 0 {
		t.Errorf("graph = %d nodes / %d links, want empty", len(g.Nodes), len(g.Links))
	}
}

// TestFindPaths answers "how do I join these two tables?", the question a star
// schema diagram is usually consulted for.
func TestFindPaths(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	paths, err := gs.FindPaths(ctx, m.Snapshot.ID, "domain_one/dim_alpha", "domain_two/dim_beta", 4, 10)
	if err != nil {
		t.Fatalf("FindPaths: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no path found between two dimensions of the same fact")
	}
	p := paths[0]
	if p.Length != 2 {
		t.Errorf("length = %d, want 2 (through the fact)", p.Length)
	}
	if len(p.Tables) != 3 || p.Tables[1] != "domain_one/fact_primary" {
		t.Errorf("tables = %v, want the fact in the middle", p.Tables)
	}
	if len(p.Hops) != 2 {
		t.Fatalf("hops = %d, want 2", len(p.Hops))
	}
	for _, h := range p.Hops {
		if h.FromColumn == "" || h.ToColumn == "" {
			t.Errorf("hop %s -> %s carries no join columns", h.From, h.To)
		}
	}
}

// TestFindPathsBetweenUnconnectedTables returns nothing rather than erroring.
func TestFindPathsBetweenUnconnectedTables(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	paths, err := gs.FindPaths(ctx, m.Snapshot.ID, "domain_one/dim_alpha", "nope/nope", 4, 10)
	if err != nil {
		t.Fatalf("FindPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("paths = %+v, want none", paths)
	}
}
