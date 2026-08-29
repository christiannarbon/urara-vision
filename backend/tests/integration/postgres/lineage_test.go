//go:build integration

// Reads that run against the grain of what was written: who points at this
// table, who reads this upstream model, and what the ingest observed.
package postgres_test

import (
	"testing"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/tests/integration/harness"
)

// TestIncomingRelationships answers "what joins this dimension?", which is the
// reverse of what the relationships table is indexed for.
func TestIncomingRelationships(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	in, err := pg.IncomingRelationships(ctx, m.Snapshot.ID, "domain_one/dim_alpha")
	if err != nil {
		t.Fatalf("IncomingRelationships: %v", err)
	}
	if len(in) != 1 {
		t.Fatalf("referrers = %+v, want just fact_primary", in)
	}
	if in[0].TableID != "domain_one/fact_primary" || in[0].Name != "fact_primary" {
		t.Errorf("referrer = %+v", in[0])
	}
	if in[0].FromColumn != "alpha_id" || in[0].ToColumn != "alpha_id" {
		t.Errorf("join columns = %s -> %s", in[0].FromColumn, in[0].ToColumn)
	}
}

func TestListSourceTablesAndDownstreamUse(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	sources, err := pg.ListSourceTables(ctx, m.Snapshot.ID)
	if err != nil {
		t.Fatalf("ListSourceTables: %v", err)
	}
	if len(sources) != 1 || sources[0].ID != "warehouse.upstream_model" {
		t.Fatalf("sources = %+v, want the one upstream model", sources)
	}
	if sources[0].Dataset != "warehouse" || sources[0].Name != "upstream_model" {
		t.Errorf("source was not split into dataset and name: %+v", sources[0])
	}

	users, err := pg.DownstreamUse(ctx, m.Snapshot.ID, "warehouse.upstream_model")
	if err != nil {
		t.Fatalf("DownstreamUse: %v", err)
	}
	if len(users) != 1 || users[0] != "domain_one/fact_primary" {
		t.Errorf("downstream = %v, want [domain_one/fact_primary]", users)
	}
}

func TestListDiagnosticsAndSeverityFilter(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	all, err := pg.ListDiagnostics(ctx, m.Snapshot.ID, "")
	if err != nil {
		t.Fatalf("ListDiagnostics: %v", err)
	}
	if len(all) != len(m.Diagnostics) {
		t.Errorf("diagnostics = %d, want %d", len(all), len(m.Diagnostics))
	}

	warnings, err := pg.ListDiagnostics(ctx, m.Snapshot.ID, model.SeverityWarning)
	if err != nil {
		t.Fatalf("ListDiagnostics(warning): %v", err)
	}
	for _, d := range warnings {
		if d.Severity != model.SeverityWarning {
			t.Errorf("severity filter returned a %q diagnostic", d.Severity)
		}
	}
	if len(warnings) > len(all) {
		t.Error("filtered list is larger than the unfiltered one")
	}
}
