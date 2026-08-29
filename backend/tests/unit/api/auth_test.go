// The bearer-token gate on /api/v1.
package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"urara-vision/backend/internal/api"
	"urara-vision/backend/internal/config"
)

const testToken = "0123456789abcdef0123456789abcdef"

// newAuthedServer wires a Server that requires testToken.
func newAuthedServer(t *testing.T) http.Handler {
	t.Helper()
	cfg := &config.Config{
		CORSOrigins:    []string{"http://localhost:5173"},
		MaxUploadBytes: 64 << 20,
		MaxFiles:       100,
		APIToken:       testToken,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.New(cfg, &fakeMeta{}, &fakeGraphs{}, log).Routes()
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

func TestAPIRequiresToken(t *testing.T) {
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

func TestAPIAcceptsToken(t *testing.T) {
	h := newAuthedServer(t)

	rec := req(t, h, http.MethodGet, "/api/v1/snapshots", "Bearer "+testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// RFC 7235 makes the scheme case-insensitive.
	rec = req(t, h, http.MethodGet, "/api/v1/snapshots", "bearer "+testToken)
	if rec.Code != http.StatusOK {
		t.Errorf("lowercase scheme: status = %d, want 200", rec.Code)
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

// A preflight cannot carry Authorization, so it must not be answered with 401.
func TestPreflightSkipsAuth(t *testing.T) {
	h := newAuthedServer(t)
	r := httptest.NewRequest(http.MethodOptions, "/api/v1/snapshots", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	r.Header.Set("Access-Control-Request-Method", "GET")
	r.Header.Set("Access-Control-Request-Headers", "Authorization")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code == http.StatusUnauthorized {
		t.Fatal("preflight was rejected with 401")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "authorization") {
		t.Errorf("Allow-Headers = %q, want it to include Authorization", got)
	}
}

// An empty APIToken is the documented local-development mode.
func TestNoTokenConfiguredLeavesAPIOpen(t *testing.T) {
	h := newServer(t, &fakeMeta{}, &fakeGraphs{})
	if rec := req(t, h, http.MethodGet, "/api/v1/snapshots", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 when no token is configured", rec.Code)
	}
}
