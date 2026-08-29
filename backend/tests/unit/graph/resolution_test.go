// How a declared relationship target is matched to a real table: same domain
// first, then a conformed instance elsewhere, then nothing.
package graph_test

import (
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

func TestLocalResolutionPreferredOverConformed(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "domain_one/fact_primary.md", Content: fixtures.Doc("fact_primary", "Fact", "Domain One",
			[]string{"primary_id", "alpha_id"}, "| `dim_alpha` | `alpha_id = alpha_id` | Many-to-one |\n")},
		{Path: "domain_one/dim_alpha.md", Content: fixtures.Doc("dim_alpha", "Dimension", "Domain One", []string{"alpha_id"}, "")},
		{Path: "users/dim_alpha.md", Content: fixtures.Doc("dim_alpha", "Dimension", "Conformed", []string{"alpha_id"}, "")},
	})
	var got model.Relationship
	for _, tb := range m.Tables {
		if tb.ID == "domain_one/fact_primary" {
			got = tb.Relationships[0]
		}
	}
	if got.Resolution != model.ResolvedLocal {
		t.Fatalf("resolution = %q, want local", got.Resolution)
	}
	if got.ToTableID != "domain_one/dim_alpha" {
		t.Errorf("target = %q, want domain_one/dim_alpha", got.ToTableID)
	}
}

func TestCrossDomainBindsToDeclaredConformed(t *testing.T) {
	// "Domain Four & Cross-Domain Reporting" is a domain name, not a claim to be
	// the conformed authority; the instance labelled "Conformed" must win.
	m := fixtures.Build([]parser.File{
		{Path: "domain_three/fact_x.md", Content: fixtures.Doc("fact_x", "Fact", "Domain Three",
			[]string{"id", "beta_id"}, "| `dim_beta` | `beta_id = beta_id` | Many-to-one |\n")},
		{Path: "domain_four/dim_beta.md", Content: fixtures.Doc("dim_beta", "Dimension",
			"Domain Four & Cross-Domain Reporting", []string{"beta_id", "a", "b", "c"}, "")},
		{Path: "domain_two/dim_beta.md", Content: fixtures.Doc("dim_beta", "Dimension", "Conformed",
			[]string{"beta_id", "x"}, "")},
	})
	var got model.Relationship
	for _, tb := range m.Tables {
		if tb.ID == "domain_three/fact_x" {
			got = tb.Relationships[0]
		}
	}
	if got.Resolution != model.ResolvedConformed {
		t.Fatalf("resolution = %q, want conformed", got.Resolution)
	}
	if got.ToTableID != "domain_two/dim_beta" {
		t.Errorf("target = %q, want domain_two/dim_beta (the declared conformed instance)", got.ToTableID)
	}
	if len(got.Candidates) != 2 {
		t.Errorf("candidates = %v, want both instances listed", got.Candidates)
	}
}

func TestRichestDefinitionWinsWithoutDeclaration(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "a/fact_x.md", Content: fixtures.Doc("fact_x", "Fact", "A",
			[]string{"id", "gamma_id"}, "| `dim_gamma` | `gamma_id = gamma_id` | Many-to-one |\n")},
		{Path: "b/dim_gamma.md", Content: fixtures.Doc("dim_gamma", "Dimension", "B", []string{"gamma_id"}, "")},
		{Path: "c/dim_gamma.md", Content: fixtures.Doc("dim_gamma", "Dimension", "C",
			[]string{"gamma_id", "name", "region"}, "")},
	})
	for _, tb := range m.Tables {
		if tb.ID == "a/fact_x" {
			if got := tb.Relationships[0].ToTableID; got != "c/dim_gamma" {
				t.Errorf("target = %q, want c/dim_gamma (most columns)", got)
			}
		}
	}
}

func TestNarrativeAndUnresolvedReferences(t *testing.T) {
	m := fixtures.Build([]parser.File{
		{Path: "a/dim_x.md", Content: fixtures.Doc("dim_x", "Dimension", "A", []string{"id"},
			"| `Various Fact Tables` | `id = id` | One-to-many |\n"+
				"| `fact_missing` | `id = id` | One-to-many |\n")},
	})
	rels := m.Tables[0].Relationships
	if rels[0].Resolution != model.ResolvedNarrative {
		t.Errorf("prose reference resolution = %q, want narrative", rels[0].Resolution)
	}
	if rels[1].Resolution != model.ResolvedUnresolved {
		t.Errorf("missing table resolution = %q, want unresolved", rels[1].Resolution)
	}
	var errs int
	for _, d := range m.Diagnostics {
		if d.Severity == model.SeverityError {
			errs++
		}
	}
	if errs != 1 {
		t.Errorf("error diagnostics = %d, want 1", errs)
	}
	// Neither reference should become a drawable edge.
	if got := len(graph.Edges(m)); got != 0 {
		t.Errorf("edges = %d, want 0", got)
	}
}
