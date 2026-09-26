// The route → permission table and its enforcement.
package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/api"
	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/model"
)

func walkRoutes(t *testing.T, h http.Handler) []string {
	t.Helper()
	var out []string
	err := chi.Walk(h.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		out = append(out, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPermissionTableCoversEveryRoute(t *testing.T) {
	declared := api.DeclaredRoutes()
	walked := walkRoutes(t, newAuthedServer(t))
	for _, r := range walked {
		if strings.Contains(r, " /api/v1/") && !slices.Contains(declared, r) {
			t.Errorf("%s has no permission entry", r)
		}
	}
	for _, r := range declared {
		if !slices.Contains(walked, r) {
			t.Errorf("stale permission entry: %s", r)
		}
	}
}

// roleServer has a signed-in user of each role; send with asRole.
func roleServer(t *testing.T, meta *fakeMeta) http.Handler {
	t.Helper()
	for _, role := range []auth.Role{auth.RoleViewer, auth.RoleCreator, auth.RoleAdmin} {
		meta.addUser(model.User{ID: string(role), Username: string(role), Role: string(role)})
		meta.addSession(auth.HashToken(string(role)+"-token"), string(role))
	}
	return newAuthedServerWith(t, meta)
}

func asRole(t *testing.T, h http.Handler, role auth.Role, method, target string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	r := withCookie(httptest.NewRequest(method, target, body), string(role)+"-token")
	r.Header.Set("X-Requested-With", "urara")
	r.Header.Set("Content-Type", "application/json")
	return serve(h, r)
}

func asService(h http.Handler, method, target string, body io.Reader) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, body)
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set("Content-Type", "application/json")
	return serve(h, r)
}

func wantCode(t *testing.T, who string, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("%s: status = %d, want %d: %s", who, rec.Code, want, rec.Body)
	}
	if want == http.StatusForbidden && !strings.Contains(rec.Body.String(), "not allowed") {
		t.Errorf("%s: body = %s", who, rec.Body)
	}
}

func TestPermissionViewer(t *testing.T) {
	h := roleServer(t, &fakeMeta{})
	wantCode(t, "GET /projects", asRole(t, h, auth.RoleViewer, http.MethodGet, "/api/v1/projects", nil), http.StatusOK)
	wantCode(t, "DELETE /projects/x", asRole(t, h, auth.RoleViewer, http.MethodDelete, "/api/v1/projects/x", nil), http.StatusForbidden)
	wantCode(t, "DELETE /projects/x/", asRole(t, h, auth.RoleViewer, http.MethodDelete, "/api/v1/projects/x/", nil), http.StatusForbidden)
	wantCode(t, "PATCH /settings", asRole(t, h, auth.RoleViewer, http.MethodPatch, "/api/v1/settings", strings.NewReader(`{}`)), http.StatusForbidden)
}

func TestPermissionCreatorAdminOnlyRoutes(t *testing.T) {
	h := roleServer(t, &fakeMeta{})
	for _, c := range []struct{ method, target string }{
		{http.MethodDelete, "/api/v1/projects/x"},
		{http.MethodDelete, "/api/v1/projects/x/versions/1.0.0"},
		{http.MethodDelete, "/api/v1/snapshots/s1"},
		{http.MethodPatch, "/api/v1/settings"},
	} {
		wantCode(t, c.method+" "+c.target, asRole(t, h, auth.RoleCreator, c.method, c.target, strings.NewReader(`{}`)), http.StatusForbidden)
	}
}

func TestPermissionService(t *testing.T) {
	h := newAuthedServer(t)
	wantCode(t, "GET /projects", asService(h, http.MethodGet, "/api/v1/projects", nil), http.StatusOK)
	wantCode(t, "DELETE /projects/x", asService(h, http.MethodDelete, "/api/v1/projects/x", nil), http.StatusForbidden)
}

func TestPermissionAnonymousAllowedEverywhere(t *testing.T) {
	h := newServer(t, &fakeMeta{}, &fakeGraphs{})
	for _, c := range []struct{ method, target string }{
		{http.MethodGet, "/api/v1/projects"},
		{http.MethodDelete, "/api/v1/projects/x"},
		{http.MethodDelete, "/api/v1/projects/x/versions/1.0.0"},
		{http.MethodPatch, "/api/v1/settings"},
	} {
		if rec := do(t, h, c.method, c.target, strings.NewReader(`{}`), "application/json"); rec.Code == http.StatusForbidden {
			t.Errorf("%s %s: 403 with auth disabled", c.method, c.target)
		}
	}
}

func TestPermissionUnmappedRouteIs500(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := api.New(authCfg(), &fakeMeta{}, &fakeGraphs{}, log)
	root := chi.NewRouter()
	root.Route("/api/v1", func(r chi.Router) {
		r.Use(s.Authenticated)
		r.Use(s.RequirePermission(root))
		r.Get("/unmapped", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	})
	wantCode(t, "GET /unmapped", asService(root, http.MethodGet, "/api/v1/unmapped", nil), http.StatusInternalServerError)
}
