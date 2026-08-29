//go:build integration

// Lineage over the DERIVED_FROM edges: what a table draws from, what reads a
// source model, and which tables share one.
package neo4j_test

import (
	"testing"

	"urara-vision/backend/tests/integration/harness"
)

func TestUpstreamAndDownstreamLineage(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	up, err := gs.Upstream(ctx, m.Snapshot.ID, "domain_one/fact_primary")
	if err != nil {
		t.Fatalf("Upstream: %v", err)
	}
	if len(up) != 1 || up[0].ID != "warehouse.upstream_model" {
		t.Fatalf("upstream = %+v, want the one source model", up)
	}
	if len(up[0].Columns) == 0 || up[0].ColumnCount == 0 {
		t.Errorf("upstream entry carries no columns: %+v", up[0])
	}

	down, err := gs.Downstream(ctx, m.Snapshot.ID, "warehouse.upstream_model")
	if err != nil {
		t.Fatalf("Downstream: %v", err)
	}
	if len(down) != 1 || down[0].ID != "domain_one/fact_primary" {
		t.Errorf("downstream = %+v, want fact_primary", down)
	}
}

// TestUpstreamOfATableWithoutLineage is empty, not an error: most dimensions
// document no column lineage.
func TestUpstreamOfATableWithoutLineage(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	up, err := gs.Upstream(ctx, m.Snapshot.ID, "domain_one/dim_alpha")
	if err != nil {
		t.Fatalf("Upstream: %v", err)
	}
	if len(up) != 0 {
		t.Errorf("upstream = %+v, want none", up)
	}
}

// TestSiblingsBySource answers "what else reads this model?" -- the question
// that source canonicalisation exists to make answerable.
func TestSiblingsBySource(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	// fact_primary is the only table reading its source, so it has no siblings,
	// and it must not be returned as its own sibling.
	sibs, err := gs.SiblingsBySource(ctx, m.Snapshot.ID, "domain_one/fact_primary")
	if err != nil {
		t.Fatalf("SiblingsBySource: %v", err)
	}
	for _, s := range sibs {
		if s.ID == "domain_one/fact_primary" {
			t.Error("a table was returned as its own sibling")
		}
	}
}
