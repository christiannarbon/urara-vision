// The shipped eShop demo documentation set, parsed and resolved.
//
// docs/demo/eshop-ddd models an analytics warehouse over Microsoft's eShop
// reference application, where the bounded contexts are the microservices
// themselves and each one owns its own database. Like the other two sets, every
// flaw in it is deliberate, so what it produces is pinned here rather than left
// to drift.
package demo_test

import (
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
)

const eshopDir = "../../../../docs/demo/eshop-ddd"

func loadEshop(t *testing.T) *model.Model { return loadSet(t, eshopDir) }

func TestEshopStats(t *testing.T) {
	m := loadEshop(t)
	s := m.Snapshot.Stats
	checkStats(t, []stat{
		{"files parsed", s.FilesParsed, 17},
		{"files skipped", s.FilesSkipped, 0},
		{"domains", s.Domains, 6},
		{"tables", s.Tables, 11},
		{"columns", s.Columns, 101},
		{"conformed instances", s.Conformed, 2},
		{"declared relationships", s.Relationships, 22},
		{"normalised edges", len(graph.Edges(m)), 12},
		{"lineage edges", s.LineageEdges, 89},
		{"source tables", s.SourceTables, 13},
	})
}

// TestEshopDiagnostics pins the flaws this sample is built around.
func TestEshopDiagnostics(t *testing.T) {
	checkDiagnostics(t, loadEshop(t), map[string]int{
		"unresolved_reference":   1, // dim_payment_method, owned by an unmodelled Payment
		"cross_domain_reference": 9, // a service may not read another service's database
		"conformed_drift":        1, // basket/dim_buyer vs Identity's
		"unmatched_join_key":     1, // product_id = id
		"empty_domain":           1, // payment.md has no directory
		"isolated_fact":          1, // fact_stock_movements
		"narrative_reference":    2, // "Various Fact Tables", "Catalog Dimensions"
		"undocumented_lineage":   2, // prose in the Source Table column
	}, 1) // exactly one error, so `relctl -strict` on this set exits non-zero
}

// TestEshopJoinKeyOrientation is this sample's headline case: fact_orders
// writes its dim_date join key dimension-column-first on a Many-to-one row, and
// the resolver must recover the true orientation from the column lists.
func TestEshopJoinKeyOrientation(t *testing.T) {
	got, ok := relationshipTo(loadEshop(t), "ordering/fact_orders", "dim_date")
	if !ok {
		t.Fatal("ordering/fact_orders no longer declares a relationship to dim_date")
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

// TestEshopSplitBuyerJoin is this sample's own case, and the reason it is worth
// shipping alongside the others: Identity declares a join from its conformed
// dim_buyer to the basket fact, while the basket fact declares the same join to
// its own local copy. A table in the declaring context wins, so the two
// declarations do not merge -- the fact ends up joined to two different buyer
// dimensions, which is precisely the disagreement the graph exists to show.
func TestEshopSplitBuyerJoin(t *testing.T) {
	targets := map[string]bool{}
	for _, e := range graph.Edges(loadEshop(t)) {
		if e.From == "basket/fact_basket_events" && e.FromColumn == "buyer_key" {
			targets[e.To] = true
		}
	}
	for _, want := range []string{"basket/dim_buyer", "identity/dim_buyer"} {
		if !targets[want] {
			t.Errorf("fact_basket_events has no buyer_key edge to %s; the split join is the point of this sample", want)
		}
	}
	if len(targets) != 2 {
		t.Errorf("fact_basket_events buyer_key edges point at %d tables (%v), want exactly 2", len(targets), targets)
	}

	// The order fact, by contrast, has no local copy to prefer, so its buyer
	// binds to the conformed instance.
	r, ok := relationshipTo(loadEshop(t), "ordering/fact_orders", "dim_buyer")
	if !ok {
		t.Fatal("ordering/fact_orders no longer declares a relationship to dim_buyer")
	}
	if r.ToTableID != "identity/dim_buyer" {
		t.Errorf("fact_orders bound dim_buyer to %q, want identity/dim_buyer", r.ToTableID)
	}
}

// TestEshopTwoSidedDeclarationsCollapse checks the seven joins this sample
// declares from both sides, which must each be a single edge.
func TestEshopTwoSidedDeclarationsCollapse(t *testing.T) {
	checkTwoSided(t, loadEshop(t), map[string][]string{
		"basket/fact_basket_events->basket/dim_buyer": {
			"basket/dim_buyer", "basket/fact_basket_events"},
		"catalog/dim_catalog_item->catalog/dim_catalog_brand": {
			"catalog/dim_catalog_brand", "catalog/dim_catalog_item"},
		"catalog/dim_catalog_item->catalog/dim_catalog_type": {
			"catalog/dim_catalog_item", "catalog/dim_catalog_type"},
		"ordering/fact_order_items->catalog/dim_catalog_item": {
			"catalog/dim_catalog_item", "ordering/fact_order_items"},
		"ordering/fact_order_items->ordering/fact_orders": {
			"ordering/fact_order_items", "ordering/fact_orders"},
		"ordering/fact_orders->identity/dim_buyer": {
			"identity/dim_buyer", "ordering/fact_orders"},
		"ordering/fact_orders->ordering/dim_order_status": {
			"ordering/dim_order_status", "ordering/fact_orders"},
	})
}

// TestEshopSourceCanonicalisation checks the sample's inconsistent citation of
// the catalog's staging model folds onto a single lineage node.
func TestEshopSourceCanonicalisation(t *testing.T) {
	m := loadEshop(t)
	for _, s := range m.SourceTables {
		if s.ID == "stg_catalog_items" {
			t.Errorf("bare stg_catalog_items survived as its own source node; it should fold onto catalogdb.stg_catalog_items")
		}
		if s.Dataset == "" {
			t.Errorf("source %q has no dataset; every source in this set names the service database it came from", s.ID)
		}
	}
	readers := map[string]bool{}
	for _, tb := range m.Tables {
		for _, l := range tb.ColumnLineage {
			if l.SourceTable == "catalogdb.stg_catalog_items" {
				readers[tb.ID] = true
			}
		}
	}
	for _, want := range []string{
		"catalog/dim_catalog_item",  // cited it qualified
		"catalog/dim_catalog_brand", // cited it qualified
		"ordering/fact_order_items", // cited it bare
	} {
		if !readers[want] {
			t.Errorf("%s does not read catalogdb.stg_catalog_items; the mixed-citation case is the point", want)
		}
	}
}
