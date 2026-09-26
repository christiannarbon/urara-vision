//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/api"
	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/config"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/projectmeta"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/fixtures"
	"urara-vision/backend/tests/integration/harness"
)

type role string

const (
	viewer  role = "viewer"
	creator role = "creator"
	admin   role = "admin"
	anon    role = ""
	service role = "service"
)

var roles = []role{viewer, creator, admin}

const matrixPassword = "role-matrix-password-1"

type matrix struct {
	t       *testing.T
	base    string
	pg      *postgres.Store
	cookies map[role]*http.Cookie
	ids     map[role]string
}

// newMatrix starts the real server without stack()'s bearer wrapper, so each
// request carries exactly the credentials the case names.
func newMatrix(t *testing.T) *matrix {
	t.Helper()
	// A skip would read as a pass in CI.
	for _, env := range []string{harness.EnvPostgresDSN, harness.EnvNeo4jURI} {
		if os.Getenv(env) == "" {
			t.Fatalf("%s is not set; the role matrix must run (see: make test-integration)", env)
		}
	}
	pg := harness.Postgres(t)
	gs := harness.Neo4j(t)
	cfg := &config.Config{
		CORSOrigins:    []string{"http://localhost:5173"},
		MaxUploadBytes: 64 << 20,
		MaxFiles:       5000,
		APIToken:       serviceToken,
		SessionTTL:     time.Hour,
		ChatEnabled:    true,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(api.New(cfg, pg, gs, log).Routes())
	t.Cleanup(srv.Close)

	m := &matrix{t: t, base: srv.URL, pg: pg, cookies: map[role]*http.Cookie{}, ids: map[role]string{}}
	hash, err := auth.HashPassword(matrixPassword)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		u := m.user(string(r), hash)
		m.ids[r] = u.ID
		m.cookies[r] = m.login(u.Username)
	}
	return m
}

func (m *matrix) user(r, hash string) *model.User {
	m.t.Helper()
	u, err := m.pg.CreatePasswordUser(harness.Context(m.t), "matrix-"+r+"-"+uuid.NewString()[:8], "", r, hash)
	if err != nil {
		m.t.Fatalf("CreatePasswordUser: %v", err)
	}
	m.t.Cleanup(func() { dropUser(m.t, u.ID) })
	return u
}

// dropUser bypasses the last-admin guard, which cleanup has no reason to honour.
func dropUser(t *testing.T, id string) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, harness.PostgresDSN(t))
	if err != nil {
		t.Errorf("cleanup connect: %v", err)
		return
	}
	defer func() { _ = conn.Close(ctx) }()
	_, _ = conn.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
}

func (m *matrix) login(username string) *http.Cookie {
	m.t.Helper()
	code, _, res := m.send(anon, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"username": username, "password": matrixPassword})
	if code != http.StatusOK {
		m.t.Fatalf("login %s = %d", username, code)
	}
	for _, c := range res.Cookies() {
		if c.Name == "urara_session" {
			return c
		}
	}
	m.t.Fatalf("login %s set no session cookie", username)
	return nil
}

func (m *matrix) send(as role, method, path string, body any) (int, []byte, *http.Response) {
	m.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			m.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, m.base+path, rd)
	if err != nil {
		m.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "urara")
	switch as {
	case service:
		req.Header.Set("Authorization", "Bearer "+serviceToken)
	case anon:
	default:
		req.AddCookie(m.cookies[as])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		m.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, raw, res
}

func ingestBody(name, version string) map[string]any {
	manifest := strings.Replace(fixtures.ProjectMetaTOML, `name = "sample-data-modelling-project"`, `name = "`+name+`"`, 1)
	manifest = strings.Replace(manifest, `version = "0.1.0"`, `version = "`+version+`"`, 1)
	files := []map[string]string{{"path": projectmeta.FileName, "content": manifest}}
	for _, f := range fixtures.StarSchema() {
		files = append(files, map[string]string{"path": f.Path, "content": f.Content})
	}
	return map[string]any{"name": name, "sourceLabel": "matrix", "files": files}
}

