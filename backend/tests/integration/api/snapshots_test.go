//go:build integration

// Snapshot lifecycle over the real stores: the "latest" alias, repeated
// ingests, and a delete reaching both stores.
package api_test

import (
	"net/http"
	"testing"
)

// TestLatestAliasOverTheRealStore: the UI deep-links to "latest", so the alias
// has to resolve against whatever was ingested last.
func TestLatestAliasOverTheRealStore(t *testing.T) {
	base := stack(t)
	sid := ingest(t, base)

	body := get(t, base, "/api/v1/snapshots/latest")
	if body["id"] != sid {
		t.Errorf("latest resolved to %v, want the snapshot just ingested (%s)", body["id"], sid)
	}
}

// TestReingestReplacesNothing: a second ingest of the same documents is a
// separate snapshot, so the previous one stays readable.
func TestReingestCreatesASeparateSnapshot(t *testing.T) {
	base := stack(t)
	first := ingest(t, base)
	second := ingest(t, base)

	if first == second {
		t.Fatal("two ingests produced the same snapshot ID")
	}
	// Both remain readable.
	for _, sid := range []string{first, second} {
		body := get(t, base, "/api/v1/snapshots/"+sid)
		if body["id"] != sid {
			t.Errorf("snapshot %s did not read back", sid)
		}
	}
}

// TestDeleteRemovesItFromBothStores, so the graph does not answer for a
// snapshot the record of truth no longer has.
func TestDeleteRemovesItFromBothStores(t *testing.T) {
	base := stack(t)
	sid := ingest(t, base)

	deleteSnapshot(t, base, sid)

	res, err := http.Get(base + "/api/v1/snapshots/" + sid)
	if err != nil {
		t.Fatalf("GET after delete: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("GET after delete = %d, want 404", res.StatusCode)
	}

	// The graph is scoped by snapshot ID, so a stale projection would still
	// answer here even though the snapshot is gone.
	gres, err := http.Get(base + "/api/v1/snapshots/" + sid + "/graph")
	if err != nil {
		t.Fatalf("GET graph after delete: %v", err)
	}
	defer func() { _ = gres.Body.Close() }()
	if gres.StatusCode != http.StatusNotFound {
		t.Errorf("GET graph after delete = %d, want 404", gres.StatusCode)
	}
}

// TestUnknownSnapshotOverTheRealStore is a 404, not an empty model.
func TestUnknownSnapshotOverTheRealStore(t *testing.T) {
	base := stack(t)
	res, err := http.Get(base + "/api/v1/snapshots/definitely-not-a-snapshot/graph")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}
