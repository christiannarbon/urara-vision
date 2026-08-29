//go:build integration

// Reading a saved model back: domains and table lists, and the full detail one
// pane shows.
package postgres_test

import (
	"errors"
	"testing"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/integration/harness"
)

func TestListDomains(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	domains, err := pg.ListDomains(ctx, m.Snapshot.ID)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	if len(domains) != len(m.Domains) {
		t.Fatalf("domains = %d, want %d", len(domains), len(m.Domains))
	}

	byID := map[string]model.Domain{}
	for _, d := range domains {
		byID[d.ID] = d
	}
	domain_one, ok := byID["domain_one"]
	if !ok {
		t.Fatalf("no domain_one domain in %v", byID)
	}
	if domain_one.Description == "" {
		t.Error("domain description was not persisted")
	}
	if domain_one.TableCount == 0 {
		t.Error("domain table_count was not persisted")
	}
	if len(domain_one.Lineage) == 0 {
		t.Error("domain lineage JSON was not persisted")
	}
}

func TestListTablesAndDomainFilter(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	all, err := pg.ListTables(ctx, m.Snapshot.ID, "")
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	if len(all) != len(m.Tables) {
		t.Fatalf("tables = %d, want %d", len(all), len(m.Tables))
	}

	var fact postgres.TableSummary
	for _, tb := range all {
		if tb.ID == "domain_one/fact_primary" {
			fact = tb
		}
	}
	if fact.ID == "" {
		t.Fatalf("fact_primary missing from %+v", all)
	}
	if fact.Kind != model.KindFact {
		t.Errorf("kind = %q, want fact", fact.Kind)
	}
	if fact.ColumnCount != 3 {
		t.Errorf("columnCount = %d, want 3", fact.ColumnCount)
	}
	if fact.Grain == "" {
		t.Error("grain was not persisted")
	}

	domain_one, err := pg.ListTables(ctx, m.Snapshot.ID, "domain_one")
	if err != nil {
		t.Fatalf("ListTables(domain_one): %v", err)
	}
	if len(domain_one) != 2 {
		t.Errorf("domain_one tables = %d, want 2", len(domain_one))
	}
	for _, tb := range domain_one {
		if tb.DomainID != "domain_one" {
			t.Errorf("%s is in domain %q, which the filter should have excluded", tb.ID, tb.DomainID)
		}
	}
}

// TestGetTableReturnsFullDetail: the detail pane needs every child collection,
// each stored in its own table, so this is where a missing join shows up.
func TestGetTableReturnsFullDetail(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	tb, err := pg.GetTable(ctx, m.Snapshot.ID, "domain_one/fact_primary")
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	if tb.Name != "fact_primary" || tb.Kind != model.KindFact {
		t.Errorf("name/kind = %q/%q", tb.Name, tb.Kind)
	}
	if tb.Grain == "" || tb.UpdateFrequency == "" || tb.Description == "" {
		t.Errorf("overview fields lost: grain %q, frequency %q, description %q",
			tb.Grain, tb.UpdateFrequency, tb.Description)
	}
	if len(tb.Columns) != 3 {
		t.Fatalf("columns = %d, want 3", len(tb.Columns))
	}
	// Columns come back in document order, and the key flags survive.
	if tb.Columns[0].Name != "primary_id" {
		t.Errorf("first column = %q, want primary_id; ordinal was not preserved", tb.Columns[0].Name)
	}
	if !tb.Columns[0].IsPK {
		t.Error("primary_id lost its PK flag")
	}
	if len(tb.ColumnLineage) != 1 {
		t.Errorf("lineage rows = %d, want 1", len(tb.ColumnLineage))
	}
	if len(tb.Relationships) != 2 {
		t.Fatalf("relationships = %d, want 2", len(tb.Relationships))
	}
	for _, r := range tb.Relationships {
		if r.ToTableID == "" {
			t.Errorf("relationship to %q was stored unresolved", r.TargetRef)
		}
		if r.Cardinality == "" {
			t.Errorf("relationship %s lost its cardinality", r.ID)
		}
	}
	if len(tb.Notes) != 1 {
		t.Errorf("notes = %v, want the one caveat", tb.Notes)
	}
}

func TestGetTableNotFound(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	if _, err := pg.GetTable(ctx, m.Snapshot.ID, "nope/nope"); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// TestConformedFlagSurvivesTheRoundTrip: the conformed dimension is the one
// piece of cross-domain resolution the detail pane shows.
func TestConformedFlagSurvivesTheRoundTrip(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	tb, err := pg.GetTable(ctx, m.Snapshot.ID, "domain_two/dim_beta")
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	if !tb.Conformed {
		t.Error("dim_beta came back unconformed")
	}
}
