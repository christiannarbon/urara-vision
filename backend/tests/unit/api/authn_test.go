// Session cookies, AUTH_DISABLED and the CSRF header.
package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/model"
)

const cookieToken = "session-token"

// cookieMeta has one viewer with a live session for cookieToken.
func cookieMeta() *fakeMeta {
	meta := &fakeMeta{}
	meta.addUser(model.User{ID: "u1", Username: "alice", DisplayName: "Alice", Role: "viewer"})
	meta.addSession(auth.HashToken(cookieToken), "u1")
	return meta
}

func withCookie(r *http.Request, token string) *http.Request {
	r.AddCookie(&http.Cookie{Name: "urara_session", Value: token})
	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestAuthDisabledIsAnonymous(t *testing.T) {
	cfg := authCfg()
	cfg.AuthDisabled = true
	h, got := principalProbe(t, cfg, &fakeMeta{})
	if rec := serve(h, httptest.NewRequest(http.MethodGet, "/", nil)); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got.Kind != auth.KindAnonymous {
		t.Errorf("kind = %q", got.Kind)
	}
}

func TestSessionCookieIsUser(t *testing.T) {
	h, got := principalProbe(t, authCfg(), cookieMeta())
	rec := serve(h, withCookie(httptest.NewRequest(http.MethodGet, "/", nil), cookieToken))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	want := auth.Principal{Kind: auth.KindUser, UserID: "u1", Username: "alice", DisplayName: "Alice", Role: auth.RoleViewer}
	if *got != want {
		t.Errorf("principal = %+v", *got)
	}
}

func TestExpiredSessionClearsCookie(t *testing.T) {
	h := newAuthedServerWith(t, cookieMeta())
	rec := serve(h, withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", nil), "expired-token"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	c := rec.Header().Get("Set-Cookie")
	if !strings.HasPrefix(c, "urara_session=;") || !strings.Contains(c, "Max-Age=0") {
		t.Errorf("Set-Cookie = %q, want urara_session cleared", c)
	}
}

func TestWrongBearerWithValidCookie(t *testing.T) {
	r := withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", nil), cookieToken)
	r.Header.Set("Authorization", "Bearer wrongwrongwrongwrongwrongwrong")
	if rec := serve(newAuthedServerWith(t, cookieMeta()), r); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestCookieWriteNeedsCSRFHeader(t *testing.T) {
	h, _ := principalProbe(t, authCfg(), cookieMeta())

	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := serve(h, withCookie(httptest.NewRequest(m, "/", nil), cookieToken))
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "missing X-Requested-With header") {
			t.Errorf("%s without header: status = %d, body %s", m, rec.Code, rec.Body)
		}
	}

	r := withCookie(httptest.NewRequest(http.MethodDelete, "/", nil), cookieToken)
	r.Header.Set("X-Requested-With", "urara")
	if rec := serve(h, r); rec.Code != http.StatusOK {
		t.Errorf("DELETE with header: status = %d", rec.Code)
	}

	if rec := serve(h, withCookie(httptest.NewRequest(http.MethodGet, "/", nil), cookieToken)); rec.Code != http.StatusOK {
		t.Errorf("GET without header: status = %d", rec.Code)
	}
}

func TestBearerWriteSkipsCSRFHeader(t *testing.T) {
	h, _ := principalProbe(t, authCfg(), &fakeMeta{})
	r := httptest.NewRequest(http.MethodDelete, "/", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	if rec := serve(h, r); rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestHealthzIgnoresBadCredentials(t *testing.T) {
	r := withCookie(httptest.NewRequest(http.MethodGet, "/healthz", nil), "expired-token")
	r.Header.Set("Authorization", "Bearer wrong")
	if rec := serve(newAuthedServerWith(t, cookieMeta()), r); rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}
