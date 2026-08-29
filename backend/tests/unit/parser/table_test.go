// What one table document turns into: its overview properties, columns,
// lineage rows, declared relationships and caveats.
package parser_test

import (
	"testing"

	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

func TestParseTableDoc(t *testing.T) {
	res := parser.Parse([]parser.File{fixtures.File("demo/fact_demo.md", fixtures.SampleTableDoc)})
	if len(res.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(res.Tables))
	}
	tb := res.Tables[0]
	if tb.Name != "fact_demo" || tb.ID != "demo/fact_demo" {
		t.Errorf("name/id = %q/%q", tb.Name, tb.ID)
	}
	if tb.Kind != "fact" {
		t.Errorf("kind = %q", tb.Kind)
	}
	if tb.Grain != "One row per thing." {
		t.Errorf("grain = %q", tb.Grain)
	}
	if tb.Description != "Prose describing the table." {
		t.Errorf("description = %q", tb.Description)
	}
	if len(tb.Columns) != 2 {
		t.Fatalf("columns = %d", len(tb.Columns))
	}
	if !tb.Columns[0].IsPK {
		t.Error("demo_id should be PK")
	}
	if !tb.Columns[1].IsFK {
		t.Error("alpha_id should be FK via join key")
	}
	if len(tb.ColumnLineage) != 2 || !tb.ColumnLineage[1].Derived {
		t.Errorf("lineage = %+v", tb.ColumnLineage)
	}
	if len(tb.Relationships) != 1 {
		t.Fatalf("relationships = %d", len(tb.Relationships))
	}
	r := tb.Relationships[0]
	if r.TargetRef != "dim_alpha" || r.FromColumn != "alpha_id" || r.ToColumn != "alpha_id" {
		t.Errorf("relationship = %+v", r)
	}
	if tb.RelationshipNote != "Some prose about joins." {
		t.Errorf("relationship note = %q", tb.RelationshipNote)
	}
	if len(tb.Notes) != 2 {
		t.Errorf("notes = %+v", tb.Notes)
	}
}

func TestEscapedPipeInCell(t *testing.T) {
	doc := "## Overview\n\n| Property | Value |\n|---|---|\n| **Table Name** | `dim_x` |\n| **Type** | Dimension |\n\n" +
		"## Columns\n\n| Column | Type | Description |\n|---|---|---|\n| `a` | STRING | left \\| right |\n"
	res := parser.Parse([]parser.File{fixtures.File("d/dim_x.md", doc)})
	if len(res.Tables) != 1 || len(res.Tables[0].Columns) != 1 {
		t.Fatalf("unexpected parse: %+v", res.Tables)
	}
	if got := res.Tables[0].Columns[0].Description; got != "left | right" {
		t.Errorf("description = %q", got)
	}
}
