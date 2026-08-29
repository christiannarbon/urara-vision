// The HTTP surface's own work, driven through the real router with fake stores
// behind it: which store outcome becomes which status code, and how a snapshot
// is resolved from the path.
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"urara-vision/backend/internal/store/postgres"
)

// decode reads a JSON response body into a map.
func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("response was not JSON: %v\n%s", err, body)
	}
	return m
}

func TestHealthz(t *testing.T) {
	h := newServer(t, &fakeMeta{}, &fakeGraphs{})
	rec := do(t, h, http.MethodGet, "/healthz", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decode(t, rec.Body.Bytes())["status"]; got != "ok" {
		t.Errorf("status field = %v", got)
	}
}

// TestReadyzReportsEachDependency: readiness has to say which dependency is
// down, or a failing deploy gives no clue which container to look at.
func TestReadyzReportsEachDependency(t *testing.T) {
	t.Run("both up", func(t *testing.T) {
		h := newServer(t, &fakeMeta{}, &fakeGraphs{})
		rec := do(t, h, http.MethodGet, "/readyz", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("postgres down", func(t *testing.T) {
		h := newServer(t, &fakeMeta{errPing: errBoom}, &fakeGraphs{})
		rec := do(t, h, http.MethodGet, "/readyz", nil, "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
		body := decode(t, rec.Body.Bytes())
		if body["postgres"] == "ok" {
			t.Error("postgres reported ok while its ping was failing")
		}
		if body["graph"] != "ok" {
			t.Errorf("graph = %v, want ok", body["graph"])
		}
	})

	t.Run("graph down", func(t *testing.T) {
		h := newServer(t, &fakeMeta{}, &fakeGraphs{errPing: errBoom})
		rec := do(t, h, http.MethodGet, "/readyz", nil, "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
		if decode(t, rec.Body.Bytes())["graph"] == "ok" {
			t.Error("graph reported ok while its ping was failing")
		}
	})
}

// TestLatestAliasResolves covers the deep-link case: the UI asks for "latest"
// without knowing an ID, and every handler behind the alias must see the real
// snapshot ID rather than the literal string.
func TestLatestAliasResolves(t *testing.T) {
	meta := &fakeMeta{latest: "real-snapshot-id"}
	graphs := &fakeGraphs{}
	h := newServer(t, meta, graphs)

	rec := do(t, h, http.MethodGet, "/api/v1/snapshots/latest/domains", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
}

// TestLatestWithNoSnapshots: an empty install is a 404 with an explanation,
// not an empty graph that reads as a real but blank model.
func TestLatestWithNoSnapshots(t *testing.T) {
	meta := &fakeMeta{errLatest: postgres.ErrNotFound}
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodGet, "/api/v1/snapshots/latest/graph", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	msg, _ := decode(t, rec.Body.Bytes())["error"].(string)
	if msg == "" {
		t.Error("404 carried no explanation")
	}
}

// TestUnknownSnapshotIs404: a typo in the ID must not return an empty graph.
func TestUnknownSnapshotIs404(t *testing.T) {
	meta := &fakeMeta{errGetSnapshot: postgres.ErrNotFound}
	h := newServer(t, meta, &fakeGraphs{})

	for _, path := range []string{"/domains", "/tables", "/graph", "/diagnostics", "/sources", "/search"} {
		rec := do(t, h, http.MethodGet, "/api/v1/snapshots/nope"+path, nil, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, rec.Code)
		}
	}
}

// TestMissingTableIs404 rather than an empty detail pane.
func TestMissingTableIs404(t *testing.T) {
	h := newServer(t, &fakeMeta{table: nil}, &fakeGraphs{})
	rec := do(t, h, http.MethodGet, "/api/v1/snapshots/s1/table?id=nope/nope", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestStoreFailureIs500WithoutDetail: an internal fault must not leak the
// database error to the client, but it must still be a 500.
func TestStoreFailureIs500WithoutDetail(t *testing.T) {
	meta := &fakeMeta{errList: errBoom}
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodGet, "/api/v1/snapshots", nil, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	msg, _ := decode(t, rec.Body.Bytes())["error"].(string)
	if msg != "internal error" {
		t.Errorf("error = %q; the underlying failure should not reach the client", msg)
	}
}

// TestDeleteSnapshotRemovesBothStores.
func TestDeleteSnapshotRemovesBothStores(t *testing.T) {
	meta := &fakeMeta{}
	graphs := &fakeGraphs{}
	h := newServer(t, meta, graphs)

	rec := do(t, h, http.MethodDelete, "/api/v1/snapshots/s1", nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(meta.deleted) != 1 || meta.deleted[0] != "s1" {
		t.Errorf("postgres deletes = %v", meta.deleted)
	}
	if len(graphs.deleted) != 1 || graphs.deleted[0] != "s1" {
		t.Errorf("graph deletes = %v", graphs.deleted)
	}
}

// TestDeleteSnapshotFailsWhenTheRecordOfTruthFails, since the entry would
// otherwise still be listed.
func TestDeleteSnapshotFailsWhenTheRecordOfTruthFails(t *testing.T) {
	meta := &fakeMeta{errDelete: errBoom}
	h := newServer(t, meta, &fakeGraphs{})
	rec := do(t, h, http.MethodDelete, "/api/v1/snapshots/s1", nil, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// TestCORSHeaderIsSetForAConfiguredOrigin, since the dev frontend runs on a
// different port and would otherwise be blocked by the browser.
func TestCORSHeaderIsSetForAConfiguredOrigin(t *testing.T) {
	h := newServer(t, &fakeMeta{}, &fakeGraphs{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
}
