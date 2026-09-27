package httpapi_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/chat/tools"
	"urara-vision/backend/internal/model"
)

// toolStore answers every tool call from testdata/fixtures, or with toolErr.
type toolStore struct {
	t          *testing.T
	resolveErr error
	toolErr    error
	resolved   []string
	searched   []string
}

func (s *toolStore) load(name string, dst any) error {
	if s.toolErr != nil {
		return s.toolErr
	}
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "fixtures", name))
	if err != nil {
		s.t.Fatal(err)
	}
	return json.Unmarshal(raw, dst)
}

func (s *toolStore) ResolveSnapshot(_ context.Context, sid string) (string, error) {
	s.resolved = append(s.resolved, sid)
	switch {
	case s.resolveErr != nil:
		return "", s.resolveErr
	case sid == "nosuch":
		return "", &apiclient.Error{Status: 404, Message: "snapshot not found"}
	case sid == "latest":
		return "real-id", nil
	}
	return sid, nil
}

func (s *toolStore) Domains(context.Context, string) ([]model.Domain, error) {
	var v struct{ Domains []model.Domain }
	return v.Domains, s.load("domains.json", &v)
}

func (s *toolStore) Tables(context.Context, string, string) ([]apiclient.TableSummary, error) {
	var v struct{ Tables []apiclient.TableSummary }
	return v.Tables, s.load("tables.json", &v)
}

func (s *toolStore) TablesDetail(context.Context, string, []string) (apiclient.TablesDetail, error) {
	var v apiclient.TablesDetail
	return v, s.load("tables_detail.json", &v)
}

func (s *toolStore) Search(_ context.Context, _, query string, limit int) ([]apiclient.SearchHit, error) {
	s.searched = append(s.searched, query, strings.Repeat("|", limit))
	var v struct{ Hits []apiclient.SearchHit }
	return v.Hits, s.load("search.json", &v)
}

func (s *toolStore) Neighbourhood(context.Context, string, string, int, bool) (apiclient.Graph, error) {
	var v apiclient.Graph
	return v, s.load("graph.json", &v)
}

func (s *toolStore) JoinPaths(context.Context, string, string, string, int, int) ([]apiclient.JoinPath, error) {
	var v struct{ Paths []apiclient.JoinPath }
	return v.Paths, s.load("paths.json", &v)
}

func (s *toolStore) Lineage(context.Context, string, string, string) ([]apiclient.LineageEntry, error) {
	var v struct{ Entries []apiclient.LineageEntry }
	return v.Entries, s.load("lineage.json", &v)
}

func (s *toolStore) Diagnostics(context.Context, string, string) ([]model.Diagnostic, error) {
	var v struct{ Diagnostics []model.Diagnostic }
	return v.Diagnostics, s.load("diagnostics.json", &v)
}

func (s *toolStore) Sources(context.Context, string) ([]model.SourceTable, error) {
	var v struct{ Sources []model.SourceTable }
	return v.Sources, s.load("sources.json", &v)
}

func toolServer(store *toolStore) http.Handler {
	return httpapi.New(httpapi.Deps{
		Settings: chatSettings(),
		Log:      slog.New(slog.NewJSONHandler(&logs{}, nil)),
		Tools:    store,
	}).Handler()
}

func invoke(t *testing.T, store *toolStore, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/debug/tool", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return serve(toolServer(store), req)
}

