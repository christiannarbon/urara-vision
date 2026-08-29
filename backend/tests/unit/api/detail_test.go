// The table detail endpoint, which assembles one response from four store
// calls.
package api_test

import (
	"net/http"
	"testing"

	"urara-vision/backend/internal/model"
	neostore "urara-vision/backend/internal/store/neo4j"
	"urara-vision/backend/internal/store/postgres"
)

// TestGetTableAggregates: the detail pane is one request, assembled from four
// store calls. A missing piece would leave the pane half-populated.
func TestGetTableAggregates(t *testing.T) {
	meta := &fakeMeta{
		table:    &model.Table{ID: "domain_one/fact_primary", Name: "fact_primary"},
		incoming: []postgres.Referrer{{TableID: "domain_one/dim_alpha", Name: "dim_alpha"}},
	}
	graphs := &fakeGraphs{
		upstream: []neostore.LineageEntry{{ID: "warehouse.upstream_model", Label: "upstream_model"}},
		siblings: []neostore.LineageEntry{{ID: "other/fact_y", Label: "fact_y"}},
	}
	h := newServer(t, meta, graphs)

	rec := do(t, h, http.MethodGet, "/api/v1/snapshots/s1/table?id=domain_one/fact_primary", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	body := decode(t, rec.Body.Bytes())
	for _, key := range []string{"table", "incoming", "upstream", "siblings"} {
		if body[key] == nil {
			t.Errorf("response is missing %q", key)
		}
	}
	if meta.getTableID != "domain_one/fact_primary" {
		t.Errorf("store queried for %q; the slash in the ID did not survive", meta.getTableID)
	}
	if graphs.upstreamID != "domain_one/fact_primary" {
		t.Errorf("lineage queried for %q", graphs.upstreamID)
	}
}
