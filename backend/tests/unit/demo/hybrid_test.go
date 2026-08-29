// The shipped Northwind hybrid demo documentation set, parsed and resolved.
//
// docs/demo/northwind-hybrid-ddd is a warehouse with two modelling styles in
// it: a Data Vault raw layer that records what arrived, and a Kimball
// presentation layer built on top of it, with one business vault table
// straddling the two. It is the set that proves roles are a drawing concern and
// nothing more -- a fact joined to a hub resolves by exactly the same rules as
// a fact joined to a dimension, because the resolver never looks at either
// table's role.
//
// It also carries the set's snowflake: dim_product joins out to dim_category
// rather than folding it in.
package demo_test

import (
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
)

const hybridDir = "../../../../docs/demo/northwind-hybrid-ddd"

func loadHybrid(t *testing.T) *model.Model { return loadSet(t, hybridDir) }

func TestHybridStats(t *testing.T) {
	m := loadHybrid(t)
	s := m.Snapshot.Stats
	checkStats(t, []stat{
		{"files parsed", s.FilesParsed, 19},
		{"files skipped", s.FilesSkipped, 0},
		{"domains", s.Domains, 6},
		{"tables", s.Tables, 13},
		{"columns", s.Columns, 88},
		{"conformed instances", s.Conformed, 2},
		{"declared relationships", s.Relationships, 23},
		{"normalised edges", len(graph.Edges(m)), 15},
		{"lineage edges", s.LineageEdges, 85},
		{"source tables", s.SourceTables, 10},
	})
}

// TestHybridRoles pins the mix. Three vocabularies in one model is the point of
// the set, so a change that quietly collapsed any of them fails here.
func TestHybridRoles(t *testing.T) {
	got := map[model.TableKind]int{}
	for _, tb := range loadHybrid(t).Tables {
		got[tb.Kind]++
	}
	for k, n := range map[model.TableKind]int{
		model.KindDimension: 4, // three in the star, plus the kernel's calendar
		model.KindFact:      2,
		model.KindHub:       2,
		model.KindSatellite: 2,
		model.KindBridge:    1,
		model.KindLink:      1,
		model.KindOutrigger: 1,
	} {
		if got[k] != n {
			t.Errorf("%s tables = %d, want %d", k, got[k], n)
		}
	}
	if got[model.KindUnknown] != 0 {
		t.Errorf("%d tables came out unknown; every document in the set declares a Type", got[model.KindUnknown])
	}
}

// TestHybridCrossVocabularyJoins is the headline. Each of these edges joins a
// table of one modelling style to a table of another, and every one of them
// resolves through the ordinary cardinality-and-columns path -- there is no
// special case for them anywhere in the resolver, which is exactly what the
// set exists to demonstrate.
func TestHybridCrossVocabularyJoins(t *testing.T) {
	m := loadHybrid(t)
	kind := map[string]model.TableKind{}
	for _, tb := range m.Tables {
		kind[tb.ID] = tb.Kind
	}

	want := map[string]model.TableKind{
		"presentation_sales/fact_orders->raw_vault/hub_order":                   model.KindHub,
		"presentation_sales/dim_customer->raw_vault/hub_customer":               model.KindHub,
		"business_vault/bridge_customer_order->raw_vault/hub_customer":          model.KindHub,
		"business_vault/bridge_customer_order->raw_vault/hub_order":             model.KindHub,
		"business_vault/bridge_customer_order->presentation_sales/dim_customer": model.KindDimension,
	}
	found := map[string]bool{}
	for _, e := range graph.Edges(m) {
		found[e.From+"->"+e.To] = true
	}
	for edge, targetKind := range want {
		if !found[edge] {
			t.Errorf("cross-vocabulary edge %s did not resolve; it is the point of the set", edge)
		}
		_ = targetKind
	}

	// And the snowflake: a dimension joined to a dimension.
	if !found["presentation_catalog/dim_product->presentation_catalog/dim_category"] {
		t.Error("dim_product no longer joins out to its category outrigger")
	}
	if kind["presentation_catalog/dim_category"] != model.KindOutrigger {
		t.Errorf("dim_category = %q, want outrigger", kind["presentation_catalog/dim_category"])
	}
}

// TestHybridDiagnostics pins the flaws the sample is built around.
//
// There is no isolated_fact or isolated_table here, unlike the other four sets:
// every connective table in this one is joined to something, and inventing an
// orphan just to even the sets up would have meant a table with no reason to
// exist. The vault set covers that check.
func TestHybridDiagnostics(t *testing.T) {
	checkDiagnostics(t, loadHybrid(t), map[string]int{
		"unresolved_reference":   1, // dim_shipper, in an unmodelled context
		"cross_domain_reference": 8, // both layers reach across, in both directions
		"conformed_drift":        1, // presentation_catalog/dim_date vs the kernel's
		"unmatched_join_key":     1, // product_id = product_number
		"empty_domain":           1, // shipping.md has no directory
		"narrative_reference":    1, // "Every Fact Table in the Presentation Layer"
		"undocumented_lineage":   3, // prose in the Source Table column
	}, 1) // exactly one error, so `relctl -strict` on the demo exits non-zero
}

