// The shipped AdventureWorks snowflake demo documentation set.
//
// docs/demo/adventureworks-snowflake-ddd is a star schema whose dimensions are
// normalised rather than flat, which is the variation the word snowflake names.
// Two chains run three levels deep -- product to subcategory to category, and
// customer to geography to territory -- and the second of them is shared by two
// dimensions in two different contexts, which is the argument for normalising
// in the first place.
//
// It is also the only set whose tables are named in PascalCase, following
// AdventureWorks rather than the dbt convention the other four use. That is
// deliberate: none of `DimProduct`, `FactInternetSales` or `DimGeography`
// matches a naming convention kindFromName recognises, so every role in this
// set has to come from the document's declared Type.
package demo_test

import (
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
)

const snowflakeDir = "../../../../docs/demo/adventureworks-snowflake-ddd"

func loadSnowflake(t *testing.T) *model.Model { return loadSet(t, snowflakeDir) }

func TestSnowflakeStats(t *testing.T) {
	m := loadSnowflake(t)
	s := m.Snapshot.Stats
	checkStats(t, []stat{
		{"files parsed", s.FilesParsed, 18},
		{"files skipped", s.FilesSkipped, 0},
		{"domains", s.Domains, 6},
		{"tables", s.Tables, 12},
		{"columns", s.Columns, 91},
		{"conformed instances", s.Conformed, 2},
		{"declared relationships", s.Relationships, 24},
		{"normalised edges", len(graph.Edges(m)), 14},
		{"lineage edges", s.LineageEdges, 89},
		{"source tables", s.SourceTables, 12},
	})
}

// TestSnowflakeRolesComeFromTheDeclaredType is the reason this set is named the
// way it is. `DimProduct` does not start with `dim_`, so the naming-convention
// fallback cannot help: if the parser stopped reading the Type property, every
// table here would come out unknown and this test would say so.
func TestSnowflakeRolesComeFromTheDeclaredType(t *testing.T) {
	m := loadSnowflake(t)
	got := map[model.TableKind]int{}
	for _, tb := range m.Tables {
		got[tb.Kind]++
		if tb.Kind == model.KindUnknown {
			t.Errorf("%s came out unknown; its Type says %q", tb.ID, tb.KindRaw)
		}
	}
	for k, n := range map[model.TableKind]int{
		model.KindDimension: 6,
		model.KindFact:      3,
		model.KindOutrigger: 3,
	} {
		if got[k] != n {
			t.Errorf("%s tables = %d, want %d", k, got[k], n)
		}
	}
}

// TestSnowflakeChains walks the two normalised chains the set exists to show.
// A flat star has none of these edges: every one of them is a dimension joined
// to another dimension.
func TestSnowflakeChains(t *testing.T) {
	m := loadSnowflake(t)
	edges := map[string]bool{}
	for _, e := range graph.Edges(m) {
		edges[e.From+"->"+e.To] = true
	}
	chains := [][]string{
		// Product: fact -> dimension -> outrigger -> outrigger.
		{"sales/FactInternetSales", "product/DimProduct", "product/DimProductSubcategory", "product/DimProductCategory"},
		// Customer: fact -> dimension -> outrigger -> dimension.
		{"sales/FactInternetSales", "customer/DimCustomer", "customer/DimGeography", "sales/DimSalesTerritory"},
	}
	for _, chain := range chains {
		for i := 0; i+1 < len(chain); i++ {
			if !edges[chain[i]+"->"+chain[i+1]] {
				t.Errorf("chain broken: %s -> %s did not resolve", chain[i], chain[i+1])
			}
		}
	}
	// The shared outrigger: two dimensions in two contexts pointing at one table.
	if !edges["reseller/DimReseller->customer/DimGeography"] {
		t.Error("DimReseller no longer shares DimGeography with DimCustomer; the sharing is the argument for normalising")
	}
}

// TestSnowflakeDiagnostics pins the flaws the sample is built around.
func TestSnowflakeDiagnostics(t *testing.T) {
	checkDiagnostics(t, loadSnowflake(t), map[string]int{
		"unresolved_reference":   1,  // DimEmployee, in an unmodelled context
		"cross_domain_reference": 13, // a normalised model borrows far more than a flat one
		"conformed_drift":        1,  // customer/DimDate vs the kernel's
		"unmatched_join_key":     1,  // GeographyID = GeoKey, written from the source schema
		"empty_domain":           1,  // human_resources.md has no directory
		"isolated_fact":          1,  // FactSalesQuota names its dimensions in prose
		"narrative_reference":    2,  // "Both Fact Tables", "The Employee and Date Dimensions"
		"undocumented_lineage":   2,  // prose in the Source Table column
	}, 1) // exactly one error, so `relctl -strict` on the demo exits non-zero
}

