//go:build parity

// Run with `make test-chat-parity`. No model is called.
package parity_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var (
	pyURL      = strings.TrimRight(os.Getenv("PARITY_PY_URL"), "/")
	goURL      = strings.TrimRight(os.Getenv("PARITY_GO_URL"), "/")
	backendURL = strings.TrimRight(os.Getenv("PARITY_BACKEND_URL"), "/")
	apiToken   = envOr("PARITY_API_TOKEN", "relviz-dev-token-not-for-production")

	demoDir    = filepath.Join("..", "..", "..", "docs", "demo", "jaffle-shop-ddd")
	httpClient = &http.Client{Timeout: 60 * time.Second}
)

const (
	unknownID    = "00000000-0000-4000-8000-000000000000"
	factOrders   = "ordering/fact_orders"
	dimCustomers = "customer_identity/dim_customers"
)

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// request is sent to both services. body is JSON-encoded unless it is []byte.
type request struct {
	name, method, path string
	body               any
	anonymous          bool
}

type reply struct {
	Status     int  `json:"status"`
	RetryAfter bool `json:"retryAfter"`
	Body       any  `json:"body"`
}

func TestChatParity(t *testing.T) {
	if pyURL == "" || goURL == "" || backendURL == "" {
		t.Skip("PARITY_PY_URL, PARITY_GO_URL and PARITY_BACKEND_URL unset (make test-chat-parity)")
	}
	admin := login(t)
	sid := fixture(t, admin)
	send := func(t *testing.T, base string, r request) reply { return do(t, base, r, admin.id) }
	both := func(t *testing.T, r request) (reply, reply) { return send(t, pyURL, r), send(t, goURL, r) }

	// Chat off would make every comparison a matching 503.
	stats := request{method: "GET", path: "/api/chat/stats?snapshot=" + url.QueryEscape(sid)}
	for _, base := range []string{pyURL, goURL} {
		waitFor(t, base, stats, admin.id, http.StatusOK)
	}

	var table []request
	table = append(table,
		request{name: "healthz", method: "GET", path: "/healthz"},
		request{name: "readyz", method: "GET", path: "/readyz"},
	)
	for _, c := range toolCalls {
		table = append(table, request{
			name: "tool/" + c.name, method: "POST", path: "/debug/tool",
			body: map[string]any{"snapshotId": sid, "tool": c.tool, "args": c.args},
		})
	}
	answer := func(name string, body any) request {
		return request{name: "errors/" + name, method: "POST", path: "/api/chat/answer", body: body}
	}
	table = append(table,
		request{name: "errors/anonymous", method: "GET", path: "/api/chat/conversations?snapshot=latest", anonymous: true},
		answer("validation-missing", map[string]any{}),
		answer("validation-extra", map[string]any{"snapshotId": sid, "question": "hi", "bogus": 1}),
		answer("question-empty", map[string]any{"snapshotId": sid, "question": "   "}),
		answer("question-long", map[string]any{"snapshotId": sid, "question": strings.Repeat("a", 4001)}),
		answer("too-large", bytes.Repeat([]byte("a"), 1048577)),
		request{name: "errors/conversation-unknown", method: "GET", path: "/api/chat/conversations/" + unknownID},
		answer("snapshot-unknown", map[string]any{"snapshotId": unknownID, "question": "hi"}),
		request{name: "stats", method: stats.method, path: stats.path},
	)

	t.Run("debug/tools", func(t *testing.T) {
		py, gov := both(t, request{method: "GET", path: "/debug/tools"})
		// 18.1 decision: Go drops titles and the nullable anyOf.
		py.Body = goSchema(py.Body)
		compare(t, py, gov)
	})
	for _, r := range table {
		t.Run(r.name, func(t *testing.T) {
			py, gov := both(t, r)
			compare(t, py, gov)
		})
	}

	// Sequential, so each listing sees only that service's conversation.
	t.Run("conversations", func(t *testing.T) {
		flow := func(base string) []reply {
			created := send(t, base, request{method: "POST", path: "/api/chat/conversations",
				body: map[string]string{"snapshotId": sid, "title": "parity"}})
			cid := rawID(t, base, sid, admin.id)
			t.Cleanup(func() { do(t, base, request{method: "DELETE", path: "/api/chat/conversations/" + cid}, admin.id) })
			return []reply{
				created,
				send(t, base, request{method: "GET", path: "/api/chat/conversations?snapshot=" + url.QueryEscape(sid)}),
				send(t, base, request{method: "GET", path: "/api/chat/conversations/" + cid}),
				send(t, base, request{method: "DELETE", path: "/api/chat/conversations/" + cid}),
				send(t, base, request{method: "GET", path: "/api/chat/conversations/" + cid}),
			}
		}
		py, gov := flow(pyURL), flow(goURL)
		for i, step := range []string{"create", "list", "get", "delete", "get-deleted"} {
			t.Run(step, func(t *testing.T) { compare(t, py[i], gov[i]) })
		}
	})

	// Last: switching chat off is global, and the gate caches it for up to 15s.
	t.Run("errors/chat-off", func(t *testing.T) {
		admin.setChat(t, false)
		t.Cleanup(func() { admin.setChat(t, true) })
		r := request{method: "GET", path: "/api/chat/conversations?snapshot=" + url.QueryEscape(sid)}
		for _, base := range []string{pyURL, goURL} {
			waitFor(t, base, r, admin.id, http.StatusServiceUnavailable)
		}
		py, gov := both(t, r)
		compare(t, py, gov)
	})
}

