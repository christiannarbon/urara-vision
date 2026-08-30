// The shipped TPC-H Data Vault demo documentation set, parsed and resolved.
//
// docs/demo/tpch-vault-ddd is the set that proves the tool is not a star schema
// tool. It contains no fact and no dimension: it is hubs, links, satellites and
// a point-in-time table, and every relationship in it is resolved by the same
// cardinality-and-column-names rules that resolve a star. Like the other sets,
// every flaw in it is deliberate, so what it produces is pinned here.
//
// It is also the only set that exercises `isolated_table`, the diagnostic for a
// connective table that is joined to nothing but is not a fact.
package demo_test

import (
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
)

const vaultDir = "../../../../docs/demo/tpch-vault-ddd"

func loadVault(t *testing.T) *model.Model { return loadSet(t, vaultDir) }

func TestVaultStats(t *testing.T) {
	m := loadVault(t)
	s := m.Snapshot.Stats
	checkStats(t, []stat{
		{"files parsed", s.FilesParsed, 17},
		{"files skipped", s.FilesSkipped, 0},
		{"domains", s.Domains, 6},
		{"tables", s.Tables, 11},
		{"columns", s.Columns, 69},
		{"conformed instances", s.Conformed, 2},
		{"declared relationships", s.Relationships, 15},
		{"normalised edges", len(graph.Edges(m)), 7},
		{"lineage edges", s.LineageEdges, 65},
		{"source tables", s.SourceTables, 5},
	})
}

// TestVaultRoles is the point of the set: a model with no fact and no dimension
// in it, whose every table still carries the role its document declared.
func TestVaultRoles(t *testing.T) {
	got := map[model.TableKind]int{}
	for _, tb := range loadVault(t).Tables {
		got[tb.Kind]++
	}
	want := map[model.TableKind]int{
		model.KindHub:       5, // three subject areas, plus the kernel's and Supply's copy
		model.KindSatellite: 3,
		model.KindLink:      2,
		model.KindPIT:       1,
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s tables = %d, want %d", k, got[k], n)
		}
	}
	for _, unwanted := range []model.TableKind{model.KindFact, model.KindDimension, model.KindUnknown} {
		if got[unwanted] != 0 {
			t.Errorf("%d %s tables in a raw vault; the set exists to have none", got[unwanted], unwanted)
		}
	}
}

// TestVaultDiagnostics pins the flaws the sample is built around.
func TestVaultDiagnostics(t *testing.T) {
	checkDiagnostics(t, loadVault(t), map[string]int{
		"unresolved_reference":   1, // hub_shipmode, in an unmodelled context
		"cross_domain_reference": 4, // links and the PIT table reach across for their hubs
		"conformed_drift":        1, // supply/hub_nation vs the kernel's
		"unmatched_join_key":     1, // c_custkey = customer_key, written on business keys
		"empty_domain":           1, // shipping.md has no directory
		"isolated_table":         1, // lnk_part_supplier names its parent in prose
		"narrative_reference":    2, // "Every Satellite in the Raw Vault", "Part Hubs"
		"undocumented_lineage":   4, // prose in the Source Table column
	}, 1) // exactly one error, so `uraractl -strict` on the demo exits non-zero
}

// TestVaultIsolatedTableIsNotAFact is the distinction the diagnostic exists to
// draw. A link joined to nothing is a documentation gap and must be reported;
// a hub joined to nothing is an ordinary hub and must not be.
func TestVaultIsolatedTableIsNotAFact(t *testing.T) {
	m := loadVault(t)
	var isolated []string
	for _, d := range m.Diagnostics {
		if d.Code == "isolated_table" || d.Code == "isolated_fact" {
			isolated = append(isolated, d.TableID)
		}
	}
	if len(isolated) != 1 || isolated[0] != "supply/lnk_part_supplier" {
		t.Fatalf("isolated tables = %v, want only supply/lnk_part_supplier", isolated)
	}
	// hub_supplier declares no relationship at all and must stay quiet.
	for _, d := range m.Diagnostics {
		if d.TableID == "supply/hub_supplier" {
			t.Errorf("hub_supplier produced %s; a hub with no links yet is not a gap", d.Code)
		}
	}
}

// TestVaultJoinKeyOrientation is the set's headline case. pit_customer writes
// its satellite join satellite-column-first on a Many-to-one row, and because
// `load_date` exists on both tables the resolver cannot pick a side by name
// alone -- it has to use the fact that only one table carries the other column.
func TestVaultJoinKeyOrientation(t *testing.T) {
	got, ok := relationshipTo(loadVault(t), "business_vault/pit_customer", "sat_customer_details")
	if !ok {
		t.Fatal("pit_customer no longer declares a relationship to sat_customer_details")
	}
	if got.JoinKeyRaw != "load_date = sat_customer_details_ldts" {
		t.Fatalf("the document no longer writes the key backwards (%q); that case is the point of the sample", got.JoinKeyRaw)
	}
	if got.FromColumn != "sat_customer_details_ldts" || got.ToColumn != "load_date" {
		t.Errorf("oriented key = %q = %q, want sat_customer_details_ldts = load_date", got.FromColumn, got.ToColumn)
	}
	if got.ToTableID != "party/sat_customer_details" {
		t.Errorf("target = %q, want party/sat_customer_details", got.ToTableID)
	}
}

// TestVaultTwoSidedDeclarationsCollapse checks the five joins the sample
// declares from both sides. A vault declares them from both sides more often
// than a star does: a hub lists its satellites, and every satellite names its
// hub.
func TestVaultTwoSidedDeclarationsCollapse(t *testing.T) {
	checkTwoSided(t, loadVault(t), map[string][]string{
		"ordering/lnk_customer_order->ordering/hub_order": {
			"ordering/hub_order", "ordering/lnk_customer_order"},
		"ordering/lnk_customer_order->party/hub_customer": {
			"ordering/lnk_customer_order", "party/hub_customer"},
		"ordering/sat_order_details->ordering/hub_order": {
			"ordering/hub_order", "ordering/sat_order_details"},
		"party/sat_customer_details->party/hub_customer": {
			"party/hub_customer", "party/sat_customer_details"},
		"shared_kernel/sat_nation_details->shared_kernel/hub_nation": {
			"shared_kernel/hub_nation", "shared_kernel/sat_nation_details"},
	})
}

// TestVaultSourceCanonicalisation checks the sample's inconsistent citation of
// one staging view folds onto a single lineage node.
func TestVaultSourceCanonicalisation(t *testing.T) {
	m := loadVault(t)
	for _, s := range m.SourceTables {
		if s.ID == "v_stg_orders" {
			t.Errorf("bare v_stg_orders survived as its own source node; it should fold onto tpch.v_stg_orders")
		}
		if s.Dataset != "tpch" {
			t.Errorf("source %q has dataset %q, want every demo source qualified with tpch", s.ID, s.Dataset)
		}
	}
	readers := map[string]bool{}
	for _, tb := range m.Tables {
		for _, l := range tb.ColumnLineage {
			if l.SourceTable == "tpch.v_stg_orders" {
				readers[tb.ID] = true
			}
		}
	}
	for _, want := range []string{
		"ordering/hub_order",          // cited it qualified
		"ordering/sat_order_details",  // cited it qualified
		"ordering/lnk_customer_order", // cited it bare
	} {
		if !readers[want] {
			t.Errorf("%s does not read tpch.v_stg_orders; the mixed-citation case is the point", want)
		}
	}
}
