// Turning resolved relationships into the drawable edge list: deduplication,
// direction, and stable identity across rebuilds.
package graph_test

import (
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

func TestEdgesDedupeOppositeDeclarations(t *testing.T) {
	// The fact declares Many-to-one and the dimension declares One-to-many for
	// the same join; they must collapse into a single edge.
	m := fixtures.Build([]parser.File{
		{Path: "r/fact_primary.md", Content: fixtures.Doc("fact_primary", "Fact", "R",
			[]string{"primary_id", "alpha_id"}, "| `dim_alpha` | `alpha_id = alpha_id` | Many-to-one |\n")},
		{Path: "r/dim_alpha.md", Content: fixtures.Doc("dim_alpha", "Dimension", "R",
			[]string{"alpha_id"}, "| `fact_primary` | `alpha_id = alpha_id` | One-to-many |\n")},
	})
	edges := graph.Edges(m)
	if len(edges) != 1 {
		t.Fatalf("edges = %d, want 1: %+v", len(edges), edges)
	}
	e := edges[0]
	if e.From != "r/fact_primary" || e.To != "r/dim_alpha" {
		t.Errorf("edge direction = %s -> %s, want fact -> dim", e.From, e.To)
	}
	if e.Cardinality != "Many-to-one" {
		t.Errorf("cardinality = %q", e.Cardinality)
	}
	if len(e.DeclaredBy) != 2 {
		t.Errorf("declaredBy = %v, want both tables", e.DeclaredBy)
	}
}

func TestOneToManyFlipsColumnsWithDirection(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "g/dim_gamma.md", Content: fixtures.Doc("dim_gamma", "Dimension", "G",
			[]string{"gamma_id"}, "| `dim_beta` | `gamma_id = beta_gamma_id` | One-to-many |\n")},
		{Path: "g/dim_beta.md", Content: fixtures.Doc("dim_beta", "Dimension", "G", []string{"beta_gamma_id"}, "")},
	})
	edges := graph.Edges(m)
	if len(edges) != 1 {
		t.Fatalf("edges = %d", len(edges))
	}
	e := edges[0]
	if e.From != "g/dim_beta" || e.To != "g/dim_gamma" {
		t.Fatalf("direction = %s -> %s, want dim_beta -> dim_gamma", e.From, e.To)
	}
	// After reversal the many side's column must lead.
	if e.FromColumn != "beta_gamma_id" || e.ToColumn != "gamma_id" {
		t.Errorf("columns = %s -> %s, want beta_gamma_id -> gamma_id", e.FromColumn, e.ToColumn)
	}
}

func TestEdgeIDsAreStableAcrossBuilds(t *testing.T) {
	files := []parser.File{
		{Path: "r/fact_primary.md", Content: fixtures.Doc("fact_primary", "Fact", "R",
			[]string{"primary_id", "alpha_id"}, "| `dim_alpha` | `alpha_id = alpha_id` | Many-to-one |\n")},
		{Path: "r/dim_alpha.md", Content: fixtures.Doc("dim_alpha", "Dimension", "R", []string{"alpha_id"}, "")},
	}
	a, b := graph.Edges(fixtures.Build(files)), graph.Edges(fixtures.Build(files))
	if len(a) != len(b) || a[0].ID != b[0].ID {
		t.Errorf("edge IDs unstable: %v vs %v", a, b)
	}
}

// TestJoinKeyOrientationAgainstAuthoringOrder covers the real-world case where
// a dimension writes its join key fact-column-first on its own One-to-many row.
// Trusting the written order reverses the columns; matching them against the
// tables' real column lists keeps the join correct and lets both declarations
// collapse into one edge.
func TestJoinKeyOrientationAgainstAuthoringOrder(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "r/fact_primary.md", Content: fixtures.Doc("fact_primary", "Fact", "R",
			[]string{"primary_id", "beta_id_1", "beta_id_2"},
			"| `dim_beta` | `beta_id_1 = beta_id` | Many-to-one |\n"+
				"| `dim_beta` | `beta_id_2 = beta_id` | Many-to-one |\n")},
		// dim_beta names the fact's column first even though it declares the
		// relationship from its own side.
		{Path: "r/dim_beta.md", Content: fixtures.Doc("dim_beta", "Dimension", "R",
			[]string{"beta_id", "beta_name"},
			"| `fact_primary` | `beta_id_1 = beta_id` | One-to-many |\n"+
				"| `fact_primary` | `beta_id_2 = beta_id` | One-to-many |\n")},
	})

	// dim_beta's own relationships must name its own column first.
	for _, tb := range m.Tables {
		if tb.ID != "r/dim_beta" {
			continue
		}
		for _, r := range tb.Relationships {
			if r.FromColumn != "beta_id" {
				t.Errorf("dim_beta relationship FromColumn = %q, want beta_id (its own column)", r.FromColumn)
			}
			if r.ToColumn != "beta_id_1" && r.ToColumn != "beta_id_2" {
				t.Errorf("dim_beta relationship ToColumn = %q, want a fact column", r.ToColumn)
			}
		}
	}

	// The two sides describe the same two joins, so exactly two edges survive:
	// one per key role, not four.
	edges := graph.Edges(m)
	if len(edges) != 2 {
		t.Fatalf("edges = %d, want 2 (one per key role); got %+v", len(edges), edges)
	}
	roles := map[string]string{}
	for _, e := range edges {
		if e.From != "r/fact_primary" || e.To != "r/dim_beta" {
			t.Errorf("edge direction = %s -> %s, want fact -> dim", e.From, e.To)
		}
		if e.ToColumn != "beta_id" {
			t.Errorf("edge ToColumn = %q, want beta_id", e.ToColumn)
		}
		roles[e.FromColumn] = e.ToColumn
		if len(e.DeclaredBy) != 2 {
			t.Errorf("edge %s declaredBy = %v, want both tables", e.FromColumn, e.DeclaredBy)
		}
	}
	if _, ok := roles["beta_id_1"]; !ok {
		t.Error("missing the beta_id_1 role edge")
	}
	if _, ok := roles["beta_id_2"]; !ok {
		t.Error("missing the beta_id_2 role edge")
	}
}

func TestUnmatchedJoinKeyIsReported(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "a/fact_x.md", Content: fixtures.Doc("fact_x", "Fact", "A", []string{"id"},
			"| `dim_y` | `nonexistent_a = nonexistent_b` | Many-to-one |\n")},
		{Path: "a/dim_y.md", Content: fixtures.Doc("dim_y", "Dimension", "A", []string{"y_id"}, "")},
	})
	var found bool
	for _, d := range m.Diagnostics {
		if d.Code == "unmatched_join_key" {
			found = true
		}
	}
	if !found {
		t.Error("expected an unmatched_join_key diagnostic")
	}
}
