//go:build integration

// The Go chat service over HTTP, against the real backend. No model is called.
//
// Run with `make test-chat-go-integration`.
package chat_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

var (
	chatURL    = strings.TrimRight(os.Getenv("TEST_CHAT_URL"), "/")
	backendURL = strings.TrimRight(os.Getenv("TEST_CHAT_BACKEND_URL"), "/")
	apiToken   = envOr("TEST_CHAT_API_TOKEN", "relviz-dev-token-not-for-production")

	demoDir   = filepath.Join("..", "..", "..", "..", "docs", "demo", "jaffle-shop-ddd")
	goldenDir = filepath.Join("..", "..", "unit", "chat", "testdata", "golden")

	httpClient = &http.Client{Timeout: 60 * time.Second}
)

// Not t.Skip: the backend CI job runs ./tests/integration/... and fails on a
// skip, and this suite joins it only at Phase 20's cutover.
func TestMain(m *testing.M) {
	if chatURL == "" && backendURL == "" {
		fmt.Println("chat integration: TEST_CHAT_URL and TEST_CHAT_BACKEND_URL unset; not run (make test-chat-go-integration)")
		os.Exit(0)
	}
	if chatURL == "" || backendURL == "" {
		fmt.Println("chat integration: set both TEST_CHAT_URL and TEST_CHAT_BACKEND_URL")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

var (
	adminOnce sync.Once
	adminID   string
	adminErr  error
)

// userID is the bootstrap admin's ID, logged in as a browser would.
func userID(t *testing.T) string {
	t.Helper()
	adminOnce.Do(func() { adminID, adminErr = login() })
	if adminErr != nil {
		t.Fatal(adminErr)
	}
	return adminID
}

func login() (string, error) {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	body, _ := json.Marshal(map[string]string{
		"username": envOr("TEST_CHAT_ADMIN_USERNAME", "admin"),
		"password": envOr("TEST_CHAT_ADMIN_PASSWORD", "relviz-dev-admin-password"),
	})
	req, _ := http.NewRequest("POST", backendURL+"/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "urara")
	res, err := c.Do(req)
	if err != nil {
		return "", err
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login: %d", res.StatusCode)
	}

	req, _ = http.NewRequest("GET", backendURL+"/api/v1/auth/me", nil)
	req.Header.Set("X-Requested-With", "urara")
	res, err = c.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(res.Body).Decode(&me); err != nil || me.User.ID == "" {
		return "", fmt.Errorf("auth/me: %d %v", res.StatusCode, err)
	}
	return me.User.ID, nil
}

// backendDo calls the backend as the service; asAdmin adds the acting user.
func backendDo(t *testing.T, method, path string, body any, asAdmin bool) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, backendURL+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiToken)
	if asAdmin {
		req.Header.Set("X-Acting-User", userID(t))
	}
	res, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// fixture ingests jaffle-shop-ddd as a unique project, deletes the project in
// t.Cleanup, and returns the snapshot ID.
func fixture(t *testing.T) string {
	t.Helper()
	name := "chat-go-it-" + uuid.NewString()[:12]
	files := demoFiles(t, name)
	res := backendDo(t, "POST", "/api/v1/ingest", map[string]any{"name": name, "sourceLabel": "chat-go-it", "files": files}, false)
	defer func() { _ = res.Body.Close() }()
	var out struct {
		Snapshot struct {
			ID string `json:"id"`
		} `json:"snapshot"`
		Project struct {
			Slug string `json:"slug"`
		} `json:"project"`
	}
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusCreated || json.Unmarshal(raw, &out) != nil || out.Snapshot.ID == "" {
		t.Fatalf("ingest: %d %.300s", res.StatusCode, raw)
	}
	t.Cleanup(func() {
		res := backendDo(t, "DELETE", "/api/v1/projects/"+out.Project.Slug, nil, true)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusNotFound {
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

type response struct {
	status int
	header http.Header
	raw    []byte
}

func (r response) json(t *testing.T) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(r.raw, &body); err != nil {
		t.Fatalf("body %.300s: %v", r.raw, err)
	}
	return body
}

// chatDo calls chat-go as nginx would, with X-User-Id unless user is empty.
func chatDo(t *testing.T, method, path, user string, body any) response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, chatURL+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("X-User-Id", user)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	return response{status: res.StatusCode, header: res.Header, raw: raw}
}

func chatGet(t *testing.T, path string) response {
	t.Helper()
	return chatDo(t, "GET", path, userID(t), nil)
}

func chatPost(t *testing.T, path string, body any) response {
	t.Helper()
	return chatDo(t, "POST", path, userID(t), body)
}

func chatDelete(t *testing.T, path string) response {
	t.Helper()
	return chatDo(t, "DELETE", path, userID(t), nil)
}

// Copied from tests/unit/chat/httpapi/golden_test.go: capture.sh's placeholders.
var (
	uuidPattern    = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	timePattern    = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}`)
	latencyPattern = regexp.MustCompile(`(?i)latency|duration`)
)

func normalise(v any) any {
	switch v := v.(type) {
	case string:
		s := uuidPattern.ReplaceAllString(v, "<uuid>")
		if timePattern.MatchString(s) {
			return "<time>"
		}
		return s
	case []any:
		for i := range v {
			v[i] = normalise(v[i])
		}
		return v
	case map[string]any:
		for k, x := range v {
			switch _, isNumber := x.(float64); {
			case k == "requestId":
				v[k] = "<request-id>"
			case isNumber && latencyPattern.MatchString(k):
				v[k] = "<number>"
			default:
				v[k] = normalise(x)
			}
		}
		if fields, ok := v["fields"].([]any); ok {
			for _, f := range fields {
				if m, ok := f.(map[string]any); ok {
					delete(m, "reason")
				}
			}
		}
		return v
	}
	return v
}

type golden struct {
	Status  int             `json:"status"`
	Headers map[string]bool `json:"headers"`
	Body    any             `json:"body"`
}

// assertGolden compares r with golden/<name>; rewrite, if given, adjusts the golden body first.
func assertGolden(t *testing.T, name string, r response, rewrite ...func(any) any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenDir, name))
	if err != nil {
		t.Fatal(err)
	}
	var want golden
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	for _, f := range rewrite {
		want.Body = f(want.Body)
	}
	want.Body = normalise(want.Body)

	got := golden{
		Status: r.status,
		Headers: map[string]bool{
			"Retry-After":  r.header.Get("Retry-After") != "",
			"X-Request-Id": r.header.Get("X-Request-Id") != "",
		},
	}
	if len(r.raw) > 0 && json.Unmarshal(r.raw, &got.Body) != nil {
		got.Body = nil
	}
	got.Body = normalise(got.Body)

	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("%s does not match\n got: %.2000s\nwant: %.2000s", name, gotJSON, wantJSON)
	}
}
