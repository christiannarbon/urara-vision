// Upstream source models, read from column-level lineage: which references are
// modelled at all, and folding one model's several spellings into one node.
package graph_test

import (
	"testing"

	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

// TestProseSourcesAreNotModelledAsTables covers the placeholders that appear
// in real documentation: "not available" was the second most-referenced
// "source model" before these were filtered out.
func TestProseSourcesAreNotModelledAsTables(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "a/fact_x.md", Content: fixtures.LineageDoc("fact_x", "Fact", []string{"a", "b", "c", "d"},
			"| `a` | `ds.real_model` | `a` | |\n"+
				"| `b` | `not available` | | |\n"+
				"| `c` | `N/A` | | |\n"+
				"| `d` | `GA event models` | | |\n")},
	})
	if len(m.SourceTables) != 1 || m.SourceTables[0].ID != "ds.real_model" {
		t.Fatalf("source tables = %+v, want only ds.real_model", m.SourceTables)
	}
	if m.Snapshot.Stats.LineageEdges != 1 {
		t.Errorf("lineage edges = %d, want 1", m.Snapshot.Stats.LineageEdges)
	}
	var found bool
	for _, d := range m.Diagnostics {
		if d.Code == "undocumented_lineage" && d.TableID == "a/fact_x" {
			found = true
		}
	}
	if !found {
		t.Error("expected an undocumented_lineage diagnostic")
	}
}

// TestSourceIdentityIsCanonicalised covers the same upstream model being cited
// with and without its dataset. Left unfolded it becomes two lineage nodes and
// "what else reads this?" silently returns the wrong answer.
func TestSourceIdentityIsCanonicalised(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "a/fact_x.md", Content: fixtures.LineageDoc("fact_x", "Fact", []string{"a"},
			"| `a` | `warehouse.upstream_model` | `a` | |\n")},
		{Path: "b/dim_y.md", Content: fixtures.LineageDoc("dim_y", "Dimension", []string{"b"},
			"| `b` | `upstream_model` | `b` | |\n")},
	})
	if len(m.SourceTables) != 1 {
		t.Fatalf("source tables = %+v, want a single canonical entry", m.SourceTables)
	}
	src := m.SourceTables[0]
	if src.ID != "warehouse.upstream_model" || src.Refs != 2 {
		t.Errorf("source = %+v, want warehouse.upstream_model with 2 refs", src)
	}
	// The rewrite must reach the stored lineage too, or the detail pane and the
	// graph disagree about what a column came from.
	for _, tb := range m.Tables {
		if got := tb.ColumnLineage[0].SourceTable; got != "warehouse.upstream_model" {
			t.Errorf("%s lineage source = %q, want canonical form", tb.ID, got)
		}
	}
}

// TestAmbiguousBareNameIsLeftAlone: if a bare name maps to two datasets there
// is no non-guessing answer, so it must stay as written.
func TestAmbiguousBareNameIsLeftAlone(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "a/fact_x.md", Content: fixtures.LineageDoc("fact_x", "Fact", []string{"a"},
			"| `a` | `ds_one.shared` | `a` | |\n")},
		{Path: "b/fact_y.md", Content: fixtures.LineageDoc("fact_y", "Fact", []string{"b"},
			"| `b` | `ds_two.shared` | `b` | |\n")},
		{Path: "c/fact_z.md", Content: fixtures.LineageDoc("fact_z", "Fact", []string{"c"},
			"| `c` | `shared` | `c` | |\n")},
	})
	ids := map[string]bool{}
	for _, s := range m.SourceTables {
		ids[s.ID] = true
	}
	if !ids["ds_one.shared"] || !ids["ds_two.shared"] || !ids["shared"] {
		t.Errorf("source tables = %+v, want all three kept distinct", m.SourceTables)
	}
}
