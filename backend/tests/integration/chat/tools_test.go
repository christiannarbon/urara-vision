//go:build integration

// Ported from chat/tests/integration/test_tools_live.py, through /debug/tool.
package chat_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const (
	demoDomains      = 6
	demoTables       = 10
	demoSourceTables = 7
	demoErrors       = 1
	factOrders       = "ordering/fact_orders"
	dimCustomers     = "customer_identity/dim_customers"
)

// One call per tool, with capture.sh's arguments so the golden files apply.
var liveCalls = []struct {
	golden, tool string
	args         map[string]any
}{
	{"tool-list_domains.json", "list_domains", map[string]any{}},
	{"tool-list_tables.json", "list_tables", map[string]any{"domain": "ordering"}},
	{"tool-get_tables.json", "get_tables", map[string]any{"ids": []string{factOrders}}},
	{"tool-search_model.json", "search_model", map[string]any{"query": "customer", "limit": 5}},
	{"tool-get_neighbourhood.json", "get_neighbourhood", map[string]any{"table_id": factOrders}},
	{"tool-find_join_paths.json", "find_join_paths", map[string]any{"from_table": "ordering/fact_order_items", "to_table": dimCustomers}},
	{"tool-get_lineage.json", "get_lineage", map[string]any{"table_id": factOrders}},
	{"tool-list_diagnostics.json", "list_diagnostics", map[string]any{}},
	{"tool-list_source_models.json", "list_source_models", map[string]any{}},
}

