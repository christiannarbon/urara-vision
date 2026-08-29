// Conformed dimensions: which tables are flagged, and when their definitions
// have drifted apart.
package graph_test

import (
	"testing"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

func TestConformedDriftDetected(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "a/dim_shared.md", Content: fixtures.Doc("dim_shared", "Dimension", "Conformed",
			[]string{"shared_id", "attr_one", "attr_two"}, "")},
		{Path: "b/dim_shared.md", Content: fixtures.Doc("dim_shared", "Dimension", "B",
			[]string{"shared_id", "attr_one", "attr_three"}, "")},
	})
	var found *model.Diagnostic
	for i := range m.Diagnostics {
		if m.Diagnostics[i].Code == "conformed_drift" {
			found = &m.Diagnostics[i]
		}
	}
	if found == nil {
		t.Fatal("expected a conformed_drift diagnostic")
	}
	if found.TableID != "b/dim_shared" {
		t.Errorf("drift reported on %q, want b/dim_shared", found.TableID)
	}
}

func TestIdenticalConformedInstancesProduceNoDrift(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "a/dim_shared.md", Content: fixtures.Doc("dim_shared", "Dimension", "Conformed", []string{"shared_id", "attr_one"}, "")},
		{Path: "b/dim_shared.md", Content: fixtures.Doc("dim_shared", "Dimension", "B", []string{"shared_id", "attr_one"}, "")},
	})
	for _, d := range m.Diagnostics {
		if d.Code == "conformed_drift" {
			t.Errorf("unexpected drift: %s", d.Message)
		}
	}
}

func TestStatsAndConformedFlags(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "a/dim_shared.md", Content: fixtures.Doc("dim_shared", "Dimension", "Conformed", []string{"shared_id"}, "")},
		{Path: "b/dim_shared.md", Content: fixtures.Doc("dim_shared", "Dimension", "B", []string{"shared_id"}, "")},
		{Path: "b/fact_y.md", Content: fixtures.Doc("fact_y", "Fact", "B",
			[]string{"id", "shared_id"}, "| `dim_shared` | `shared_id = shared_id` | Many-to-one |\n")},
	})
	if m.Snapshot.Stats.Tables != 3 || m.Snapshot.Stats.Conformed != 2 {
		t.Errorf("stats = %+v", m.Snapshot.Stats)
	}
	for _, tb := range m.Tables {
		if tb.Name == "dim_shared" && !tb.Conformed {
			t.Errorf("%s should be marked conformed", tb.ID)
		}
	}
}
