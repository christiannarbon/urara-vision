// Ported from chat/tests/unit/test_client.py and test_models.py.
package apiclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/reqctx"
	"urara-vision/backend/internal/model"
)

const (
	sid     = "snap-1"
	tableID = "ordering/fact_orders"
)

// fake answers every request with one status and body, and records what it got.
type fake struct {
	mu       sync.Mutex
	status   int
	body     string
	requests []*http.Request
	bodies   []string
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.requests = append(f.requests, r)
	f.bodies = append(f.bodies, string(raw))
	f.mu.Unlock()
	if f.status != http.StatusNoContent {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(f.status)
	_, _ = io.WriteString(w, f.body)
}

func (f *fake) last(t *testing.T) *http.Request {
	t.Helper()
	if len(f.requests) == 0 {
		t.Fatal("no request reached the backend")
	}
	return f.requests[len(f.requests)-1]
}

// lastBody decodes the last request body as JSON.
func (f *fake) lastBody(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(f.bodies[len(f.bodies)-1]), &m); err != nil {
		t.Fatalf("request body %q: %v", f.bodies[len(f.bodies)-1], err)
	}
	return m
}

func serve(t *testing.T, status int, body string) (*apiclient.Client, *fake) {
	return serveWithToken(t, status, body, "")
}

func serveWithToken(t *testing.T, status int, body, token string) (*apiclient.Client, *fake) {
	t.Helper()
	f := &fake{status: status, body: body}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return apiclient.New(srv.URL, token, 5*time.Second), f
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func wantQuery(t *testing.T, r *http.Request, want map[string]string) {
	t.Helper()
	q := r.URL.Query()
	for k, v := range want {
		if got := q.Get(k); got != v {
			t.Errorf("query %s = %q, want %q", k, got, v)
		}
	}
	for k := range q {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected query parameter %s=%q", k, q.Get(k))
		}
	}
}