func TestTools(t *testing.T) {
	sid := fixture(t)
	call := func(t *testing.T, tool string, args map[string]any) response {
		t.Helper()
		return chatDo(t, "POST", "/debug/tool", "", map[string]any{"snapshotId": sid, "tool": tool, "args": args})
	}
	result := func(t *testing.T, tool string, args map[string]any) map[string]any {
		t.Helper()
		r := call(t, tool, args)
		if r.status != http.StatusOK {
			t.Fatalf("%s: %d %.300s", tool, r.status, r.raw)
		}
		return r.json(t)
	}
	items := func(r map[string]any) []map[string]any {
		raw, _ := r["items"].([]any)
		out := make([]map[string]any, len(raw))
		for i, x := range raw {
			out[i], _ = x.(map[string]any)
		}
		return out
	}

	t.Run("the sweep covers every tool", func(t *testing.T) {
		listing := chatDo(t, "GET", "/debug/tools", "", nil)
		assertGolden(t, "debug/tools.json", listing, goSchema)
		var tools []struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(listing.raw, &tools)
		var names, swept []string
		for _, tool := range tools {
			names = append(names, tool.Name)
		}
		for _, c := range liveCalls {
			swept = append(swept, c.tool)
		}
		slices.Sort(names)
		slices.Sort(swept)
		if !slices.Equal(names, swept) {
			t.Errorf("tools %v, swept %v", names, swept)
		}
	})

	for _, c := range liveCalls {
		t.Run("golden/"+c.tool, func(t *testing.T) {
			r := call(t, c.tool, c.args)
			assertGolden(t, "debug/"+c.golden, r)
			body := r.json(t)
			total, _ := body["total"].(float64)
			if _, ok := body["truncated"].(bool); !ok || (int(total) != len(items(body)) && body["truncated"] != true) {
				t.Errorf("not the uniform shape: %.300s", r.raw)
			}
		})
	}

	t.Run("list_domains", func(t *testing.T) {
		r := result(t, "list_domains", map[string]any{})
		var ids []string
		for _, d := range items(r) {
			ids = append(ids, d["id"].(string))
		}
		if r["total"] != float64(demoDomains) || !slices.Contains(ids, "ordering") ||
			!slices.Contains(ids, "customer_identity") || !slices.Contains(ids, "delivery_logistics") {
			t.Errorf("total %v, ids %v", r["total"], ids)
		}
	})

	t.Run("list_tables", func(t *testing.T) {
		all := result(t, "list_tables", map[string]any{})
		if all["total"] != float64(demoTables) || all["truncated"] != false {
			t.Errorf("total %v truncated %v", all["total"], all["truncated"])
		}
		ordering := result(t, "list_tables", map[string]any{"domain": "ordering"})
		if n := ordering["total"].(float64); n <= 0 || n >= demoTables {
			t.Errorf("ordering total %v", n)
		}
		for _, tbl := range items(ordering) {
			if tbl["domainId"] != "ordering" {
				t.Errorf("table %v", tbl["id"])
			}
		}
	})

	t.Run("get_tables returns a shrunk document", func(t *testing.T) {
		r := result(t, "get_tables", map[string]any{"ids": []string{factOrders}})
		table, _ := items(r)[0]["table"].(map[string]any)
		cols, _ := table["columns"].([]any)
		raw, _ := json.Marshal(r)
		if table["id"] != factOrders || table["grain"] != "One row per order." || len(cols) == 0 ||
			strings.Contains(string(raw), "docPath") {
			t.Errorf("table %.300s", raw)
		}
	})

	t.Run("get_tables reports a missing id", func(t *testing.T) {
		r := call(t, "get_tables", map[string]any{"ids": []string{factOrders, "ordering/nope"}})
		assertGolden(t, "debug/tool-get_tables-missing.json", r)
		body := r.json(t)
		if body["total"] != float64(1) || !slicesEqualAny(body["missing"], "ordering/nope") {
			t.Errorf("total %v missing %v", body["total"], body["missing"])
		}
	})

	t.Run("search_model", func(t *testing.T) {
		r := result(t, "search_model", map[string]any{"query": "orders"})
		found := false
		for _, h := range items(r) {
			found = found || h["tableId"] == factOrders
		}
		if !found {
			t.Error("fact_orders not found")
		}
	})

	t.Run("get_neighbourhood", func(t *testing.T) {
		r := result(t, "get_neighbourhood", map[string]any{"table_id": factOrders, "depth": 1})
		var ids []string
		dimension := false
		for _, n := range items(r) {
			ids = append(ids, n["id"].(string))
			dimension = dimension || n["kind"] == "dimension"
		}
		links, _ := r["links"].([]any)
		if !slices.Contains(ids, factOrders) || len(ids) < 2 || !dimension || len(links) == 0 {
			t.Errorf("ids %v, links %d", ids, len(links))
		}
	})

	t.Run("find_join_paths carries the join columns", func(t *testing.T) {
		r := result(t, "find_join_paths", map[string]any{"from_table": factOrders, "to_table": dimCustomers})
		paths := items(r)
		if len(paths) == 0 {
			t.Fatal("no path")
		}
		hops, _ := paths[0]["hops"].([]any)
		withColumns := false
		for _, h := range hops {
			hop := h.(map[string]any)
			withColumns = withColumns || (hop["fromColumn"] != "" && hop["toColumn"] != "")
		}
		if len(hops) == 0 || !withColumns {
			t.Errorf("hops %v", hops)
		}
	})

	t.Run("get_lineage both directions", func(t *testing.T) {
		up := result(t, "get_lineage", map[string]any{"table_id": factOrders})
		if up["direction"] != "upstream" || len(items(up)) == 0 {
			t.Fatalf("upstream %v", up)
		}
		source := items(up)[0]["id"].(string)
		down := result(t, "get_lineage", map[string]any{"table_id": source, "direction": "downstream"})
		found := false
		for _, e := range items(down) {
			found = found || e["id"] == factOrders
		}
		if down["direction"] != "downstream" || !found {
			t.Errorf("downstream %v", down)
		}
	})

	t.Run("list_diagnostics", func(t *testing.T) {
		if all := result(t, "list_diagnostics", map[string]any{}); all["total"].(float64) <= 0 {
			t.Error("the demo set is built around deliberate flaws")
		}
		errs := result(t, "list_diagnostics", map[string]any{"severity": "error"})
		if errs["total"] != float64(demoErrors) {
			t.Errorf("errors %v", errs["total"])
		}
		for _, d := range items(errs) {
			if d["severity"] != "error" {
				t.Errorf("severity %v", d["severity"])
			}
		}
	})

	t.Run("list_source_models", func(t *testing.T) {
		r := result(t, "list_source_models", map[string]any{})
		if r["total"] != float64(demoSourceTables) {
			t.Errorf("total %v", r["total"])
		}
		for _, s := range items(r) {
			if s["dataset"] == "" || s["refs"].(float64) <= 0 {
				t.Errorf("source %v", s)
			}
		}
	})

	t.Run("errors", func(t *testing.T) {
		assertGolden(t, "debug/tool-unknown.json", call(t, "nope", map[string]any{}))
		assertGolden(t, "debug/tool-bad-args.json", call(t, "search_model", map[string]any{"query": "customer", "limit": 500}))
	})
}

// goSchema is the golden schema as Go sends it (18.1): no title keys, and a
// nullable optional ({"anyOf": [X, null], "default": null}) as plain X.
// Copied from tests/unit/chat/tools/tools_test.go.
func goSchema(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range v {
			if k != "title" {
				out[k] = goSchema(x)
			}
		}
		if anyOf, ok := out["anyOf"].([]any); ok && len(anyOf) == 2 && reflect.DeepEqual(anyOf[1], map[string]any{"type": "null"}) {
			delete(out, "anyOf")
			if out["default"] == nil {
				delete(out, "default")
			}
			for k, x := range anyOf[0].(map[string]any) {
				out[k] = x
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = goSchema(x)
		}
		return out
	}
	return v
}

func slicesEqualAny(v any, want ...string) bool {
	raw, _ := v.([]any)
	if len(raw) != len(want) {
		return false
	}
	for i, x := range raw {
		if x != want[i] {
			return false
		}
	}
	return true
}
