// Findings the resolver reports about a model that parsed cleanly.
package graph_test

import (
	"testing"

	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

func TestIsolatedFactWarning(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "a/fact_lonely.md", Content: fixtures.Doc("fact_lonely", "Fact", "A", []string{"id"}, "")},
	})
	var found bool
	for _, d := range m.Diagnostics {
		if d.Code == "isolated_fact" && d.TableID == "a/fact_lonely" {
			found = true
		}
	}
	if !found {
		t.Error("expected an isolated_fact diagnostic")
	}
}
