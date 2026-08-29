//go:build integration

// Walking the endpoints the UI calls after an upload, in the order it calls
// them.
package api_test

import (
	"io"
	"net/http"
	"testing"
)

func TestReadyzWithRealStores(t *testing.T) {
	base := stack(t)
	res, err := http.Get(base + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("readyz = %d: %s", res.StatusCode, body)
	}
}

// TestIngestThenReadEverything walks the endpoints the UI calls after an
// upload, in the order it calls them.
func TestIngestThenReadEverything(t *testing.T) {
	base := stack(t)
	sid := ingest(t, base)
	snap := "/api/v1/snapshots/" + sid

	t.Run("snapshot", func(t *testing.T) {
		body := get(t, base, snap)
		if body["id"] != sid {
			t.Errorf("id = %v, want %s", body["id"], sid)
		}
	})

	t.Run("domains", func(t *testing.T) {
		domains, ok := get(t, base, snap+"/domains")["domains"].([]any)
		if !ok || len(domains) != 2 {
			t.Errorf("domains = %v, want 2", domains)
		}
	})

	t.Run("tables", func(t *testing.T) {
		tables, ok := get(t, base, snap+"/tables")["tables"].([]any)
		if !ok || len(tables) != 3 {
			t.Errorf("tables = %v, want 3", tables)
		}
		filtered, _ := get(t, base, snap+"/tables?domain=domain_one")["tables"].([]any)
		if len(filtered) != 2 {
			t.Errorf("domain_one tables = %d, want 2", len(filtered))
		}
	})

	t.Run("graph", func(t *testing.T) {
		body := get(t, base, snap+"/graph")
		nodes, _ := body["nodes"].([]any)
		links, _ := body["links"].([]any)
		if len(nodes) != 3 {
			t.Errorf("nodes = %d, want 3", len(nodes))
		}
		if len(links) != 2 {
			t.Errorf("links = %d, want 2", len(links))
		}
	})

	t.Run("graph with sources", func(t *testing.T) {
		nodes, _ := get(t, base, snap+"/graph?sources=true")["nodes"].([]any)
		if len(nodes) != 4 {
			t.Errorf("nodes = %d, want 4 with the source model included", len(nodes))
		}
	})

	t.Run("table detail", func(t *testing.T) {
		body := get(t, base, snap+"/table?id=domain_one/fact_primary")
		for _, key := range []string{"table", "incoming", "upstream", "siblings"} {
			if _, ok := body[key]; !ok {
				t.Errorf("detail response is missing %q", key)
			}
		}
		tb, _ := body["table"].(map[string]any)
		if tb["name"] != "fact_primary" {
			t.Errorf("table.name = %v", tb["name"])
		}
		cols, _ := tb["columns"].([]any)
		if len(cols) != 3 {
			t.Errorf("columns = %d, want 3", len(cols))
		}
		up, _ := body["upstream"].([]any)
		if len(up) != 1 {
			t.Errorf("upstream = %d, want the one source model", len(up))
		}
	})

	t.Run("neighborhood", func(t *testing.T) {
		nodes, _ := get(t, base, snap+"/neighborhood?table=domain_one/dim_alpha&depth=1")["nodes"].([]any)
		if len(nodes) != 2 {
			t.Errorf("nodes = %d, want the dimension and its fact", len(nodes))
		}
	})

	t.Run("paths", func(t *testing.T) {
		paths, _ := get(t, base,
			snap+"/paths?from=domain_one/dim_alpha&to=domain_two/dim_beta")["paths"].([]any)
		if len(paths) == 0 {
			t.Error("no join path found between two dimensions of the same fact")
		}
	})

	t.Run("lineage", func(t *testing.T) {
		up := get(t, base, snap+"/lineage?id=domain_one/fact_primary")
		if up["direction"] != "upstream" {
			t.Errorf("direction = %v", up["direction"])
		}
		if entries, _ := up["entries"].([]any); len(entries) != 1 {
			t.Errorf("upstream entries = %d, want 1", len(entries))
		}

		down := get(t, base,
			snap+"/lineage?id=warehouse.upstream_model&direction=downstream")
		if down["direction"] != "downstream" {
			t.Errorf("direction = %v", down["direction"])
		}
	})

	t.Run("search", func(t *testing.T) {
		hits, _ := get(t, base, snap+"/search?q=prim")["hits"].([]any)
		if len(hits) == 0 {
			t.Error("a prefix search found nothing")
		}
	})

	t.Run("diagnostics", func(t *testing.T) {
		if _, ok := get(t, base, snap+"/diagnostics")["diagnostics"]; !ok {
			t.Error("response has no diagnostics field")
		}
	})

	t.Run("sources", func(t *testing.T) {
		sources, _ := get(t, base, snap+"/sources")["sources"].([]any)
		if len(sources) != 1 {
			t.Errorf("sources = %d, want 1", len(sources))
		}
	})
}