func TestReadsHitTheRightRouteAndDecodeTheirFixture(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		fixture string
		path    string
		query   map[string]string
		call    func(c *apiclient.Client) (any, error)
		check   func(t *testing.T, got any)
	}{
		{
			name: "context", fixture: "context.json", path: "/api/v1/snapshots/snap-1/context",
			call: func(c *apiclient.Client) (any, error) { return c.Context(ctx, sid) },
			check: func(t *testing.T, got any) {
				ctx := got.(apiclient.Context)
				if ctx.Snapshot.ID == "" || ctx.Snapshot.Stats.Tables != 10 ||
					ctx.Snapshot.Project.Project.Name != "jaffle-shop-ddd" || len(ctx.Tables) != 10 ||
					ctx.Diagnostics["warning"] != 11 {
					t.Errorf("context decoded as %+v", ctx.Snapshot)
				}
			},
		},
		{
			name: "domains", fixture: "domains.json", path: "/api/v1/snapshots/snap-1/domains",
			call: func(c *apiclient.Client) (any, error) { return c.Domains(ctx, sid) },
			check: func(t *testing.T, got any) {
				if d := got.([]model.Domain); len(d) == 0 || d[0].ID == "" || d[0].TableCount == 0 {
					t.Errorf("domains = %+v", d)
				}
			},
		},
		{
			name: "tables filtered by domain", fixture: "tables.json", path: "/api/v1/snapshots/snap-1/tables",
			query: map[string]string{"domain": "ordering"},
			call:  func(c *apiclient.Client) (any, error) { return c.Tables(ctx, sid, "ordering") },
			check: func(t *testing.T, got any) {
				if ts := got.([]apiclient.TableSummary); len(ts) == 0 || ts[0].ColumnCount == 0 {
					t.Errorf("tables = %+v", ts)
				}
			},
		},
		{
			name: "tables without a filter", fixture: "tables.json", path: "/api/v1/snapshots/snap-1/tables",
			call: func(c *apiclient.Client) (any, error) { return c.Tables(ctx, sid, "") },
		},
		{
			name: "table", fixture: "table.json", path: "/api/v1/snapshots/snap-1/table",
			query: map[string]string{"id": tableID},
			call:  func(c *apiclient.Client) (any, error) { return c.Table(ctx, sid, tableID) },
			check: func(t *testing.T, got any) {
				d := got.(apiclient.TableDetail)
				if d.Table.ID != tableID || d.Table.Grain != "One row per order." || len(d.Table.Columns) == 0 {
					t.Errorf("table = %+v", d.Table)
				}
			},
		},
		{
			name: "tables detail joins ids", fixture: "tables_detail.json", path: "/api/v1/snapshots/snap-1/tables/detail",
			query: map[string]string{"ids": "a/one,b/two"},
			call:  func(c *apiclient.Client) (any, error) { return c.TablesDetail(ctx, sid, []string{"a/one", "b/two"}) },
			check: func(t *testing.T, got any) {
				if d := got.(apiclient.TablesDetail); len(d.Tables) == 0 || d.Missing == nil {
					t.Errorf("tables detail = %+v", d)
				}
			},
		},
		{
			name: "search", fixture: "search.json", path: "/api/v1/snapshots/snap-1/search",
			query: map[string]string{"q": "orders", "limit": "5"},
			call:  func(c *apiclient.Client) (any, error) { return c.Search(ctx, sid, "orders", 5) },
			check: func(t *testing.T, got any) {
				if h := got.([]apiclient.SearchHit); len(h) == 0 || h[0].TableID == "" {
					t.Errorf("hits = %+v", h)
				}
			},
		},
		{
			name: "neighbourhood uses the American route", fixture: "graph.json", path: "/api/v1/snapshots/snap-1/neighborhood",
			query: map[string]string{"table": tableID, "depth": "2", "sources": "true"},
			call:  func(c *apiclient.Client) (any, error) { return c.Neighbourhood(ctx, sid, tableID, 2, true) },
			check: func(t *testing.T, got any) {
				g := got.(apiclient.Graph)
				if len(g.Nodes) == 0 || len(g.Links) == 0 || g.Links[0].Source == "" {
					t.Errorf("graph = %+v", g)
				}
			},
		},
		{
			name: "join paths", fixture: "paths.json", path: "/api/v1/snapshots/snap-1/paths",
			query: map[string]string{"from": "a/one", "to": "b/two", "maxDepth": "3", "limit": "2"},
			call:  func(c *apiclient.Client) (any, error) { return c.JoinPaths(ctx, sid, "a/one", "b/two", 3, 2) },
			check: func(t *testing.T, got any) {
				p := got.([]apiclient.JoinPath)
				if len(p) == 0 || len(p[0].Hops) == 0 || p[0].Hops[0].From == "" {
					t.Errorf("paths = %+v; the hop's from key must decode", p)
				}
			},
		},
		{
			name: "lineage", fixture: "lineage.json", path: "/api/v1/snapshots/snap-1/lineage",
			query: map[string]string{"id": tableID, "direction": "downstream"},
			call:  func(c *apiclient.Client) (any, error) { return c.Lineage(ctx, sid, tableID, "downstream") },
			check: func(t *testing.T, got any) {
				if e := got.([]apiclient.LineageEntry); len(e) == 0 {
					t.Error("no entries")
				}
			},
		},
		{
			name: "diagnostics filtered by severity", fixture: "diagnostics.json", path: "/api/v1/snapshots/snap-1/diagnostics",
			query: map[string]string{"severity": "error"},
			call:  func(c *apiclient.Client) (any, error) { return c.Diagnostics(ctx, sid, "error") },
		},
		{
			name: "diagnostics without a filter", fixture: "diagnostics.json", path: "/api/v1/snapshots/snap-1/diagnostics",
			call: func(c *apiclient.Client) (any, error) { return c.Diagnostics(ctx, sid, "") },
		},
		{
			name: "sources", fixture: "sources.json", path: "/api/v1/snapshots/snap-1/sources",
			call: func(c *apiclient.Client) (any, error) { return c.Sources(ctx, sid) },
		},
		{
			name: "list conversations", fixture: "", path: "/api/v1/conversations",
			query: map[string]string{"snapshot": sid},
			call:  func(c *apiclient.Client) (any, error) { return c.ListConversations(ctx, sid, 0) },
		},
		{
			name: "list conversations with a limit", fixture: "", path: "/api/v1/conversations",
			query: map[string]string{"snapshot": sid, "limit": "3"},
			call:  func(c *apiclient.Client) (any, error) { return c.ListConversations(ctx, sid, 3) },
		},
		{
			name: "get conversation", fixture: "conversation.json", path: "/api/v1/conversations/conv-1",
			call: func(c *apiclient.Client) (any, error) { return c.GetConversation(ctx, "conv-1") },
			check: func(t *testing.T, got any) {
				conv := got.(model.Conversation)
				if len(conv.Messages) == 0 || conv.Messages[0].Ordinal != 0 || conv.Messages[0].Role == "" {
					t.Errorf("messages = %+v", conv.Messages)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"conversations":[{"id":"conv-1"}]}`
			if tc.fixture != "" {
				body = fixture(t, tc.fixture)
			}
			c, f := serve(t, 200, body)
			got, err := tc.call(c)
			if err != nil {
				t.Fatalf("call = %v", err)
			}
			r := f.last(t)
			if r.Method != http.MethodGet || r.URL.Path != tc.path {
				t.Errorf("%s %s, want GET %s", r.Method, r.URL.Path, tc.path)
			}
			wantQuery(t, r, tc.query)
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestResolveSnapshotReturnsTheConcreteID(t *testing.T) {
	c, f := serve(t, 200, `{"id":"real-id","createdAt":"2026-01-01T00:00:00Z"}`)
	id, err := c.ResolveSnapshot(context.Background(), "latest")
	if err != nil || id != "real-id" {
		t.Fatalf("ResolveSnapshot = %q, %v", id, err)
	}
	if p := f.last(t).URL.Path; p != "/api/v1/snapshots/latest" {
		t.Errorf("path = %s", p)
	}
}

func TestFeatures(t *testing.T) {
	c, f := serve(t, 200, `{"chat":{"available":true,"enabled":false}}`)
	got, err := c.Features(context.Background())
	if err != nil || !got.Chat.Available || got.Chat.Enabled {
		t.Fatalf("Features() = %+v, %v", got, err)
	}
	if p := f.last(t).URL.Path; p != "/api/v1/features" {
		t.Errorf("path = %s", p)
	}
}

// Table IDs contain a slash, so they must travel encoded in the query.
func TestTheTableIDSlashIsEncodedIntoTheQuery(t *testing.T) {
	c, f := serve(t, 200, `{"table":{"id":"ordering/fact_orders","name":"fact_orders"}}`)
	if _, err := c.Table(context.Background(), sid, tableID); err != nil {
		t.Fatal(err)
	}
	r := f.last(t)
	if r.URL.Path != "/api/v1/snapshots/snap-1/table" || r.URL.RawQuery != "id=ordering%2Ffact_orders" {
		t.Errorf("path %q query %q", r.URL.Path, r.URL.RawQuery)
	}
}

func TestErrorClassification(t *testing.T) {
	sentinels := []error{apiclient.ErrNotFound, apiclient.ErrForbidden, apiclient.ErrRejected, apiclient.ErrUnavailable}
	cases := []struct {
		status  int
		body    string
		want    error // nil: a plain *Error
		message string
	}{
		{404, `{"error":"snapshot not found"}`, apiclient.ErrNotFound, "snapshot not found"},
		{403, `{"error":"forbidden"}`, apiclient.ErrForbidden, "forbidden"},
		{400, `{"error":"malformed parameter"}`, apiclient.ErrRejected, "malformed parameter"},
		{422, `{"error":"unprocessable"}`, apiclient.ErrRejected, "unprocessable"},
		{500, `{"error":"boom"}`, nil, "boom"},
		{503, `{"error":"down"}`, nil, "down"},
		{502, `<html>Bad Gateway</html>`, nil, "Bad Gateway"},
		{500, ``, nil, "Internal Server Error"},
		{500, `{"error":42}`, nil, "Internal Server Error"},
	}
	for _, tc := range cases {
		c, _ := serve(t, tc.status, tc.body)
		_, err := c.Context(context.Background(), sid)

		var apiErr *apiclient.Error
		if !errors.As(err, &apiErr) {
			t.Fatalf("%d: error %v is not an *apiclient.Error", tc.status, err)
		}
		if apiErr.Status != tc.status || apiErr.Message != tc.message {
			t.Errorf("%d: got status %d message %q, want %q", tc.status, apiErr.Status, apiErr.Message, tc.message)
		}
		for _, s := range sentinels {
			if got := errors.Is(err, s); got != (s == tc.want) {
				t.Errorf("%d: errors.Is(%v) = %v", tc.status, s, got)
			}
		}
	}
}

func TestTransportFailuresAreUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	c := apiclient.New(srv.URL, "", time.Second)

	_, err := c.Domains(context.Background(), sid)
	var apiErr *apiclient.Error
	if !errors.Is(err, apiclient.ErrUnavailable) || !errors.As(err, &apiErr) ||
		apiErr.Status != 502 || !strings.Contains(apiErr.Message, "unreachable") {
		t.Fatalf("Domains() on a closed server = %v", err)
	}
	if errors.Is(err, apiclient.ErrRejected) || errors.Is(err, apiclient.ErrNotFound) {
		t.Error("an outage must not read as a refusal")
	}
	if _, err := c.CreateConversation(context.Background(), sid, ""); !errors.Is(err, apiclient.ErrUnavailable) {
		t.Errorf("a write on a closed server = %v", err)
	}
}

func TestATimeoutIsUnavailable(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(func() { close(release); srv.Close() })
	c := apiclient.New(srv.URL, "", 50*time.Millisecond)

	if _, err := c.Domains(context.Background(), sid); !errors.Is(err, apiclient.ErrUnavailable) {
		t.Fatalf("Domains() past the timeout = %v", err)
	}
}

func TestAuthorization(t *testing.T) {
	c, f := serveWithToken(t, 200, `{}`, "")
	c.Health(context.Background())
	if h, ok := f.last(t).Header["Authorization"]; ok {
		t.Errorf("Authorization = %q with no token", h)
	}

	c, f = serveWithToken(t, 200, `{}`, "a-token")
	c.Health(context.Background())
	if h := f.last(t).Header.Get("Authorization"); h != "Bearer a-token" {
		t.Errorf("Authorization = %q", h)
	}
	if h := f.last(t).Header.Get("Accept"); h != "application/json" {
		t.Errorf("Accept = %q", h)
	}
}

func TestRequestAndUserIDsAreForwardedFromTheContext(t *testing.T) {
	c, f := serve(t, 200, `{"sources":[]}`)

	if _, err := c.Sources(context.Background(), sid); err != nil {
		t.Fatal(err)
	}
	r := f.last(t)
	for _, h := range []string{"X-Request-Id", "X-Acting-User"} {
		if _, ok := r.Header[h]; ok {
			t.Errorf("%s sent with nothing on the context", h)
		}
	}

	ctx := reqctx.WithUserID(reqctx.WithRequestID(context.Background(), "req-1"), "user-1")
	if _, err := c.Sources(ctx, sid); err != nil {
		t.Fatal(err)
	}
	r = f.last(t)
	if r.Header.Get("X-Request-Id") != "req-1" || r.Header.Get("X-Acting-User") != "user-1" {
		t.Errorf("headers = %v", r.Header)
	}
}

func TestHealth(t *testing.T) {
	ctx := context.Background()
	c, f := serve(t, 200, `{"postgres":"ok"}`)
	if !c.Health(ctx) || f.last(t).URL.Path != "/readyz" {
		t.Error("a 200 from /readyz is healthy")
	}
	c, _ = serve(t, 503, `{"postgres":"down"}`)
	if c.Health(ctx) {
		t.Error("a 503 is unhealthy")
	}
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	if apiclient.New(srv.URL, "", time.Second).Health(ctx) {
		t.Error("an unreachable backend is unhealthy")
	}
}

func TestAnUnknownFieldIsTolerated(t *testing.T) {
	c, _ := serve(t, 200, `{"domains":[{"id":"ordering","addedInSomeLaterRelease":1}]}`)
	d, err := c.Domains(context.Background(), sid)
	if err != nil || len(d) != 1 || d[0].ID != "ordering" {
		t.Fatalf("Domains() = %+v, %v", d, err)
	}
}

func TestAMissingRequiredFieldIsAnError(t *testing.T) {
	ctx := context.Background()
	calls := map[string]struct {
		body string
		call func(*apiclient.Client) error
	}{
		"snapshot id":     {`{"name":"x"}`, func(c *apiclient.Client) error { _, err := c.ResolveSnapshot(ctx, "latest"); return err }},
		"features chat":   {`{}`, func(c *apiclient.Client) error { _, err := c.Features(ctx); return err }},
		"conversation id": {`{"title":"x"}`, func(c *apiclient.Client) error { _, err := c.GetConversation(ctx, "c"); return err }},
		"message role": {`{"ordinal":1}`, func(c *apiclient.Client) error {
			_, err := c.AppendMessage(ctx, "c", "user", "hi", nil, nil)
			return err
		}},
	}
	for name, tc := range calls {
		c, _ := serve(t, 200, tc.body)
		err := tc.call(c)
		var apiErr *apiclient.Error
		if err == nil || errors.As(err, &apiErr) {
			t.Errorf("%s: err = %v, want a decoding error", name, err)
		}
	}
}

func TestCreateConversationPostsTheDocumentedBody(t *testing.T) {
	c, f := serve(t, 201, `{"id":"conv-1","snapshotId":"resolved-id","title":"why"}`)
	conv, err := c.CreateConversation(context.Background(), "latest", "why")
	if err != nil || conv.ID != "conv-1" || conv.SnapshotID != "resolved-id" {
		t.Fatalf("CreateConversation = %+v, %v", conv, err)
	}
	r := f.last(t)
	if r.Method != http.MethodPost || r.URL.Path != "/api/v1/conversations" {
		t.Errorf("%s %s", r.Method, r.URL.Path)
	}
	// "latest" is passed through for the backend to resolve; no GET first.
	if got := f.lastBody(t); !reflect.DeepEqual(got, map[string]any{"snapshotId": "latest", "title": "why"}) {
		t.Errorf("body = %v", got)
	}
	if len(f.requests) != 1 {
		t.Errorf("%d requests, want 1", len(f.requests))
	}
}

func TestDeleteConversation(t *testing.T) {
	c, f := serve(t, 204, ``)
	if err := c.DeleteConversation(context.Background(), "conv-1"); err != nil {
		t.Fatalf("a 204 is success: %v", err)
	}
	if r := f.last(t); r.Method != http.MethodDelete || r.URL.Path != "/api/v1/conversations/conv-1" {
		t.Errorf("%s %s", r.Method, r.URL.Path)
	}

	c, _ = serve(t, 404, `{"error":"conversation not found"}`)
	if err := c.DeleteConversation(context.Background(), "nope"); !errors.Is(err, apiclient.ErrNotFound) {
		t.Errorf("DeleteConversation(nope) = %v", err)
	}
}

func TestSetConversationTitlePatchesOnlyTheTitle(t *testing.T) {
	c, f := serve(t, 200, `{"id":"conv-1","snapshotId":"snap-1","title":"a new title"}`)
	conv, err := c.SetConversationTitle(context.Background(), "conv-1", "a new title")
	if err != nil || conv.Title != "a new title" {
		t.Fatalf("SetConversationTitle = %+v, %v", conv, err)
	}
	r := f.last(t)
	if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/conversations/conv-1" {
		t.Errorf("%s %s", r.Method, r.URL.Path)
	}
	if got := f.lastBody(t); !reflect.DeepEqual(got, map[string]any{"title": "a new title"}) {
		t.Errorf("body = %v", got)
	}
}

func TestAppendMessage(t *testing.T) {
	ctx := context.Background()

	t.Run("fixture and server-assigned ordinal", func(t *testing.T) {
		c, f := serve(t, 201, fixture(t, "message.json"))
		msg, err := c.AppendMessage(ctx, "conv-1", "user", "hi", nil, nil)
		if err != nil || msg.Role != "user" || msg.CreatedAt.IsZero() {
			t.Fatalf("AppendMessage = %+v, %v", msg, err)
		}
		if r := f.last(t); r.Method != http.MethodPost || r.URL.Path != "/api/v1/conversations/conv-1/messages" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
	})

	t.Run("nil citations are sent as a list and meta is omitted", func(t *testing.T) {
		c, f := serve(t, 201, `{"ordinal":7,"role":"user","content":"hi","citations":[]}`)
		msg, err := c.AppendMessage(ctx, "conv-1", "user", "hi", nil, nil)
		if err != nil || msg.Ordinal != 7 {
			t.Fatalf("AppendMessage = %+v, %v", msg, err)
		}
		body := f.lastBody(t)
		if cites, ok := body["citations"].([]any); !ok || len(cites) != 0 {
			t.Errorf("citations = %#v, want []", body["citations"])
		}
		if _, ok := body["meta"]; ok {
			t.Error("meta sent when nil")
		}
	})

	t.Run("citations and meta survive", func(t *testing.T) {
		c, f := serve(t, 201, `{"ordinal":1,"role":"assistant","content":"two","citations":["ordering/fact_orders"]}`)
		msg, err := c.AppendMessage(ctx, "conv-1", "assistant", "two", []string{tableID}, map[string]any{"tokens": 10})
		if err != nil || !reflect.DeepEqual(msg.Citations, []string{tableID}) {
			t.Fatalf("AppendMessage = %+v, %v", msg, err)
		}
		want := map[string]any{
			"role": "assistant", "content": "two",
			"citations": []any{tableID}, "meta": map[string]any{"tokens": float64(10)},
		}
		if got := f.lastBody(t); !reflect.DeepEqual(got, want) {
			t.Errorf("body = %v", got)
		}
	})

	t.Run("the backend's refusal is carried", func(t *testing.T) {
		c, _ := serve(t, 400, `{"error":"\"robot\" is not a valid role"}`)
		_, err := c.AppendMessage(ctx, "conv-1", "robot", "hi", nil, nil)
		var apiErr *apiclient.Error
		if !errors.As(err, &apiErr) || apiErr.Status != 400 || !strings.Contains(apiErr.Message, "not a valid role") {
			t.Errorf("AppendMessage = %v", err)
		}
	})
}

func TestNotFoundOnConversationRoutes(t *testing.T) {
	ctx := context.Background()
	c, _ := serve(t, 404, `{"error":"not found"}`)
	calls := map[string]func() error{
		"create": func() error { _, err := c.CreateConversation(ctx, "nope", ""); return err },
		"get":    func() error { _, err := c.GetConversation(ctx, "nope"); return err },
		"title":  func() error { _, err := c.SetConversationTitle(ctx, "nope", "x"); return err },
		"append": func() error { _, err := c.AppendMessage(ctx, "nope", "user", "hi", nil, nil); return err },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, apiclient.ErrNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestACancelledCallKeepsItsCause(t *testing.T) {
	c, _ := serve(t, 200, `{"sources":[]}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Sources(ctx, sid)
	if !errors.Is(err, apiclient.ErrUnavailable) || !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want both ErrUnavailable and context.Canceled", err)
	}
}