// Two argument sets per tool, then capture.sh's error cases.
var toolCalls = []struct {
	name, tool string
	args       map[string]any
}{
	{"list_domains", "list_domains", map[string]any{}},
	{"list_domains-extra", "list_domains", map[string]any{"bogus": 1}},
	{"list_tables-ordering", "list_tables", map[string]any{"domain": "ordering"}},
	{"list_tables-all", "list_tables", map[string]any{}},
	{"get_tables", "get_tables", map[string]any{"ids": []string{factOrders}}},
	{"get_tables-several", "get_tables", map[string]any{"ids": []string{factOrders, dimCustomers, "ordering/fact_order_items"}}},
	{"search_model", "search_model", map[string]any{"query": "customer", "limit": 5}},
	{"search_model-default-limit", "search_model", map[string]any{"query": "orders"}},
	{"get_neighbourhood", "get_neighbourhood", map[string]any{"table_id": factOrders}},
	{"get_neighbourhood-depth-3", "get_neighbourhood", map[string]any{"table_id": dimCustomers, "depth": 3}},
	{"find_join_paths", "find_join_paths", map[string]any{"from_table": "ordering/fact_order_items", "to_table": dimCustomers}},
	{"find_join_paths-short", "find_join_paths", map[string]any{"from_table": factOrders, "to_table": dimCustomers, "max_depth": 1}},
	{"get_lineage", "get_lineage", map[string]any{"table_id": factOrders}},
	{"get_lineage-downstream", "get_lineage", map[string]any{"table_id": factOrders, "direction": "downstream"}},
	{"list_diagnostics", "list_diagnostics", map[string]any{}},
	{"list_diagnostics-warning", "list_diagnostics", map[string]any{"severity": "warning"}},
	{"list_source_models", "list_source_models", map[string]any{}},
	{"list_source_models-extra", "list_source_models", map[string]any{"bogus": 1}},
	{"get_tables-missing", "get_tables", map[string]any{"ids": []string{factOrders, "ordering/nope"}}},
	{"unknown", "nope", map[string]any{}},
	{"bad-args", "search_model", map[string]any{"query": "customer", "limit": 500}},
}

