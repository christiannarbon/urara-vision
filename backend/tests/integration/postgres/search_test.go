//go:build integration

// The full-text index: prefix matching as the reader types, what a hit reports
// about itself, and the bounds on a query.
package postgres_test

import (
	"testing"

	"urara-vision/backend/tests/integration/harness"
)

// TestSearchIsAPrefixMatch: the search overlay queries as the reader types, so
// a partial word has to match.
func TestSearchIsAPrefixMatch(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	hits, err := pg.Search(ctx, m.Snapshot.ID, "prim", 50)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var found bool
	for _, h := range hits {
		if h.TableID == "domain_one/fact_primary" {
			found = true
		}
	}
	if !found {
		t.Errorf("searching %q did not find fact_primary: %+v", "prim", hits)
	}
}

// TestSearchReportsMatchingColumns, so a hit can explain itself when the table
// name is not what matched.
func TestSearchReportsMatchingColumns(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	hits, err := pg.Search(ctx, m.Snapshot.ID, "beta_id", 50)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("searching a column name found nothing")
	}
	var any bool
	for _, h := range hits {
		if len(h.MatchedOn) > 0 {
			any = true
		}
	}
	if !any {
		t.Errorf("no hit reported which column matched: %+v", hits)
	}
}

// TestSearchEmptyAndUnusableQueries must not error, since both happen while a
// reader is typing.
func TestSearchEmptyAndUnusableQueries(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	for _, q := range []string{"", "   ", "!!!", "%_%"} {
		hits, err := pg.Search(ctx, m.Snapshot.ID, q, 50)
		if err != nil {
			t.Errorf("Search(%q) = %v, want no error", q, err)
		}
		if len(hits) != 0 {
			t.Errorf("Search(%q) returned %d hits, want none", q, len(hits))
		}
	}
}

// TestSearchLimitIsApplied, so a broad query cannot return the whole model.
func TestSearchLimitIsApplied(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	hits, err := pg.Search(ctx, m.Snapshot.ID, "id", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) > 1 {
		t.Errorf("limit 1 returned %d hits", len(hits))
	}
}

// TestSearchIsScopedToItsSnapshot: two snapshots of the same directory must not
// bleed into each other's results.
func TestSearchIsScopedToItsSnapshot(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	a := harness.SavedModel(t, ctx, pg)
	b := harness.SavedModel(t, ctx, pg)

	hits, err := pg.Search(ctx, a.Snapshot.ID, "domain_one", 200)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits at all")
	}
	// Every hit belongs to snapshot a; b is a separate model with the same
	// table names, and a leak would double the result count.
	other, err := pg.Search(ctx, b.Snapshot.ID, "domain_one", 200)
	if err != nil {
		t.Fatalf("Search(b): %v", err)
	}
	if len(hits) != len(other) {
		t.Errorf("snapshot a returned %d hits and b returned %d; the query is not scoped",
			len(hits), len(other))
	}
}
