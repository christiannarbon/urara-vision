// Package fixtures builds the markdown documents the test suites parse.
//
// The parser's input is documentation, so almost every test starts by writing
// a small document. Keeping the builders here means the unit and integration
// suites describe the same shapes, and a change to the documentation format is
// one edit rather than one per suite.
package fixtures

import (
	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/parser"
)

// Doc builds a minimal table document: an Overview block, a Columns table and
// a Relationships table. rels is the pre-rendered body of the relationships
// table, so a caller can write however many rows it needs.
func Doc(name, kind, domainLabel string, cols []string, rels string) string {
	s := "# " + name + "\n## Overview\n\n| Property | Value |\n|---|---|\n" +
		"| **Table Name** | `" + name + "` |\n| **Type** | " + kind + " |\n" +
		"| **Domain** | " + domainLabel + " |\n\n## Columns\n\n| Column | Type | Description |\n|---|---|---|\n"
	for _, c := range cols {
		s += "| `" + c + "` | STRING | col |\n"
	}
	s += "\n## Relationships\n\n| Related Table | Join Key | Relationship |\n|---|---|---|\n" + rels
	return s
}

// LineageDoc builds a table document carrying a Column-Level Lineage section
// instead of relationships.
func LineageDoc(name, kind string, cols []string, lineage string) string {
	s := "# " + name + "\n## Overview\n\n| Property | Value |\n|---|---|\n" +
		"| **Table Name** | `" + name + "` |\n| **Type** | " + kind + " |\n\n" +
		"## Columns\n\n| Column | Type | Description |\n|---|---|---|\n"
	for _, c := range cols {
		s += "| `" + c + "` | STRING | col |\n"
	}
	s += "\n## Column-Level Lineage\n\n| Column | Source Table | Source Column | Notes |\n|---|---|---|---|\n" + lineage
	return s
}

// File is a convenience constructor so table-driven tests stay readable.
func File(path, content string) parser.File {
	return parser.File{Path: path, Content: content}
}

// Build parses files and resolves them into a model, the way an ingest does.
func Build(files []parser.File) *model.Model {
	return BuildAs("snap", files)
}

// BuildAs is Build with the snapshot ID chosen by the caller. The integration
// suites give every test its own ID: both stores are snapshot-scoped and
// replace on write, so a unique ID is all the isolation they need.
func BuildAs(snapshotID string, files []parser.File) *model.Model {
	return graph.Build(snapshotID, "test", "test", parser.Parse(files))
}

// SampleTableDoc is a fully populated table document: every section the parser
// understands, with the cells a real document carries.
const SampleTableDoc = "# fact_demo\n" +
	"## Overview\n\n" +
	"| Property | Value |\n|---|---|\n" +
	"| **Table Name** | `fact_demo` |\n" +
	"| **Type** | Fact |\n" +
	"| **Domain** | Demo |\n" +
	"| **Grain** | One row per thing. |\n" +
	"| **Update Frequency** | Daily |\n" +
	"| **Layer** | Star Schema (proposed) |\n\n" +
	"Prose describing the table.\n\n" +
	"## Columns\n\n" +
	"| Column | Type | Description |\n|---|---|---|\n" +
	"| `demo_id` | STRING | Unique id (PK) |\n" +
	"| `alpha_id` | STRING | User who did it |\n\n" +
	"## Column-Level Lineage\n\n" +
	"| Column | Source Table | Source Column | Notes |\n|---|---|---|---|\n" +
	"| `demo_id` | `ds.raw_demo` | `id` | Primary Key |\n" +
	"| `alpha_id` | `ds.raw_demo` | `uid` | Derived: something |\n\n" +
	"## Relationships\n\n" +
	"Some prose about joins.\n\n" +
	"| Related Table | Join Key | Relationship |\n|---|---|---|\n" +
	"| `dim_alpha` | `alpha_id = alpha_id` | Many-to-one |\n\n" +
	"## Notes / Caveats\n\n" +
	"- First caveat.\n" +
	"- Second caveat.\n"

// StarSchema is a small but complete two-domain model: a fact joined to two
// dimensions, one of them a conformed dimension shared with a second domain,
// plus column lineage onto an upstream source model. The integration suites
// ingest it and then assert against what the stores return.
func StarSchema() []parser.File {
	return []parser.File{
		File("domain_one.md", "# Domain One Domain\n\n## Description\n\nThe domain_one domain.\n\n"+
			"## Lineage\n\n| Proposed Table | Source Model(s) |\n| :--- | :--- |\n"+
			"| `fact_primary` | `warehouse.upstream_model` |\n"),
		File("domain_one/fact_primary.md", "# fact_primary\n## Overview\n\n| Property | Value |\n|---|---|\n"+
			"| **Table Name** | `fact_primary` |\n| **Type** | Fact |\n| **Domain** | Domain One |\n"+
			"| **Grain** | One row per thing. |\n| **Update Frequency** | Daily |\n\n"+
			"Prose describing the table.\n\n"+
			"## Columns\n\n| Column | Type | Description |\n|---|---|---|\n"+
			"| `primary_id` | STRING | Unique id (PK) |\n"+
			"| `alpha_id` | STRING | User who did it |\n"+
			"| `beta_id` | STRING | Where it happened |\n\n"+
			"## Column-Level Lineage\n\n| Column | Source Table | Source Column | Notes |\n|---|---|---|---|\n"+
			"| `primary_id` | `warehouse.upstream_model` | `id` | Primary Key |\n\n"+
			"## Relationships\n\n| Related Table | Join Key | Relationship |\n|---|---|---|\n"+
			"| `dim_alpha` | `alpha_id = alpha_id` | Many-to-one |\n"+
			"| `dim_beta` | `beta_id = beta_id` | Many-to-one |\n\n"+
			"## Notes / Caveats\n\n- Excludes cancelled rows.\n"),
		File("domain_one/dim_alpha.md", Doc("dim_alpha", "Dimension", "Domain One",
			[]string{"alpha_id", "created_at"}, "")),
		File("domain_two.md", "# Domain Two Domain\n\n## Description\n\nThe domain_two domain.\n"),
		File("domain_two/dim_beta.md", Doc("dim_beta", "Dimension", "Conformed",
			[]string{"beta_id", "beta_name"}, "")),
	}
}