func TestDebugToolsListsNineWithTheirSchemas(t *testing.T) {
	rec := get(toolServer(&toolStore{t: t}), "/debug/tools", "")
	var listed []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Schema      map[string]any `json:"schema"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); rec.Code != 200 || err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var names []string
	for _, tool := range listed {
		names = append(names, tool.Name)
		if tool.Description == "" || tool.Schema["type"] != "object" {
			t.Errorf("%s = %+v", tool.Name, tool)
		}
	}
	if !slices.Equal(names, tools.Names()) {
		t.Errorf("names = %v", names)
	}
	if lower := strings.ToLower(rec.Body.String()); strings.Contains(lower, "snapshot") {
		t.Error("a schema offers a snapshot")
	}
}

// The fixtures hold the same jaffle data these goldens were captured from.
func TestDebugToolMatchesTheGoldens(t *testing.T) {
	for _, c := range []struct{ golden, tool, args string }{
		{"tool-list_domains.json", "list_domains", `{}`},
		{"tool-find_join_paths.json", "find_join_paths", `{"from_table":"ordering/fact_order_items","to_table":"customer_identity/dim_customers"}`},
		{"tool-get_lineage.json", "get_lineage", `{"table_id":"ordering/fact_orders"}`},
		{"tool-list_diagnostics.json", "list_diagnostics", `{}`},
		{"tool-list_source_models.json", "list_source_models", `{}`},
		{"tool-unknown.json", "nope", `{}`},
		{"tool-bad-args.json", "search_model", `{"query":"customer","limit":500}`},
	} {
		rec := invoke(t, &toolStore{t: t}, `{"snapshotId":"snap-1","tool":"`+c.tool+`","args":`+c.args+`}`)
		assertGolden(t, "debug/"+c.golden, rec)
	}
}

func TestDebugToolRunsTheNamedToolWithItsArgs(t *testing.T) {
	store := &toolStore{t: t}
	rec := invoke(t, store, `{"snapshotId":"snap-1","tool":"search_model","args":{"query":"orders","limit":5}}`)
	body := decode(t, rec)
	if rec.Code != 200 || body["items"].([]any)[0].(map[string]any)["tableId"] != "ordering/fact_orders" {
		t.Errorf("%d %v", rec.Code, body)
	}
	if !slices.Equal(store.searched, []string{"orders", "|||||"}) {
		t.Errorf("search got %v", store.searched)
	}
}

func TestDebugToolResolvesLatest(t *testing.T) {
	store := &toolStore{t: t}
	if rec := invoke(t, store, `{"snapshotId":"latest","tool":"list_domains","args":{}}`); rec.Code != 200 {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if !slices.Equal(store.resolved, []string{"latest"}) {
		t.Errorf("resolved %v", store.resolved)
	}
}

func TestDebugToolRefusesBadRequests(t *testing.T) {
	for _, c := range []struct {
		body   string
		status int
		want   string
	}{
		{`{"snapshotId":"snap-1","tool":"get_tables","args":{"ids":[]}}`, 400, "ids"},
		{`{"snapshotId":"snap-1","tool":"get_neighbourhood","args":{"table_id":"d/t","depth":9}}`, 400, "depth"},
		{`{"snapshotId":"snap-1","tool":"search_model","args":{"query":"x","limt":5}}`, 400, "limt"},
		{`{"snapshotId":"snap-1","tool":"list_domains","args":{},"toolz":"x"}`, 400, "toolz"},
		{`{"tool":"list_domains"}`, 400, "snapshotId"},
		{`{"snapshotId":"snap-1"}`, 400, `"tool"`},
		{`{"snapshotId":"nosuch","tool":"list_domains","args":{}}`, 404, "snapshot not found"},
	} {
		rec := invoke(t, &toolStore{t: t}, c.body)
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.want) || rec.Header().Get("X-Request-Id") == "" {
			t.Errorf("%s: %d %s", c.body, rec.Code, rec.Body)
		}
	}
}

func TestDebugToolAcceptsSnakeCaseAndNoArgs(t *testing.T) {
	for _, body := range []string{
		`{"snapshot_id":"snap-1","tool":"list_domains","args":{}}`,
		`{"snapshotId":"snap-1","tool":"list_domains"}`,
	} {
		if rec := invoke(t, &toolStore{t: t}, body); rec.Code != 200 {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
}

// Unguarded, as Python's _run: backend failures are statuses, not advice.
func TestDebugToolMapsBackendErrors(t *testing.T) {
	for _, c := range []struct {
		err    error
		status int
		detail string
	}{
		{&apiclient.Error{Status: 404, Message: "no such table"}, 404, "no such table"},
		{&apiclient.Error{Status: 500, Message: "boom"}, 502, "boom"},
		{&apiclient.Error{Status: 403, Message: "forbidden"}, 502, "forbidden"},
		{unreachableError(t), 502, "backend unreachable"},
	} {
		for _, store := range []*toolStore{{t: t, toolErr: c.err}, {t: t, resolveErr: c.err}} {
			rec := invoke(t, store, `{"snapshotId":"snap-1","tool":"list_domains","args":{}}`)
			if d, _ := decode(t, rec)["detail"].(string); rec.Code != c.status || !strings.Contains(d, c.detail) {
				t.Errorf("%v (resolve %v): %d %s", c.err, store.resolveErr != nil, rec.Code, rec.Body)
			}
		}
	}
}
