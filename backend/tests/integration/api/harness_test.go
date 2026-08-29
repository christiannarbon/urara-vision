//go:build integration

// Full-stack integration tests: the real router over both real stores.
//
// Everything goes in through HTTP and comes back out through HTTP, so what is
// under test is the wiring. The handlers' own logic is covered by the unit
// suite, which needs no database.
//
// Run with `make test-integration`; without both a DSN and a URI they skip.
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"urara-vision/backend/internal/api"
	"urara-vision/backend/internal/config"
	"urara-vision/backend/tests/fixtures"
	"urara-vision/backend/tests/integration/harness"
)

// stack starts a test server over both real stores and returns its base URL.
func stack(t *testing.T) string {
	t.Helper()
	pg := harness.Postgres(t)
	gs := harness.Neo4j(t)

	cfg := &config.Config{
		CORSOrigins:    []string{"http://localhost:5173"},
		MaxUploadBytes: 64 << 20,
		MaxFiles:       5000,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(api.New(cfg, pg, gs, log).Routes())
	t.Cleanup(srv.Close)
	return srv.URL
}

// get issues a GET and decodes the JSON body, failing on a non-200.
func get(t *testing.T, base, path string) map[string]any {
	t.Helper()
	res, err := http.Get(base + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, res.StatusCode, body)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("GET %s returned non-JSON: %v\n%s", path, err, body)
	}
	return out
}

// ingest uploads the star-schema fixture and returns the new snapshot ID. The
// snapshot is deleted through the API when the test ends, so a failure leaves
// nothing behind.
func ingest(t *testing.T, base string) string {
	t.Helper()

	type file struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	req := struct {
		Name        string `json:"name"`
		SourceLabel string `json:"sourceLabel"`
		Files       []file `json:"files"`
	}{Name: "integration", SourceLabel: "fixtures"}
	for _, f := range fixtures.StarSchema() {
		req.Files = append(req.Files, file{Path: f.Path, Content: f.Content})
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	res, err := http.Post(base+"/api/v1/ingest", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /ingest: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /ingest = %d: %s", res.StatusCode, raw)
	}

	var out struct {
		Snapshot struct {
			ID    string `json:"id"`
			Stats struct {
				Tables        int `json:"tables"`
				Relationships int `json:"relationships"`
			} `json:"stats"`
		} `json:"snapshot"`
		Edges int `json:"edges"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("ingest response: %v\n%s", err, raw)
	}
	if out.Snapshot.ID == "" {
		t.Fatalf("ingest returned no snapshot ID: %s", raw)
	}
	if out.Snapshot.Stats.Tables != 3 {
		t.Errorf("stats.tables = %d, want 3", out.Snapshot.Stats.Tables)
	}
	if out.Edges == 0 {
		t.Error("ingest projected no edges")
	}

	sid := out.Snapshot.ID
	t.Cleanup(func() { deleteSnapshot(t, base, sid) })
	return sid
}

func deleteSnapshot(t *testing.T, base, sid string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, base+"/api/v1/snapshots/"+sid, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("DELETE snapshot %s: %v", sid, err)
		return
	}
	defer func() { _ = res.Body.Close() }()
	// 404 is fine: a test may have deleted it already.
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusNotFound {
		t.Errorf("DELETE snapshot %s = %d", sid, res.StatusCode)
	}
}