// TestHybridJoinKeyOrientation is the same case the star sets carry, checked
// here because a hybrid warehouse is where a modeller is most likely to write
// a key backwards -- the raw layer joins on hash keys and the star joins on
// surrogate keys, and the two habits do not agree about which side comes first.
func TestHybridJoinKeyOrientation(t *testing.T) {
	got, ok := relationshipTo(loadHybrid(t), "presentation_sales/fact_orders", "dim_date")
	if !ok {
		t.Fatal("fact_orders no longer declares a relationship to dim_date")
	}
	if got.JoinKeyRaw != "date_key = order_date_key" {
		t.Fatalf("the document no longer writes the key backwards (%q); that case is the point of the sample", got.JoinKeyRaw)
	}
	if got.FromColumn != "order_date_key" || got.ToColumn != "date_key" {
		t.Errorf("oriented key = %q = %q, want order_date_key = date_key", got.FromColumn, got.ToColumn)
	}
	if got.ToTableID != "shared_kernel/dim_date" {
		t.Errorf("target = %q, want shared_kernel/dim_date", got.ToTableID)
	}
}

// TestHybridTwoSidedDeclarationsCollapse checks the six joins the sample
// declares from both sides and which merge into one edge each.
//
// fact_order_items/dim_product is declared from both sides too and is
// deliberately absent from this list: see TestHybridBadKeySplitsTheEdge.
func TestHybridTwoSidedDeclarationsCollapse(t *testing.T) {
	checkTwoSided(t, loadHybrid(t), map[string][]string{
		"presentation_catalog/dim_product->presentation_catalog/dim_category": {
			"presentation_catalog/dim_category", "presentation_catalog/dim_product"},
		"presentation_sales/fact_order_items->presentation_sales/fact_orders": {
			"presentation_sales/fact_order_items", "presentation_sales/fact_orders"},
		"raw_vault/lnk_order_customer->raw_vault/hub_customer": {
			"raw_vault/hub_customer", "raw_vault/lnk_order_customer"},
		"raw_vault/lnk_order_customer->raw_vault/hub_order": {
			"raw_vault/hub_order", "raw_vault/lnk_order_customer"},
		"raw_vault/sat_customer_details->raw_vault/hub_customer": {
			"raw_vault/hub_customer", "raw_vault/sat_customer_details"},
		"raw_vault/sat_order_details->raw_vault/hub_order": {
			"raw_vault/hub_order", "raw_vault/sat_order_details"},
	})
}

// TestHybridBadKeySplitsTheEdge pins the second, quieter consequence of a join
// key written wrong. fact_order_items and dim_product both declare the join
// between them, but the fact writes it on the source schema's column names. The
// two declarations therefore do not merge, and the graph shows two edges
// between the same pair of tables where it should show one.
//
// The warning is the loud symptom; this is the one that actually misleads a
// reader, and it is worth a test of its own so that nobody "fixes" the demo by
// correcting the key and quietly removes the only coverage of both.
func TestHybridBadKeySplitsTheEdge(t *testing.T) {
	m := loadHybrid(t)
	var edges []graph.Edge
	for _, e := range graph.Edges(m) {
		if e.From == "presentation_sales/fact_order_items" && e.To == "presentation_catalog/dim_product" {
			edges = append(edges, e)
		}
	}
	if len(edges) != 2 {
		t.Fatalf("fact_order_items -> dim_product edges = %d, want 2: one per spelling of the key", len(edges))
	}
	keys := []string{edges[0].FromColumn + " = " + edges[0].ToColumn, edges[1].FromColumn + " = " + edges[1].ToColumn}
	want := map[string]bool{"product_id = product_number": true, "product_key = product_key": true}
	for _, k := range keys {
		if !want[k] {
			t.Errorf("unexpected join key %q on the split edge", k)
		}
	}
	for _, e := range edges {
		if len(e.DeclaredBy) != 1 {
			t.Errorf("edge %q was declared by %v; a split edge has exactly one side each", keys, e.DeclaredBy)
		}
	}
}

// TestHybridPresentationIsBuiltOnTheVault checks the lineage that makes this a
// hybrid rather than two schemas in one directory: the star's tables cite vault
// tables as their source, not the staging models the vault was loaded from.
func TestHybridPresentationIsBuiltOnTheVault(t *testing.T) {
	m := loadHybrid(t)
	want := map[string]string{
		"presentation_sales/dim_customer":      "northwind.sat_customer_details",
		"presentation_sales/fact_orders":       "northwind.lnk_order_customer",
		"business_vault/bridge_customer_order": "northwind.lnk_order_customer",
	}
	for id, src := range want {
		found := false
		for _, tb := range m.Tables {
			if tb.ID != id {
				continue
			}
			for _, l := range tb.ColumnLineage {
				if l.SourceTable == src {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s does not cite %s; the star being built on the vault is the point", id, src)
		}
	}
	for _, s := range m.SourceTables {
		if s.ID == "stg_orders" {
			t.Errorf("bare stg_orders survived as its own source node; it should fold onto northwind.stg_orders")
		}
	}
}