// TestSnowflakeCrossDomainIsHigherThanAStar records something the set is meant
// to demonstrate rather than merely something it happens to do. Normalising a
// dimension out multiplies the boundary crossings: DimGeography is read by two
// contexts and reads a third, where a flat DimCustomer would have crossed
// nothing at all.
func TestSnowflakeCrossDomainIsHigherThanAStar(t *testing.T) {
	m := loadSnowflake(t)
	n := 0
	for _, d := range m.Diagnostics {
		if d.Code == "cross_domain_reference" {
			n++
		}
	}
	if n <= 10 {
		t.Errorf("cross-domain references = %d; the set is supposed to show that normalising raises this, and every other set is in single figures", n)
	}
}

// TestSnowflakeJoinKeyOrientation is the case every set carries.
func TestSnowflakeJoinKeyOrientation(t *testing.T) {
	got, ok := relationshipTo(loadSnowflake(t), "sales/FactInternetSales", "DimDate")
	if !ok {
		t.Fatal("FactInternetSales no longer declares a relationship to DimDate")
	}
	if got.JoinKeyRaw != "DateKey = OrderDateKey" {
		t.Fatalf("the document no longer writes the key backwards (%q); that case is the point of the sample", got.JoinKeyRaw)
	}
	if got.FromColumn != "OrderDateKey" || got.ToColumn != "DateKey" {
		t.Errorf("oriented key = %q = %q, want OrderDateKey = DateKey", got.FromColumn, got.ToColumn)
	}
	if got.ToTableID != "shared_kernel/DimDate" {
		t.Errorf("target = %q, want shared_kernel/DimDate", got.ToTableID)
	}
}

// TestSnowflakeTwoSidedDeclarationsCollapse checks the seven joins the sample
// declares from both sides. A chain is declared from both ends more often than
// a star is: each link in it is somebody's parent and somebody else's child.
func TestSnowflakeTwoSidedDeclarationsCollapse(t *testing.T) {
	checkTwoSided(t, loadSnowflake(t), map[string][]string{
		"customer/DimCustomer->customer/DimGeography": {
			"customer/DimCustomer", "customer/DimGeography"},
		"customer/DimGeography->sales/DimSalesTerritory": {
			"customer/DimGeography", "sales/DimSalesTerritory"},
		"product/DimProduct->product/DimProductSubcategory": {
			"product/DimProduct", "product/DimProductSubcategory"},
		"product/DimProductSubcategory->product/DimProductCategory": {
			"product/DimProductCategory", "product/DimProductSubcategory"},
		"sales/FactInternetSales->customer/DimCustomer": {
			"customer/DimCustomer", "sales/FactInternetSales"},
		"sales/FactInternetSales->product/DimProduct": {
			"product/DimProduct", "sales/FactInternetSales"},
		"sales/FactResellerSales->reseller/DimReseller": {
			"reseller/DimReseller", "sales/FactResellerSales"},
	})
}

// TestSnowflakeSourceCanonicalisation checks the sample's inconsistent citation
// of the sales order header folds onto a single lineage node.
func TestSnowflakeSourceCanonicalisation(t *testing.T) {
	m := loadSnowflake(t)
	for _, s := range m.SourceTables {
		if s.ID == "stg_salesorderheader" {
			t.Errorf("bare stg_salesorderheader survived as its own source node; it should fold onto adventureworks.stg_salesorderheader")
		}
		if s.Dataset != "adventureworks" {
			t.Errorf("source %q has dataset %q, want every demo source qualified with adventureworks", s.ID, s.Dataset)
		}
	}
	readers := map[string]bool{}
	for _, tb := range m.Tables {
		for _, l := range tb.ColumnLineage {
			if l.SourceTable == "adventureworks.stg_salesorderheader" {
				readers[tb.ID] = true
			}
		}
	}
	for _, want := range []string{
		"sales/FactInternetSales", // cited it qualified
		"sales/FactResellerSales", // cited it bare
	} {
		if !readers[want] {
			t.Errorf("%s does not read adventureworks.stg_salesorderheader; the mixed-citation case is the point", want)
		}
	}
}
