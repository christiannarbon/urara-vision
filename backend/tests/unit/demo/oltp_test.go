// The shipped Sakila 3NF demo documentation set, parsed and resolved.
//
// docs/demo/sakila-oltp-ddd is the set furthest from what this tool was
// originally built for: not a warehouse at all, but the analytics replica of an
// operational schema. There is no fact, no dimension and no measure anywhere in
// it -- entities, two junction tables, a lookup and shared reference data -- and
// it resolves by exactly the same rules as the star sets do.
//
// It also demonstrates something about third normal form that the warehouse
// sets cannot: a well-normalised schema names its foreign keys after the
// primary keys they point at, so join-key orientation is almost never
// ambiguous. See TestOLTPForeignKeysAreNamedAfterTheirTargets.
package demo_test

import (
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
)

const oltpDir = "../../../../docs/demo/sakila-oltp-ddd"

func loadOLTP(t *testing.T) *model.Model { return loadSet(t, oltpDir) }

func TestOLTPStats(t *testing.T) {
	m := loadOLTP(t)
	s := m.Snapshot.Stats
	checkStats(t, []stat{
		{"files parsed", s.FilesParsed, 20},
		{"files skipped", s.FilesSkipped, 0},
		{"domains", s.Domains, 6},
		{"tables", s.Tables, 14},
		{"columns", s.Columns, 72},
		{"conformed instances", s.Conformed, 2},
		{"declared relationships", s.Relationships, 29},
		{"normalised edges", len(graph.Edges(m)), 15},
		{"lineage edges", s.LineageEdges, 70},
		{"source tables", s.SourceTables, 13},
	})
}

// TestOLTPRoles is the point of the set: a model with no warehouse vocabulary
// in it at all.
func TestOLTPRoles(t *testing.T) {
	got := map[model.TableKind]int{}
	for _, tb := range loadOLTP(t).Tables {
		got[tb.Kind]++
	}
	for k, n := range map[model.TableKind]int{
		model.KindEntity:      8,
		model.KindReference:   3, // country, city, and Party's stale copy
		model.KindAssociative: 2,
		model.KindLookup:      1,
	} {
		if got[k] != n {
			t.Errorf("%s tables = %d, want %d", k, got[k], n)
		}
	}
	for _, unwanted := range []model.TableKind{model.KindFact, model.KindDimension, model.KindUnknown} {
		if got[unwanted] != 0 {
			t.Errorf("%d %s tables in an operational schema; the set exists to have none", got[unwanted], unwanted)
		}
	}
}

// TestOLTPAssociativeTablesJoinBothSides checks the shape that makes a junction
// table a junction table. film_actor holds two foreign keys and nothing else,
// and both of them must resolve or the table cannot do its only job.
func TestOLTPAssociativeTablesJoinBothSides(t *testing.T) {
	m := loadOLTP(t)
	edges := map[string]bool{}
	for _, e := range graph.Edges(m) {
		edges[e.From+"->"+e.To] = true
	}
	for _, want := range []string{
		"catalog/film_actor->catalog/film",
		"catalog/film_actor->catalog/actor",
	} {
		if !edges[want] {
			t.Errorf("%s did not resolve; a junction table joined to one side is not a junction table", want)
		}
	}
}

// TestOLTPDiagnostics pins the flaws the sample is built around.
func TestOLTPDiagnostics(t *testing.T) {
	checkDiagnostics(t, loadOLTP(t), map[string]int{
		"unresolved_reference":   1,  // staff, in an unmodelled context
		"cross_domain_reference": 12, // shared geography is read by nearly everything
		"conformed_drift":        1,  // party/country vs the kernel's
		"unmatched_join_key":     1,  // city = city_name, joined on the name not the key
		"empty_domain":           1,  // staffing.md has no directory
		"isolated_table":         1,  // film_category names its parents in prose
		"narrative_reference":    2,  // "Every Address Table", "The Category Tables"
		"undocumented_lineage":   2,  // prose in the Source Table column
	}, 1) // exactly one error, so `relctl -strict` on the demo exits non-zero
}

// TestOLTPIsolatedAssociativeIsReportedLikeAnIsolatedFact is the generalisation
// working on a third vocabulary. The Data Vault set has a link joined to
// nothing and the star sets have a fact joined to nothing; here it is a
// junction table, and the check must treat all three the same way.
func TestOLTPIsolatedAssociativeIsReportedLikeAnIsolatedFact(t *testing.T) {
	m := loadOLTP(t)
	for _, d := range m.Diagnostics {
		if d.Code != "isolated_table" {
			continue
		}
		if d.TableID != "catalog/film_category" {
			t.Errorf("isolated_table on %s, want catalog/film_category", d.TableID)
		}
		if d.Message != "Associative table has no resolvable relationship to any other table." {
			t.Errorf("message = %q; it should name the role it actually found", d.Message)
		}
		return
	}
	t.Error("no isolated_table diagnostic; film_category names its parents in prose and should produce one")
}

