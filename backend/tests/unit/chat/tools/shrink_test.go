// Ported from chat/tests/unit/test_tools.py. Argument bounds are 18.2.
package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/tools"
	"urara-vision/backend/internal/model"
)

// fakeBackend returns what it is given and records each call.
type fakeBackend struct {
	domains     []model.Domain
	tables      []apiclient.TableSummary
	detail      apiclient.TablesDetail
	hits        []apiclient.SearchHit
	graph       apiclient.Graph
	paths       []apiclient.JoinPath
	lineage     []apiclient.LineageEntry
	diagnostics []model.Diagnostic
	sources     []model.SourceTable

	snapshots []string
	calls     []string
}

func (f *fakeBackend) record(sid, call string, args ...any) {
	f.snapshots = append(f.snapshots, sid)
	f.calls = append(f.calls, fmt.Sprint(append([]any{call}, args...)...))
}

func (f *fakeBackend) Domains(_ context.Context, sid string) ([]model.Domain, error) {
	f.record(sid, "domains")
	return f.domains, nil
}

func (f *fakeBackend) Tables(_ context.Context, sid, domain string) ([]apiclient.TableSummary, error) {
	f.record(sid, "tables", " ", domain)
	return f.tables, nil
}

func (f *fakeBackend) TablesDetail(_ context.Context, sid string, ids []string) (apiclient.TablesDetail, error) {
	f.record(sid, "tables_detail", ids)
	return f.detail, nil
}

func (f *fakeBackend) Search(_ context.Context, sid, query string, limit int) ([]apiclient.SearchHit, error) {
	f.record(sid, "search", " ", query, " ", limit)
	return f.hits, nil
}

func (f *fakeBackend) Neighbourhood(_ context.Context, sid, tableID string, depth int, sources bool) (apiclient.Graph, error) {
	f.record(sid, "neighbourhood", " ", tableID, " ", depth, " ", sources)
	return f.graph, nil
}

func (f *fakeBackend) JoinPaths(_ context.Context, sid, from, to string, maxDepth, limit int) ([]apiclient.JoinPath, error) {
	f.record(sid, "join_paths", " ", from, " ", to, " ", maxDepth, " ", limit)
	return f.paths, nil
}

func (f *fakeBackend) Lineage(_ context.Context, sid, tableID, direction string) ([]apiclient.LineageEntry, error) {
	f.record(sid, "lineage", " ", tableID, " ", direction)
	return f.lineage, nil
}

func (f *fakeBackend) Diagnostics(_ context.Context, sid, severity string) ([]model.Diagnostic, error) {
	f.record(sid, "diagnostics", " ", severity)
	return f.diagnostics, nil
}

func (f *fakeBackend) Sources(_ context.Context, sid string) ([]model.SourceTable, error) {
	f.record(sid, "sources")
	return f.sources, nil
}

