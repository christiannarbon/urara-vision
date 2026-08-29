//go:build integration

// The overview query's filters. A filter that keeps a link whose endpoint it
// dropped is a rendering error, so the node and link sets are checked together.
package neo4j_test

import (
	"testing"

	neostore "urara-vision/backend/internal/store/neo4j"
	"urara-vision/backend/tests/integration/harness"
)

func TestGraphDomainFilter(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	g, err := gs.GetGraph(ctx, m.Snapshot.ID, neostore.GraphOptions{Domains: []string{"domain_one"}})
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Errorf("nodes = %d, want the 2 domain_one tables: %v", len(g.Nodes), nodeIDs(g))
	}
	for _, n := range g.Nodes {
		if n.DomainID != "domain_one" {
			t.Errorf("node %s is in domain %q", n.ID, n.DomainID)
		}
	}
	// The join to domain_two/dim_beta has one end outside the filter, so it must not
	// be drawn -- a link to a node that is not there is a rendering error.
	ids := nodeIDs(g)
	for _, l := range g.Links {
		if _, ok := ids[l.Source]; !ok {
			t.Errorf("link %s starts at %s, which is not in the filtered node set", l.ID, l.Source)
		}
		if _, ok := ids[l.Target]; !ok {
			t.Errorf("link %s ends at %s, which is not in the filtered node set", l.ID, l.Target)
		}
	}
}

func TestGraphKindFilter(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	g, err := gs.GetGraph(ctx, m.Snapshot.ID, neostore.GraphOptions{Kinds: []string{"fact"}})
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	if len(g.Nodes) != 1 {
		t.Errorf("nodes = %d, want the single fact: %v", len(g.Nodes), nodeIDs(g))
	}
	if len(g.Links) != 0 {
		t.Errorf("links = %d; a fact-only view has no dimension to join to", len(g.Links))
	}
}

// TestGraphCrossDomainOnly is the "what depends on another domain?" view.
func TestGraphCrossDomainOnly(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	g, err := gs.GetGraph(ctx, m.Snapshot.ID, neostore.GraphOptions{CrossDomainOnly: true})
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	if len(g.Links) != 1 {
		t.Fatalf("links = %d, want the single domain_one -> domain_two join: %+v", len(g.Links), g.Links)
	}
	l := g.Links[0]
	if !l.CrossDomain {
		t.Error("the surviving link is not flagged crossDomain")
	}
	if l.Source != "domain_one/fact_primary" || l.Target != "domain_two/dim_beta" {
		t.Errorf("link = %s -> %s, want domain_one/fact_primary -> domain_two/dim_beta", l.Source, l.Target)
	}
}

// TestGraphIncludeSources adds the upstream models and their DERIVED_FROM
// edges, which is a second node type in the same projection.
func TestGraphIncludeSources(t *testing.T) {
	ctx := harness.Context(t)
	gs := harness.Neo4j(t)
	m, _ := harness.ProjectedModel(t, ctx, gs)

	without, err := gs.GetGraph(ctx, m.Snapshot.ID, neostore.GraphOptions{})
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	with, err := gs.GetGraph(ctx, m.Snapshot.ID, neostore.GraphOptions{IncludeSources: true})
	if err != nil {
		t.Fatalf("GetGraph(sources): %v", err)
	}
	if len(with.Nodes) != len(without.Nodes)+1 {
		t.Errorf("nodes = %d with sources and %d without, want one more", len(with.Nodes), len(without.Nodes))
	}

	var src *neostore.Node
	for i := range with.Nodes {
		if with.Nodes[i].Type == "source" {
			src = &with.Nodes[i]
		}
	}
	if src == nil {
		t.Fatal("no source node was returned")
	}
	if src.ID != "warehouse.upstream_model" || src.Dataset != "warehouse" {
		t.Errorf("source node = %+v", src)
	}

	var derived int
	for _, l := range with.Links {
		if l.Type == "derived_from" {
			derived++
			if len(l.Columns) == 0 {
				t.Errorf("lineage link %s carries no columns", l.ID)
			}
		}
	}
	if derived != 1 {
		t.Errorf("derived_from links = %d, want 1", derived)
	}
}