// TestOLTPLookupIsNotConnective checks the other half of the same rule. A
// lookup with nothing pointing at it would be dead weight, not a broken join,
// so `language` must not produce a diagnostic the way `film_category` does.
func TestOLTPLookupIsNotConnective(t *testing.T) {
	for _, d := range loadOLTP(t).Diagnostics {
		if d.TableID == "catalog/language" {
			t.Errorf("language produced %s; a lookup is not a connective table", d.Code)
		}
	}
}

// TestOLTPForeignKeysAreNamedAfterTheirTargets records a property of third
// normal form the warehouse sets cannot show. Sakila names every foreign key
// after the primary key it points at -- `rental.inventory_id` at
// `inventory.inventory_id` -- so the order a join key is written in carries no
// information and none is needed.
//
// Exactly one join in the set is written with different names on each side, and
// it is the deliberate mistake: joining geography on the name rather than the
// key. That is not a coincidence worth losing, so it is pinned.
func TestOLTPForeignKeysAreNamedAfterTheirTargets(t *testing.T) {
	m := loadOLTP(t)
	var mismatched []string
	for _, tb := range m.Tables {
		for _, r := range tb.Relationships {
			if r.FromColumn != "" && r.ToColumn != "" && r.FromColumn != r.ToColumn {
				mismatched = append(mismatched, tb.ID+" -> "+r.TargetRef+": "+r.JoinKeyRaw)
			}
		}
	}
	if len(mismatched) != 1 {
		t.Fatalf("joins named differently on each side = %v, want only the deliberate city-by-name mistake", mismatched)
	}
	if mismatched[0] != "party/address -> city: city = city_name" {
		t.Errorf("got %q, want the address-to-city join", mismatched[0])
	}
}

// TestOLTPTwoSidedDeclarationsCollapse checks the eleven joins the sample
// declares from both sides. An operational schema declares nearly everything
// from both sides, because every foreign key is a fact about both tables.
//
// party/address->shared_kernel/city is declared from both sides too and is
// deliberately absent: the address document writes the key on the name rather
// than the key, so the two declarations do not merge. Same case as the hybrid
// set's dim_product.
func TestOLTPTwoSidedDeclarationsCollapse(t *testing.T) {
	checkTwoSided(t, loadOLTP(t), map[string][]string{
		"catalog/film->catalog/language":            {"catalog/film", "catalog/language"},
		"catalog/film_actor->catalog/actor":         {"catalog/actor", "catalog/film_actor"},
		"catalog/film_actor->catalog/film":          {"catalog/film", "catalog/film_actor"},
		"inventory/inventory->catalog/film":         {"catalog/film", "inventory/inventory"},
		"inventory/inventory->inventory/store":      {"inventory/inventory", "inventory/store"},
		"party/customer->inventory/store":           {"inventory/store", "party/customer"},
		"party/customer->party/address":             {"party/address", "party/customer"},
		"rental/payment->rental/rental":             {"rental/payment", "rental/rental"},
		"rental/rental->inventory/inventory":        {"inventory/inventory", "rental/rental"},
		"rental/rental->party/customer":             {"party/customer", "rental/rental"},
		"shared_kernel/city->shared_kernel/country": {"shared_kernel/city", "shared_kernel/country"},
	})
}

// TestOLTPSourceCanonicalisation checks the sample's inconsistent citation of
// one replicated table folds onto a single lineage node.
func TestOLTPSourceCanonicalisation(t *testing.T) {
	m := loadOLTP(t)
	for _, s := range m.SourceTables {
		if s.ID == "raw_actor" {
			t.Errorf("bare raw_actor survived as its own source node; it should fold onto sakila.raw_actor")
		}
		if s.Dataset != "sakila" {
			t.Errorf("source %q has dataset %q, want every demo source qualified with sakila", s.ID, s.Dataset)
		}
	}
	// catalog/actor writes the model both ways within its own document: bare on
	// the first row, qualified on the rest. Every one of its columns must end up
	// on the qualified node, or the bare rows are lost from the lineage graph.
	var cited []string
	for _, tb := range m.Tables {
		if tb.ID != "catalog/actor" {
			continue
		}
		for _, l := range tb.ColumnLineage {
			cited = append(cited, l.SourceTable)
		}
	}
	if len(cited) != 4 {
		t.Fatalf("catalog/actor cites %d sources, want 4", len(cited))
	}
	for _, c := range cited {
		if c != "sakila.raw_actor" {
			t.Errorf("catalog/actor cites %q; every spelling should fold onto sakila.raw_actor", c)
		}
	}
}