// run calls one tool and returns its result as decoded JSON.
func run(t *testing.T, f *fakeBackend, name, args string) map[string]any {
	t.Helper()
	for _, s := range tools.Build(f, "snap-1") {
		if s.Name != name {
			continue
		}
		var raw json.RawMessage
		if args != "" {
			raw = json.RawMessage(args)
		}
		out, err := s.Run(context.Background(), raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, err := json.Marshal(out)
		if err != nil {
			t.Fatalf("%s result is not JSON: %v", name, err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s did not return an object: %s", name, b)
		}
		return m
	}
	t.Fatalf("no tool %s", name)
	return nil
}

func fixture(t *testing.T, name string, dst any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func items(t *testing.T, result map[string]any) []any {
	t.Helper()
	it, ok := result["items"].([]any)
	if !ok {
		t.Fatalf("items = %v", result["items"])
	}
	return it
}

// A silently cut list teaches the model something false.
func TestTruncationIsAlwaysReported(t *testing.T) {
	var many []model.Domain
	for i := range 137 {
		many = append(many, model.Domain{ID: fmt.Sprintf("d%d", i), Title: fmt.Sprintf("D%d", i)})
	}
	result := run(t, &fakeBackend{domains: many}, "list_domains", "")
	if len(items(t, result)) != tools.MaxItems || result["truncated"] != true || result["total"] != 137.0 {
		t.Errorf("137: %d items, truncated %v, total %v", len(items(t, result)), result["truncated"], result["total"])
	}

	result = run(t, &fakeBackend{domains: many[:51]}, "list_domains", "")
	if len(items(t, result)) != 50 || result["truncated"] != true || result["total"] != 51.0 {
		t.Errorf("51: %d items, truncated %v, total %v", len(items(t, result)), result["truncated"], result["total"])
	}

	result = run(t, &fakeBackend{domains: many[:3]}, "list_domains", "")
	if len(items(t, result)) != 3 || result["truncated"] != false || result["total"] != 3.0 {
		t.Errorf("3: %v", result)
	}

	if result := run(t, &fakeBackend{}, "list_domains", ""); !reflect.DeepEqual(result, map[string]any{
		"items": []any{}, "truncated": false, "total": 0.0,
	}) {
		t.Errorf("empty: %v", result)
	}
}

func detail(description string) apiclient.TablesDetail {
	return apiclient.TablesDetail{Tables: []apiclient.TableDetail{{Table: model.Table{
		ID: "ordering/fact_orders", SnapshotID: "snap-1", Name: "fact_orders", DomainID: "ordering",
		DocPath: "ordering/fact_orders.md", Grain: "One row per order.",
		Columns: []model.Column{{Name: "order_id", Type: "int", Description: description, IsPK: true}},
	}}}}
}

func firstTable(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	return items(t, result)[0].(map[string]any)["table"].(map[string]any)
}

func TestDocPathAndSnapshotAreDropped(t *testing.T) {
	d := detail("")
	d.Tables[0].Incoming = []apiclient.Referrer{{TableID: "a/b", Name: "b"}}
	b, _ := json.Marshal(run(t, &fakeBackend{detail: d}, "get_tables", `{"ids":["ordering/fact_orders"]}`))
	if strings.Contains(string(b), "docPath") || strings.Contains(string(b), "snapshotId") {
		t.Errorf("result = %s", b)
	}
}

func TestALongColumnDescriptionIsCut(t *testing.T) {
	result := run(t, &fakeBackend{detail: detail(strings.Repeat("x", 500))}, "get_tables", `{"ids":["ordering/fact_orders"]}`)
	desc := firstTable(t, result)["columns"].([]any)[0].(map[string]any)["description"].(string)
	if utf8.RuneCountInString(desc) != 301 || !strings.HasSuffix(desc, "…") {
		t.Errorf("%d characters: %q…", utf8.RuneCountInString(desc), desc[:10])
	}

	// Counted in characters, not bytes.
	ja := strings.Repeat("注", 301)
	result = run(t, &fakeBackend{detail: detail(ja)}, "get_tables", `{"ids":["ordering/fact_orders"]}`)
	desc = firstTable(t, result)["columns"].([]any)[0].(map[string]any)["description"].(string)
	if desc != strings.Repeat("注", 300)+"…" {
		t.Errorf("Japanese cut to %d characters", utf8.RuneCountInString(desc))
	}
	if got := tools.TruncateRunes(strings.Repeat("注", 300), 300); got != strings.Repeat("注", 300) {
		t.Errorf("300 characters were cut")
	}
}

func TestEmptyFieldsAreDropped(t *testing.T) {
	table := firstTable(t, run(t, &fakeBackend{detail: detail("")}, "get_tables", `{"ids":["ordering/fact_orders"]}`))
	for _, key := range []string{"layer", "notes", "conformed", "relationships"} {
		if _, ok := table[key]; ok {
			t.Errorf("%s reached the prompt: %v", key, table[key])
		}
	}
	col := table["columns"].([]any)[0].(map[string]any)
	if _, ok := col["description"]; ok || col["isPk"] != true {
		t.Errorf("column = %v", col)
	}
	if _, ok := col["ordinal"]; !ok {
		t.Errorf("a zero ordinal was dropped: %v", col)
	}
}

// In Python False == 0, so a naive zero check keeps every false flag.
func TestPruneDropsFalseAndKeepsZero(t *testing.T) {
	got := tools.Prune(map[string]any{
		"conformed": false, "ordinal": 0, "rank": 0.0, "count": json.Number("0"), "name": "x", "empty": "",
		"none": nil, "list": []any{}, "map": map[string]any{}, "nested": map[string]any{"flag": false},
		"keep": []any{"", false},
	})
	want := map[string]any{"ordinal": 0, "rank": 0.0, "count": json.Number("0"), "name": "x", "keep": []any{"", false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Prune = %v", got)
	}
}

// An absent key would read as though every ID was found.
func TestMissingIsAlwaysReported(t *testing.T) {
	result := run(t, &fakeBackend{detail: apiclient.TablesDetail{Missing: []string{"nope/missing"}}}, "get_tables", `{"ids":["nope/missing"]}`)
	if !reflect.DeepEqual(result["missing"], []any{"nope/missing"}) {
		t.Errorf("missing = %v", result["missing"])
	}
	result = run(t, &fakeBackend{detail: detail("")}, "get_tables", `{"ids":["ordering/fact_orders"]}`)
	if m, ok := result["missing"]; !ok || !reflect.DeepEqual(m, []any{}) {
		t.Errorf("empty missing = %v, present %v", m, ok)
	}
}

func TestNeighbourhoodReturnsIDsAndJoinsNotCanvasData(t *testing.T) {
	g := apiclient.Graph{
		Nodes: []apiclient.Node{
			{ID: "a/one", Label: "one", Kind: "fact", Degree: 9, Refs: 4},
			{ID: "b/two", Label: "two", Kind: "dimension", Degree: 2},
		},
		Links: []apiclient.Link{{ID: "l1", Source: "a/one", Target: "b/two", FromColumn: "x"}},
	}
	result := run(t, &fakeBackend{graph: g}, "get_neighbourhood", `{"table_id":"a/one"}`)
	if n := items(t, result); len(n) != 2 || n[0].(map[string]any)["id"] != "a/one" || n[1].(map[string]any)["id"] != "b/two" {
		t.Errorf("nodes = %v", n)
	}
	b, _ := json.Marshal(result)
	if strings.Contains(string(b), "degree") || strings.Contains(string(b), "refs") {
		t.Errorf("canvas data reached the prompt: %s", b)
	}
	if link := result["links"].([]any)[0].(map[string]any); link["fromColumn"] != "x" {
		t.Errorf("link = %v", link)
	}
}

// A link to a cut node names a table the model cannot see.
func TestLinksToDroppedNodesAreRemoved(t *testing.T) {
	var g apiclient.Graph
	for i := range 60 {
		g.Nodes = append(g.Nodes, apiclient.Node{ID: fmt.Sprintf("d/t%d", i), Label: fmt.Sprintf("t%d", i)})
	}
	g.Links = []apiclient.Link{{Source: "d/t0", Target: "d/t59"}, {Source: "d/t0", Target: "d/t1"}}
	result := run(t, &fakeBackend{graph: g}, "get_neighbourhood", `{"table_id":"d/t0"}`)
	if result["truncated"] != true {
		t.Error("not truncated")
	}
	links := result["links"].([]any)
	if len(links) != 1 || links[0].(map[string]any)["target"] != "d/t1" {
		t.Errorf("links = %v", links)
	}
}

func TestJoinPathsReturnHopsWithTheirColumns(t *testing.T) {
	path := apiclient.JoinPath{Length: 1, Tables: []string{"a/one", "b/two"},
		Hops: []apiclient.PathHop{{From: "a/one", To: "b/two", FromColumn: "x", ToColumn: "y"}}}
	result := run(t, &fakeBackend{paths: []apiclient.JoinPath{path}}, "find_join_paths", `{"from_table":"a/one","to_table":"b/two"}`)
	hop := items(t, result)[0].(map[string]any)["hops"].([]any)[0].(map[string]any)
	// Not pruned, as in Python: an empty cardinality is still reported.
	want := map[string]any{"from": "a/one", "to": "b/two", "fromColumn": "x", "toColumn": "y", "cardinality": ""}
	if !reflect.DeepEqual(hop, want) {
		t.Errorf("hop = %v", hop)
	}
}

func TestArgumentsAndDefaultsReachTheBackend(t *testing.T) {
	f := &fakeBackend{}
	for _, c := range []struct{ name, args string }{
		{"list_tables", `{"domain":"ordering"}`},
		{"search_model", `{"query":"orders","limit":5}`},
		{"get_neighbourhood", `{"table_id":"d/t","depth":3}`},
		{"find_join_paths", `{"from_table":"a/x","to_table":"b/y","max_depth":2}`},
		{"get_lineage", `{"table_id":"d/t","direction":"downstream"}`},
		{"list_diagnostics", `{"severity":"error"}`},
		{"search_model", `{"query":"orders"}`},
		{"get_neighbourhood", `{"table_id":"d/t"}`},
		{"find_join_paths", `{"from_table":"a/x","to_table":"b/y"}`},
		{"get_lineage", `{"table_id":"d/t"}`},
		{"list_tables", ``},
	} {
		run(t, f, c.name, c.args)
	}
	want := []string{
		"tables ordering",
		"search orders 5",
		"neighbourhood d/t 3 false",
		"join_paths a/x b/y 2 10",
		"lineage d/t downstream",
		"diagnostics error",
		"search orders 20",
		"neighbourhood d/t 1 false",
		"join_paths a/x b/y 4 10",
		"lineage d/t upstream",
		"tables ",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls =\n%q\nwant\n%q", f.calls, want)
	}
}

func TestLineageReportsItsDirectionAndTable(t *testing.T) {
	f := &fakeBackend{lineage: []apiclient.LineageEntry{{ID: "src.model"}}}
	result := run(t, f, "get_lineage", `{"table_id":"d/t","direction":"downstream"}`)
	if result["direction"] != "downstream" || result["tableId"] != "d/t" {
		t.Errorf("result = %v", result)
	}
	if result := run(t, f, "get_lineage", `{"table_id":"d/t"}`); result["direction"] != "upstream" {
		t.Errorf("default direction = %v", result["direction"])
	}
}

// Every tool over the captured backend responses returns the uniform object.
func TestEveryToolOverTheFixtures(t *testing.T) {
	f := &fakeBackend{}
	var domains struct{ Domains []model.Domain }
	var tables struct{ Tables []apiclient.TableSummary }
	var hits struct{ Hits []apiclient.SearchHit }
	var paths struct{ Paths []apiclient.JoinPath }
	var lineage struct{ Entries []apiclient.LineageEntry }
	var diags struct{ Diagnostics []model.Diagnostic }
	var sources struct{ Sources []model.SourceTable }
	fixture(t, "domains.json", &domains)
	fixture(t, "tables.json", &tables)
	fixture(t, "tables_detail.json", &f.detail)
	fixture(t, "search.json", &hits)
	fixture(t, "graph.json", &f.graph)
	fixture(t, "paths.json", &paths)
	fixture(t, "lineage.json", &lineage)
	fixture(t, "diagnostics.json", &diags)
	fixture(t, "sources.json", &sources)
	f.domains, f.tables, f.hits, f.paths = domains.Domains, tables.Tables, hits.Hits, paths.Paths
	f.lineage, f.diagnostics, f.sources = lineage.Entries, diags.Diagnostics, sources.Sources

	args := map[string]string{
		"get_tables":        `{"ids":["ordering/fact_orders","nope/missing"]}`,
		"search_model":      `{"query":"orders"}`,
		"get_neighbourhood": `{"table_id":"ordering/fact_orders"}`,
		"find_join_paths":   `{"from_table":"ordering/fact_order_items","to_table":"customer_identity/dim_customers"}`,
		"get_lineage":       `{"table_id":"ordering/fact_orders"}`,
	}
	for _, name := range tools.Names() {
		result := run(t, f, name, args[name])
		if len(items(t, result)) == 0 || result["total"] == nil {
			t.Errorf("%s over its fixture = %v", name, result)
		}
		b, _ := json.Marshal(result)
		if strings.Contains(string(b), "docPath") || strings.Contains(string(b), `"degree"`) {
			t.Errorf("%s leaks backend-only fields", name)
		}
	}
}