// cleanupProject removes a project as the admin, whether or not the case deleted it.
func (m *matrix) cleanupProject(name string) string {
	slug := projectmeta.Slug(name)
	m.t.Cleanup(func() {
		if code, raw, _ := m.send(admin, http.MethodDelete, "/api/v1/projects/"+slug, nil); code != http.StatusNoContent && code != http.StatusNotFound {
			m.t.Errorf("cleanup project %s = %d: %s", slug, code, raw)
		}
	})
	return slug
}

// project ingests a fresh project as the service: its name, slug and snapshot IDs.
func (m *matrix) project(versions ...string) (string, string, []string) {
	m.t.Helper()
	name := "matrix-" + uuid.NewString()[:8]
	slug := m.cleanupProject(name)
	var sids []string
	for _, v := range versions {
		code, raw, _ := m.send(service, http.MethodPost, "/api/v1/ingest", ingestBody(name, v))
		if code != http.StatusCreated {
			m.t.Fatalf("fixture ingest = %d: %s", code, raw)
		}
		var out struct {
			Snapshot struct{ ID string } `json:"snapshot"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			m.t.Fatal(err)
		}
		sids = append(sids, out.Snapshot.ID)
	}
	return name, slug, sids
}

type request struct {
	method, path string
	body         any
}

type matrixCase struct {
	name string
	perm auth.Permission // empty for a route any signed-in caller may use
	// req runs once per role, so a destructive case can build its own fixture.
	req  func(m *matrix, as role) request
	want map[role]int
}

func all(code int) map[role]int { return map[role]int{viewer: code, creator: code, admin: code} }

func adminOnly(code int) map[role]int {
	return map[role]int{viewer: http.StatusForbidden, creator: http.StatusForbidden, admin: code}
}

func matrixCases(slug, name string, sids []string) []matrixCase {
	get := func(path string) func(*matrix, role) request {
		return func(*matrix, role) request { return request{http.MethodGet, path, nil} }
	}
	importers := map[role]int{viewer: http.StatusForbidden, creator: http.StatusCreated, admin: http.StatusCreated}
	return []matrixCase{
		{"list projects", auth.PermProjectView, get("/api/v1/projects"), all(http.StatusOK)},
		{"get version", auth.PermProjectView, get("/api/v1/projects/" + slug + "/versions/0.1.0"), all(http.StatusOK)},
		{"diff", auth.PermProjectView, get("/api/v1/projects/" + slug + "/diff?from=0.1.0&to=0.2.0"), all(http.StatusOK)},
		{"snapshot tables", auth.PermProjectView, get("/api/v1/snapshots/" + sids[0] + "/tables"), all(http.StatusOK)},
		{"me", "", get("/api/v1/auth/me"), all(http.StatusOK)},
		{"ingest new project", auth.PermProjectImport, func(m *matrix, _ role) request {
			fresh := "matrix-" + uuid.NewString()[:8]
			m.cleanupProject(fresh)
			return request{http.MethodPost, "/api/v1/ingest", ingestBody(fresh, "1.0.0")}
		}, importers},
		{"ingest new version", auth.PermVersionImport, func(*matrix, role) request {
			return request{http.MethodPost, "/api/v1/ingest", ingestBody(name, "9.0.0-"+uuid.NewString()[:8])}
		}, importers},
		{"delete version", auth.PermVersionDelete, func(m *matrix, _ role) request {
			_, p, _ := m.project("0.1.0", "0.2.0")
			return request{http.MethodDelete, "/api/v1/projects/" + p + "/versions/0.1.0", nil}
		}, adminOnly(http.StatusNoContent)},
		{"delete project", auth.PermProjectDelete, func(m *matrix, _ role) request {
			_, p, _ := m.project("0.1.0")
			return request{http.MethodDelete, "/api/v1/projects/" + p, nil}
		}, adminOnly(http.StatusNoContent)},
		{"delete snapshot", auth.PermProjectDelete, func(m *matrix, _ role) request {
			_, _, s := m.project("0.1.0")
			return request{http.MethodDelete, "/api/v1/snapshots/" + s[0], nil}
		}, adminOnly(http.StatusNoContent)},
		{"patch settings", auth.PermSettingsManage, func(*matrix, role) request {
			return request{http.MethodPatch, "/api/v1/settings", map[string]bool{"chatEnabled": true}}
		}, adminOnly(http.StatusOK)},
		{"list users", auth.PermUserManage, get("/api/v1/users"), adminOnly(http.StatusOK)},
		{"reset password", auth.PermUserManage, func(m *matrix, _ role) request {
			u := m.user("viewer", "h")
			return request{http.MethodPost, "/api/v1/users/" + u.ID + "/password", map[string]string{"password": matrixPassword}}
		}, adminOnly(http.StatusNoContent)},
		{"delete user", auth.PermUserDelete, func(m *matrix, _ role) request {
			u := m.user("viewer", "h")
			return request{http.MethodDelete, "/api/v1/users/" + u.ID, nil}
		}, adminOnly(http.StatusNoContent)},
		{"create conversation", auth.PermChatUse, func(*matrix, role) request {
			return request{http.MethodPost, "/api/v1/conversations", map[string]string{"snapshotId": sids[0]}}
		}, all(http.StatusCreated)},
	}
}

func TestRoleMatrix(t *testing.T) {
	m := newMatrix(t)
	name, slug, sids := m.project("0.1.0", "0.2.0")
	cases := matrixCases(slug, name, sids)

	for _, c := range cases {
		for _, r := range roles {
			t.Run(c.name+"/"+string(r), func(t *testing.T) {
				req := c.req(m, r)
				code, raw, _ := m.send(r, req.method, req.path, req.body)
				if code != c.want[r] {
					t.Errorf("%s %s as %s = %d, want %d: %s", req.method, req.path, r, code, c.want[r], raw)
				}
			})
		}
	}

	t.Run("no cookie is 401", func(t *testing.T) {
		if code, raw, _ := m.send(anon, http.MethodGet, "/api/v1/projects", nil); code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401: %s", code, raw)
		}
	})

	t.Run("another user's conversation is 404", func(t *testing.T) {
		code, raw, _ := m.send(viewer, http.MethodPost, "/api/v1/conversations", map[string]string{"snapshotId": sids[0]})
		if code != http.StatusCreated {
			t.Fatalf("create = %d: %s", code, raw)
		}
		var conv struct{ ID string }
		_ = json.Unmarshal(raw, &conv)
		if code, _, _ := m.send(viewer, http.MethodGet, "/api/v1/conversations/"+conv.ID, nil); code != http.StatusOK {
			t.Errorf("owner GET = %d, want 200", code)
		}
		if code, raw, _ := m.send(creator, http.MethodGet, "/api/v1/conversations/"+conv.ID, nil); code != http.StatusNotFound {
			t.Errorf("stranger GET = %d, want 404: %s", code, raw)
		}
	})

	// A permission some route requires must have a case. Ingest decides in the handler.
	t.Run("every routed permission has a case", func(t *testing.T) {
		routed := map[auth.Permission]bool{auth.PermProjectImport: true, auth.PermVersionImport: true}
		for _, p := range api.RoutePermissions() {
			routed[p] = true
		}
		covered := map[auth.Permission]bool{}
		for _, c := range cases {
			covered[c.perm] = true
		}
		for _, p := range auth.Permissions(auth.RoleAdmin) {
			switch {
			case routed[p] && !covered[p]:
				t.Errorf("%s guards a route but has no matrix case", p)
			case !routed[p]:
				t.Logf("%s guards no route yet", p)
			}
		}
	})
}
