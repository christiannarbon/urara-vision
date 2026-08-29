// What a domain index document turns into.
package parser_test

import (
	"testing"

	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

func TestDomainIndexParsing(t *testing.T) {
	doc := "# Domain One Domain - Star Schema Proposal\n\n## Description\n\nThe domain_one domain is the core.\n\n" +
		"## Star Schema Diagram\n\n```mermaid\nerDiagram\n  fact_primary ||--o{ dim_alpha : x\n```\n\n" +
		"## Lineage\n\n| Proposed Table | Source Model(s) |\n| :--- | :--- |\n" +
		"| `fact_primary` | `a_model`, `b_model` |\n"
	res := parser.Parse([]parser.File{fixtures.File("domain_one.md", doc)})
	if len(res.Domains) != 1 {
		t.Fatalf("domains = %d", len(res.Domains))
	}
	d := res.Domains[0]
	if d.ID != "domain_one" || d.Title != "Domain One Domain - Star Schema Proposal" {
		t.Errorf("id/title = %q/%q", d.ID, d.Title)
	}
	if d.Description != "The domain_one domain is the core." {
		t.Errorf("description = %q", d.Description)
	}
	if d.Mermaid == "" || d.Lineage[0].ProposedTable != "fact_primary" || len(d.Lineage[0].SourceModels) != 2 {
		t.Errorf("mermaid/lineage = %q / %+v", d.Mermaid, d.Lineage)
	}
}
