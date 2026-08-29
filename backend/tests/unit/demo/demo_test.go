// The shipped Jaffle Shop demo documentation set, parsed and resolved.
//
// docs/demo/jaffle-shop-ddd is one of three samples a new user can point the
// app at, and every flaw in it is deliberate: each one exists so that one check
// has something to find. That makes it a fixture as much as a demo, so what it
// produces is pinned here. A change to the resolver that alters these numbers
// is either a bug or a documented behaviour change, and a well-meant edit to
// the sample that "fixes" one of the flaws fails this suite rather than
// quietly removing a check's only coverage.
//
// The sibling suites in this package do the same for the Fintech and eShop
// sets; the walk they share lives in load_test.go.
package demo_test

import (
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
)

// demoDir is the sample set, relative to this test's package directory.
const demoDir = "../../../../docs/demo/jaffle-shop-ddd"

// load resolves the Jaffle Shop set.
func load(t *testing.T) *model.Model { return loadSet(t, demoDir) }

func TestDemoStats(t *testing.T) {
	m := load(t)
	s := m.Snapshot.Stats
	checkStats(t, []stat{
		{"files parsed", s.FilesParsed, 16},
		{"files skipped", s.FilesSkipped, 0},
		{"domains", s.Domains, 6},
		{"tables", s.Tables, 10},
		{"columns", s.Columns, 80},
		{"conformed instances", s.Conformed, 2},
		{"declared relationships", s.Relationships, 16},
		{"normalised edges", len(graph.Edges(m)), 9},
		{"lineage edges", s.LineageEdges, 60},
		{"source tables", s.SourceTables, 7},
	})
}

// TestDemoDiagnostics pins the flaws the sample is built around.
func TestDemoDiagnostics(t *testing.T) {
	checkDiagnostics(t, load(t), map[string]int{
		"unresolved_reference":   1, // dim_delivery_partner, in an unmodelled context
		"cross_domain_reference": 7, // Ordering owns no dimensions of its own
		"conformed_drift":        1, // customer_identity/dim_date vs the kernel's
		"unmatched_join_key":     1, // sales_date = calendar_date
		"empty_domain":           1, // delivery_logistics.md has no directory
		"isolated_fact":          1, // fact_supply_cost_snapshot
		"narrative_reference":    2, // "Various Fact Tables", "Product Catalog Dimensions"
		"undocumented_lineage":   2, // prose in the Source Table column
	}, 1) // exactly one error, so `relctl -strict` on the demo exits non-zero
}

// TestDemoJoinKeyOrientation is the sample's headline case: fact_orders writes
// its dim_date join key dimension-column-first on a Many-to-one row, and the
// resolver must recover the true orientation from the column lists.
func TestDemoJoinKeyOrientation(t *testing.T) {
	got, ok := relationshipTo(load(t), "ordering/fact_orders", "dim_date")
	if !ok {
		t.Fatal("ordering/fact_orders no longer declares a relationship to dim_date")
	}
	if got.JoinKeyRaw != "date_key = ordered_at_date_key" {
		t.Fatalf("the document no longer writes the key backwards (%q); that case is the point of the sample", got.JoinKeyRaw)
	}
	if got.FromColumn != "ordered_at_date_key" || got.ToColumn != "date_key" {
		t.Errorf("oriented key = %q = %q, want ordered_at_date_key = date_key", got.FromColumn, got.ToColumn)
	}
	if got.ToTableID != "shared_kernel/dim_date" {
		t.Errorf("target = %q, want shared_kernel/dim_date", got.ToTableID)
	}
}

// TestDemoTwoSidedDeclarationsCollapse checks the four joins the sample
// declares from both sides, which must each be a single edge.
func TestDemoTwoSidedDeclarationsCollapse(t *testing.T) {
	checkTwoSided(t, load(t), map[string][]string{
		"ordering/fact_order_items->ordering/fact_orders": {
			"ordering/fact_order_items", "ordering/fact_orders"},
		"ordering/fact_orders->customer_identity/dim_customers": {
			"customer_identity/dim_customers", "ordering/fact_orders"},
		"product_catalog/dim_supplies->product_catalog/dim_products": {
			"product_catalog/dim_products", "product_catalog/dim_supplies"},
		"store_operations/fact_location_daily_sales->store_operations/dim_locations": {
			"store_operations/dim_locations", "store_operations/fact_location_daily_sales"},
	})
}

// TestDemoSourceCanonicalisation checks the sample's inconsistent citation of
// one dbt model folds onto a single lineage node. Without it, asking what reads
// stg_products would miss the documents that cited it bare.
func TestDemoSourceCanonicalisation(t *testing.T) {
	m := load(t)
	for _, s := range m.SourceTables {
		if s.ID == "stg_products" {
			t.Errorf("bare stg_products survived as its own source node; it should fold onto jaffle_shop.stg_products")
		}
		if s.Dataset != "jaffle_shop" {
			t.Errorf("source %q has dataset %q, want every demo source qualified with jaffle_shop", s.ID, s.Dataset)
		}
	}
	readers := map[string]bool{}
	for _, tb := range m.Tables {
		for _, l := range tb.ColumnLineage {
			if l.SourceTable == "jaffle_shop.stg_products" {
				readers[tb.ID] = true
			}
		}
	}
	for _, want := range []string{
		"product_catalog/dim_products",              // cited it qualified
		"ordering/fact_order_items",                 // cited it bare
		"product_catalog/fact_supply_cost_snapshot", // cited it bare
	} {
		if !readers[want] {
			t.Errorf("%s does not read jaffle_shop.stg_products; the mixed-citation case is the point", want)
		}
	}
}
