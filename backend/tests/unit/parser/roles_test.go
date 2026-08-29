// What a document's Type property turns into, across modelling vocabularies.
package parser_test

import (
	"testing"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

// kindOf parses a one-table document declaring the given Type and returns the
// role it resolved to.
func kindOf(t *testing.T, name, declared string) model.TableKind {
	t.Helper()
	doc := fixtures.Doc(name, declared, "Domain One", []string{"id"}, "")
	res := parser.Parse([]parser.File{fixtures.File("domain_one/"+name+".md", doc)})
	if len(res.Tables) != 1 {
		t.Fatalf("tables = %d for type %q", len(res.Tables), declared)
	}
	return res.Tables[0].Kind
}

func TestKindAcrossVocabularies(t *testing.T) {
	cases := []struct {
		declared string
		want     model.TableKind
	}{
		// Kimball, including the qualifiers documents actually carry.
		{"Fact", model.KindFact},
		{"Fact Table", model.KindFact},
		{"Periodic Snapshot", model.KindFact},
		{"Dimension", model.KindDimension},
		{"Dimension (Conformed)", model.KindDimension},
		{"Slowly Changing Dimension", model.KindDimension},

		// The specific reading must beat the general one, even though the
		// general one is a substring of it.
		{"Factless Fact", model.KindFactless},
		{"Junk Dimension", model.KindJunk},
		{"Degenerate Dimension", model.KindDegenerate},

		// Snowflake: a normalised dimension is an outrigger, not a dimension.
		{"Outrigger", model.KindOutrigger},
		{"Sub-Dimension", model.KindOutrigger},
		{"Snowflaked Dimension", model.KindOutrigger},

		// Data Vault.
		{"Hub", model.KindHub},
		{"Link", model.KindLink},
		{"Satellite", model.KindSatellite},
		{"Effectivity Satellite", model.KindSatellite},
		{"Point-in-Time", model.KindPIT},

		// Third normal form.
		{"Entity", model.KindEntity},
		{"Junction Table", model.KindAssociative},
		{"Lookup Table", model.KindLookup},
		{"Reference Data", model.KindReference},
	}
	for _, c := range cases {
		if got := kindOf(t, "some_table", c.declared); got != c.want {
			t.Errorf("Type %q = %q, want %q", c.declared, got, c.want)
		}
	}
}

// A role nobody anticipated is the whole point of an open vocabulary: it keeps
// its own name rather than collapsing into "unknown".
func TestUnrecognisedTypeBecomesItsOwnRole(t *testing.T) {
	if got := kindOf(t, "some_table", "Anchor"); got != "anchor" {
		t.Errorf("Anchor = %q, want anchor", got)
	}
	if got := kindOf(t, "some_table", "Anchor (Historised)"); got != "anchor" {
		t.Errorf("qualifier should not change the role: got %q", got)
	}
	if got := kindOf(t, "some_table", "Tie Table"); got != "tie_table" {
		t.Errorf("Tie Table = %q, want tie_table", got)
	}
}

// Prose in the Type column is not a role. Without this, a sentence would become
// a role of its own and clutter every filter list in the UI.
func TestProseTypeFallsBackToUnknown(t *testing.T) {
	long := "This table holds one row per customer and is rebuilt nightly"
	if got := kindOf(t, "some_table", long); got != model.KindUnknown {
		t.Errorf("prose Type = %q, want unknown", got)
	}
}

// The name is only consulted when the document declares no Type at all.
func TestKindFromNamingConventions(t *testing.T) {
	cases := []struct {
		name string
		want model.TableKind
	}{
		{"fact_orders", model.KindFact},
		{"dim_customers", model.KindDimension},
		{"hub_customer", model.KindHub},
		{"lnk_order_customer", model.KindLink},
		{"sat_customer_detail", model.KindSatellite},
		{"xref_order_promo", model.KindAssociative},
		{"lkp_status", model.KindLookup},
		{"customer_dim", model.KindDimension},
		{"orders_fact", model.KindFact},
		{"something_else", model.KindUnknown},
	}
	for _, c := range cases {
		if got := kindOf(t, c.name, ""); got != c.want {
			t.Errorf("name %q = %q, want %q", c.name, got, c.want)
		}
	}
}

// An explicit Type wins over the naming convention: the document is the more
// deliberate statement of the two.
func TestDeclaredTypeBeatsName(t *testing.T) {
	if got := kindOf(t, "dim_customers", "Hub"); got != model.KindHub {
		t.Errorf("declared Hub on a dim_ name = %q, want hub", got)
	}
}

func TestRoleOfDescribesUnknownRoles(t *testing.T) {
	r := model.RoleOf(model.KindSatellite)
	if r.Label != "Satellite" || r.Family != model.FamilyVault {
		t.Errorf("satellite = %+v", r)
	}
	// A role read from a document still gets a usable label and family.
	own := model.RoleOf("tie_table")
	if own.Label != "Tie table" || own.Family != model.FamilyOther {
		t.Errorf("tie_table = %+v", own)
	}
	if own.Connective {
		t.Error("an unrecognised role must not be assumed connective")
	}
}

// Connective roles are the ones a missing join is a bug for.
func TestConnectiveRoles(t *testing.T) {
	for _, k := range []model.TableKind{model.KindFact, model.KindLink, model.KindAssociative, model.KindBridge} {
		if !model.IsConnective(k) {
			t.Errorf("%q should be connective", k)
		}
	}
	for _, k := range []model.TableKind{model.KindDimension, model.KindSatellite, model.KindHub, model.KindUnknown} {
		if model.IsConnective(k) {
			t.Errorf("%q should not be connective", k)
		}
	}
}

// A domain index is recognised by its diagram heading, and star schema wording
// is only one of the headings a directory might use.
func TestDomainIndexHeadingsBeyondStarSchema(t *testing.T) {
	for _, heading := range []string{
		"Star Schema Diagram",
		"Snowflake Schema Diagram",
		"Data Model Diagram",
		"ER Diagram",
	} {
		doc := "# Domain One\n\n## " + heading + "\n\n```mermaid\nerDiagram\n  a ||--o{ b : x\n```\n"
		res := parser.Parse([]parser.File{fixtures.File("domain_one.md", doc)})
		if len(res.Domains) != 1 {
			t.Errorf("heading %q produced %d domains", heading, len(res.Domains))
			continue
		}
		if res.Domains[0].Mermaid == "" {
			t.Errorf("heading %q lost the diagram", heading)
		}
	}
}