func do(t *testing.T, base string, r request, user string) reply {
	t.Helper()
	var body io.Reader
	switch b := r.body.(type) {
	case nil:
	case []byte:
		body = bytes.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		body = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(r.method, base+r.path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if !r.anonymous {
		req.Header.Set("X-User-Id", user)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	out := reply{Status: res.StatusCode, RetryAfter: res.Header.Get("Retry-After") != ""}
	if json.Unmarshal(raw, &out.Body) != nil {
		out.Body = nil
	}
	out.Body = normalise(out.Body)
	return out
}

// rawID reads the conversation ID before normalise hides it.
func rawID(t *testing.T, base, sid, user string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/api/chat/conversations?snapshot="+url.QueryEscape(sid), nil)
	req.Header.Set("X-User-Id", user)
	res, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out struct {
		Conversations []struct {
			ID string `json:"id"`
		} `json:"conversations"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil || len(out.Conversations) != 1 {
		t.Fatalf("%s: listing %d, %v", base, len(out.Conversations), err)
	}
	return out.Conversations[0].ID
}

func waitFor(t *testing.T, base string, r request, user string, status int) {
	t.Helper()
	for range 60 {
		if do(t, base, r, user).Status == status {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("%s%s: no %d after 60s", base, r.path, status)
}

func compare(t *testing.T, py, gov reply) {
	t.Helper()
	if reflect.DeepEqual(py, gov) {
		return
	}
	pyJSON, _ := json.MarshalIndent(py, "", "  ")
	goJSON, _ := json.MarshalIndent(gov, "", "  ")
	var a, b any
	_ = json.Unmarshal(pyJSON, &a)
	_ = json.Unmarshal(goJSON, &b)
	t.Errorf("mismatch at %s\npython: %s\n    go: %s", firstDiff(a, b, "$"), pyJSON, goJSON)
}

// firstDiff is the path to the first differing value, keys in sorted order.
func firstDiff(a, b any, path string) string {
	switch a := a.(type) {
	case map[string]any:
		bm, ok := b.(map[string]any)
		if !ok {
			return path
		}
		keys := map[string]bool{}
		for k := range a {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			av, aok := a[k]
			bv, bok := bm[k]
			if aok != bok {
				return path + "." + k
			}
			if !reflect.DeepEqual(av, bv) {
				return firstDiff(av, bv, path+"."+k)
			}
		}
	case []any:
		bs, ok := b.([]any)
		if !ok {
			return path
		}
		for i := range min(len(a), len(bs)) {
			if !reflect.DeepEqual(a[i], bs[i]) {
				return firstDiff(a[i], bs[i], fmt.Sprintf("%s[%d]", path, i))
			}
		}
		if len(a) != len(bs) {
			return fmt.Sprintf("%s[%d]", path, min(len(a), len(bs)))
		}
	}
	return path
}

type session struct {
	id     string
	client *http.Client
}

// login signs in as the bootstrap admin, as a browser would.
func login(t *testing.T) session {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	s := session{client: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
	res := s.do(t, "POST", "/api/v1/auth/login", map[string]string{
		"username": envOr("PARITY_ADMIN_USERNAME", "admin"),
		"password": envOr("PARITY_ADMIN_PASSWORD", "relviz-dev-admin-password"),
	})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", res.StatusCode)
	}
	res = s.do(t, "GET", "/api/v1/auth/me", nil)
	defer func() { _ = res.Body.Close() }()
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(res.Body).Decode(&me); err != nil || me.User.ID == "" {
		t.Fatalf("auth/me: %d %v", res.StatusCode, err)
	}
	s.id = me.User.ID
	return s
}

func (s session) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		r = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, backendURL+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "urara")
	res, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// setChat needs the admin session; the API token gets 403.
func (s session) setChat(t *testing.T, on bool) {
	t.Helper()
	res := s.do(t, "PATCH", "/api/v1/settings", map[string]bool{"chatEnabled": on})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("chatEnabled=%v: %d", on, res.StatusCode)
	}
}

// backendDo calls the backend as the service, acting as the admin.
func backendDo(t *testing.T, method, path string, body any, admin session) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		r = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, backendURL+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("X-Acting-User", admin.id)
	res, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// fixture ingests jaffle-shop-ddd as a unique project, deleted in t.Cleanup.
func fixture(t *testing.T, admin session) string {
	t.Helper()
	name := "chat-parity-" + uuid.NewString()[:12]
	res := backendDo(t, "POST", "/api/v1/ingest",
		map[string]any{"name": name, "sourceLabel": "chat-parity", "files": demoFiles(t, name)}, admin)
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	var out struct {
		Snapshot struct {
			ID string `json:"id"`
		} `json:"snapshot"`
		Project struct {
			Slug string `json:"slug"`
		} `json:"project"`
	}
	if res.StatusCode != http.StatusCreated || json.Unmarshal(raw, &out) != nil || out.Snapshot.ID == "" {
		t.Fatalf("ingest: %d %.300s", res.StatusCode, raw)
	}
	t.Cleanup(func() {
		res := backendDo(t, "DELETE", "/api/v1/projects/"+out.Project.Slug, nil, admin)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Errorf("deleting project %s: %d", out.Project.Slug, res.StatusCode)
		}
	})
	return out.Snapshot.ID
}

var manifestName = regexp.MustCompile(`(?m)^name\s*=\s*".*"$`)

// demoFiles reads the set as capture.sh does, renamed so the project is new.
func demoFiles(t *testing.T, name string) []map[string]string {
	t.Helper()
	var files []map[string]string
	err := filepath.WalkDir(demoDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(demoDir, p)
		if ext := filepath.Ext(p); (ext != ".md" && ext != ".toml") || rel == "README.md" {
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if rel == "projectmeta.toml" {
			content = manifestName.ReplaceAll(content, []byte(`name = "`+name+`"`))
		}
		files = append(files, map[string]string{"path": filepath.ToSlash(rel), "content": string(content)})
		return nil
	})
	if err != nil {
		t.Fatalf("reading %s (docs/ must be mounted): %v", demoDir, err)
	}
	slices.SortFunc(files, func(a, b map[string]string) int { return strings.Compare(a["path"], b["path"]) })
	return files
}

// goSchema is Python's tool schema rewritten as 18.1 decided: no title keys,
// and {"anyOf": [X, null], "default": null} as plain X.
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
