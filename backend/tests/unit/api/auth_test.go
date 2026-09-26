// Bearer-token rows of authenticate.
package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"urara-vision/backend/internal/api"
	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/config"
	"urara-vision/backend/internal/model"
)

const testToken = "0123456789abcdef0123456789abcdef"

func authCfg() *config.Config {
	return &config.Config{
		CORSOrigins:    []string{"http://localhost:5173"},
		MaxUploadBytes: 64 << 20,
		MaxFiles:       100,
		APIToken:       testToken,
	}
}

// newAuthedServer wires a Server that requires authentication.
func newAuthedServer(t *testing.T) http.Handler {
	t.Helper()
	return newAuthedServerWith(t, &fakeMeta{})
}

func newAuthedServerWith(t *testing.T, meta *fakeMeta) http.Handler {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.New(authCfg(), meta, &fakeGraphs{}, log).Routes()
}

// principalProbe runs the auth middleware in front of a handler that records
// the principal it was given.
func principalProbe(t *testing.T, cfg *config.Config, meta *fakeMeta) (http.Handler, *auth.Principal) {
	t.Helper()
	got := &auth.Principal{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := api.New(cfg, meta, &fakeGraphs{}, log).Authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			t.Error("no principal in context")
		}
		*got = p
		w.WriteHeader(http.StatusOK)
	}))
	return h, got
}

// req issues a request carrying the given Authorization header verbatim; an
// empty value sends no header at all.
func req(t *testing.T, h http.Handler, method, target, authz string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	if authz != "" {
		r.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestBadBearerIsRefused(t *testing.T) {
	h := newAuthedServer(t)
	cases := []struct {
		name  string
		authz string
	}{
		{"no header", ""},
		{"empty bearer", "Bearer "},
		{"wrong token", "Bearer wrongwrongwrongwrongwrongwrong"},
		{"right token, wrong scheme", "Basic " + testToken},
		{"bare token without scheme", testToken},
		{"token as a prefix of the header", "Bearer " + testToken + "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := req(t, h, http.MethodGet, "/api/v1/snapshots", tc.authz)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer ") {
				t.Errorf("WWW-Authenticate = %q, want a Bearer challenge", got)
			}
		})
	}
}

func TestNoCredentialsSaysNotSignedIn(t *testing.T) {
	rec := req(t, newAuthedServer(t), http.MethodGet, "/api/v1/snapshots", "")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"not signed in"`) {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestBearerAloneIsService(t *testing.T) {
	h, got := principalProbe(t, authCfg(), &fakeMeta{})
	// RFC 7235 makes the scheme case-insensitive.
	for _, scheme := range []string{"Bearer ", "bearer "} {
		if rec := req(t, h, http.MethodGet, "/", scheme+testToken); rec.Code != http.StatusOK {
			t.Fatalf("%q status = %d", scheme, rec.Code)
		}
		if got.Kind != auth.KindService {
			t.Errorf("kind = %q, want service", got.Kind)
		}
	}
}

func TestBearerWithActingUser(t *testing.T) {
	meta := &fakeMeta{}
	meta.addUser(model.User{ID: "u1", Username: "alice", Role: "creator"})
	h, got := principalProbe(t, authCfg(), meta)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set("X-Acting-User", "u1")
	r.Header.Set("X-Role", "admin") // no header can set the role
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got.Kind != auth.KindUser || got.UserID != "u1" || got.Role != auth.RoleCreator {
		t.Errorf("principal = %+v", *got)
	}
}

func TestUnknownActingUser(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set("X-Acting-User", "nobody")
	rec := httptest.NewRecorder()
	newAuthedServer(t).ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "acting user not found") {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestActingUserWithoutBearerIsIgnored(t *testing.T) {
	meta := &fakeMeta{}
	meta.addUser(model.User{ID: "u1", Username: "alice", Role: "admin"})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", nil)
	r.Header.Set("X-Acting-User", "u1")
	rec := httptest.NewRecorder()
	newAuthedServerWith(t, meta).ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"not signed in"`) {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}

// The probes have to answer an unauthenticated kubelet.
func TestProbesSkipAuth(t *testing.T) {
	h := newAuthedServer(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		if rec := req(t, h, http.MethodGet, path, ""); rec.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", path, rec.Code)
		}
	}
}

// A preflight cannot carry credentials, so it must not be answered with 401.
func TestPreflightSkipsAuth(t *testing.T) {
	h := newAuthedServer(t)
	r := httptest.NewRequest(http.MethodOptions, "/api/v1/snapshots", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	r.Header.Set("Access-Control-Request-Method", "GET")
	r.Header.Set("Access-Control-Request-Headers", "Authorization, X-Acting-User")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code == http.StatusUnauthorized {
		t.Fatal("preflight was rejected with 401")
	}
	got := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(got, "authorization") || !strings.Contains(got, "x-acting-user") {
		t.Errorf("Allow-Headers = %q", got)
	}
}
