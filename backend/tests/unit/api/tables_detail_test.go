// The batch table endpoint, which turns four round trips in a tool loop into
// one. The contract that matters is that a batch entry is the same object the
// single endpoint returns, and that a wrong guess at an ID is an answer rather
// than a failed call.
package api_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"urara-vision/backend/internal/model"
	neostore "urara-vision/backend/internal/store/neo4j"
	"urara-vision/backend/internal/store/postgres"
)

// batchMeta is a fake holding two tables, so a request can name one that exists
// and one that does not.
func batchMeta() *fakeMeta {
	return &fakeMeta{
		tablesByID: map[string]*model.Table{
			"domain_one/fact_primary": {ID: "domain_one/fact_primary", Name: "fact_primary"},
			"domain_one/dim_alpha":    {ID: "domain_one/dim_alpha", Name: "dim_alpha"},
		},
		incoming: []postgres.Referrer{{TableID: "domain_one/dim_alpha", Name: "dim_alpha"}},
	}
}

func batchGraphs() *fakeGraphs {
	return &fakeGraphs{
		upstream: []neostore.LineageEntry{{ID: "warehouse.upstream_model", Label: "upstream_model"}},
		siblings: []neostore.LineageEntry{{ID: "other/fact_y", Label: "fact_y"}},
	}
}

// getBatch issues the request and fails the test unless it returned 200.
func getBatch(t *testing.T, h http.Handler, ids string) map[string]any {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/api/v1/snapshots/s1/tables/detail?ids="+ids, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	return decode(t, rec.Body.Bytes())
}

func TestTablesDetailReturnsEachRequestedTable(t *testing.T) {
	meta := batchMeta()
	h := newServer(t, meta, batchGraphs())

	body := getBatch(t, h, "domain_one/fact_primary,domain_one/dim_alpha")

	tables, _ := body["tables"].([]any)
	if len(tables) != 2 {
		t.Fatalf("tables = %v, want 2", body["tables"])
	}
	if missing, _ := body["missing"].([]any); len(missing) != 0 {
		t.Errorf("missing = %v, want empty", body["missing"])
	}
	// The slash in a table ID has to survive the CSV split and reach the store.
	if want := []string{"domain_one/fact_primary", "domain_one/dim_alpha"}; !reflect.DeepEqual(meta.getTableIDs, want) {
		t.Errorf("store queried for %v, want %v", meta.getTableIDs, want)
	}
}

// TestTablesDetailMatchesSingleFetch is the point of the shared helper: one
// definition of what a table detail is, so the batch cannot drift from /table.
func TestTablesDetailMatchesSingleFetch(t *testing.T) {
	const id = "domain_one/fact_primary"
	h := newServer(t, batchMeta(), batchGraphs())

	rec := do(t, h, http.MethodGet, "/api/v1/snapshots/s1/table?id="+id, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("single fetch status = %d: %s", rec.Code, rec.Body)
	}
	single := decode(t, rec.Body.Bytes())

	tables, _ := getBatch(t, h, id)["tables"].([]any)
	if len(tables) != 1 {
		t.Fatalf("tables = %v, want 1", tables)
	}
	batched, _ := tables[0].(map[string]any)

	if !reflect.DeepEqual(batched, single) {
		t.Errorf("batch entry differs from the single fetch\nbatch:  %v\nsingle: %v", batched, single)
	}
}

// TestTablesDetailReportsMissingWithout404: an agent that guessed an ID wrong
// should get the tables that do exist plus the list that did not, rather than a
// failed call it has to reason its way out of.
func TestTablesDetailReportsMissingWithout404(t *testing.T) {
	t.Run("one found, one missing", func(t *testing.T) {
		h := newServer(t, batchMeta(), batchGraphs())
		body := getBatch(t, h, "domain_one/fact_primary,domain_one/nope")

		if tables, _ := body["tables"].([]any); len(tables) != 1 {
			t.Errorf("tables = %v, want 1", body["tables"])
		}
		missing, _ := body["missing"].([]any)
		if len(missing) != 1 || missing[0] != "domain_one/nope" {
			t.Errorf("missing = %v, want the one unknown ID", body["missing"])
		}
	})

	t.Run("all missing", func(t *testing.T) {
		h := newServer(t, batchMeta(), batchGraphs())
		body := getBatch(t, h, "domain_one/nope,domain_one/also_nope")

		tables, ok := body["tables"].([]any)
		if !ok || len(tables) != 0 {
			t.Errorf("tables = %v, want an empty list", body["tables"])
		}
		if missing, _ := body["missing"].([]any); len(missing) != 2 {
			t.Errorf("missing = %v, want 2", body["missing"])
		}
	})
}

// TestTablesDetailEmptiesAreListsNotNull: a caller should not have to tell []
// from null before ranging over either field.
func TestTablesDetailEmptiesAreListsNotNull(t *testing.T) {
	h := newServer(t, batchMeta(), batchGraphs())

	allFound := do(t, h, http.MethodGet,
		"/api/v1/snapshots/s1/tables/detail?ids=domain_one/fact_primary", nil, "")
	if body := allFound.Body.String(); !strings.Contains(body, `"missing":[]`) {
		t.Errorf("missing was not an empty list: %s", body)
	}

	allMissing := do(t, h, http.MethodGet,
		"/api/v1/snapshots/s1/tables/detail?ids=domain_one/nope", nil, "")
	if body := allMissing.Body.String(); !strings.Contains(body, `"tables":[]`) {
		t.Errorf("tables was not an empty list: %s", body)
	}
}

func TestTablesDetailRejectsBadIDCounts(t *testing.T) {
	h := newServer(t, batchMeta(), batchGraphs())

	t.Run("no ids", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/snapshots/s1/tables/detail", nil, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("too many ids", func(t *testing.T) {
		ids := make([]string, 9)
		for i := range ids {
			ids[i] = "domain_one/fact_primary"
		}
		rec := do(t, h, http.MethodGet,
			"/api/v1/snapshots/s1/tables/detail?ids="+strings.Join(ids, ","), nil, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		// The limit is in the message, so a caller can correct itself without
		// having to find the number by bisection.
		if msg, _ := decode(t, rec.Body.Bytes())["error"].(string); !strings.Contains(msg, "8") {
			t.Errorf("error = %q, want it to name the limit of 8", msg)
		}
	})
}

func TestTablesDetailUnknownSnapshotIs404(t *testing.T) {
	meta := batchMeta()
	meta.errGetSnapshot = postgres.ErrNotFound
	h := newServer(t, meta, batchGraphs())

	rec := do(t, h, http.MethodGet,
		"/api/v1/snapshots/nope/tables/detail?ids=domain_one/fact_primary", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
