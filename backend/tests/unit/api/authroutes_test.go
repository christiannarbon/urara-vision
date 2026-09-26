package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"urara-vision/backend/internal/api"
	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/config"
	"urara-vision/backend/internal/model"
)

const alicePassword = "alice-password-123"

var (
	aliceHashOnce sync.Once
	aliceHash     string
)

// aliceMeta has alice (creator) with a password and a live session for cookieToken.
func aliceMeta(t *testing.T) *fakeMeta {
	t.Helper()
	aliceHashOnce.Do(func() {
		h, err := auth.HashPassword(alicePassword)
		if err != nil {
			t.Fatal(err)
		}
		aliceHash = h
	})
	meta := &fakeMeta{}
	meta.addUser(model.User{ID: "u1", Username: "alice", DisplayName: "Alice", Role: "creator"})
	meta.passwords = map[string]string{"u1": aliceHash}
	meta.addSession(auth.HashToken(cookieToken), "u1")
	return meta
}

func routesServer(t *testing.T, meta *fakeMeta, mutate ...func(*config.Config)) http.Handler {
	t.Helper()
	cfg := authCfg()
	cfg.SessionTTL = time.Hour
	cfg.CookieSecure = true
	for _, m := range mutate {
		m(cfg)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.New(cfg, meta, &fakeGraphs{}, log).Routes()
}

func login(h http.Handler, username, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	return serve(h, r)
}

func sessionCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "urara_session" {
			return c
		}
	}
	t.Fatalf("no urara_session cookie in %v", rec.Header().Values("Set-Cookie"))
	return nil
}

func TestLoginSucceeds(t *testing.T) {
	meta := aliceMeta(t)
	rec := login(routesServer(t, meta), "Alice", alicePassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct{ User model.User }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.User.ID != "u1" {
		t.Errorf("body = %s", rec.Body)
	}
	c := sessionCookieFrom(t, rec)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Expires.IsZero() {
		t.Errorf("cookie = %+v", c)
	}
	if strings.Contains(rec.Body.String(), c.Value) {
		t.Error("session token leaked into the body")
	}
	if meta.sessions[auth.HashToken(c.Value)] != "u1" {
		t.Error("session not stored under the token's hash")
	}
}

func TestCookieSecureFollowsConfig(t *testing.T) {
	h := routesServer(t, aliceMeta(t), func(c *config.Config) { c.CookieSecure = false })
	if c := sessionCookieFrom(t, login(h, "alice", alicePassword)); c.Secure {
		t.Error("Secure set with COOKIE_SECURE=false")
	}
}

func TestLoginFailuresAreIdentical(t *testing.T) {
	h := routesServer(t, aliceMeta(t))
	unknown := login(h, "nobody", alicePassword)
	wrong := login(h, "alice", "wrong-password-123")
	invalid := login(h, "a b", alicePassword)
	for _, rec := range []*httptest.ResponseRecorder{unknown, wrong, invalid} {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d", rec.Code)
		}
	}
	if unknown.Body.String() != wrong.Body.String() || wrong.Body.String() != invalid.Body.String() {
		t.Errorf("bodies differ:\n%s%s%s", unknown.Body, wrong.Body, invalid.Body)
	}
	if !strings.Contains(wrong.Body.String(), `"invalid username or password"`) {
		t.Errorf("body = %s", wrong.Body)
	}
}

func TestSixthFailureIsRateLimited(t *testing.T) {
	h := routesServer(t, aliceMeta(t))
	for i := range 5 {
		if rec := login(h, "alice", "wrong-password-123"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: status = %d", i+1, rec.Code)
		}
	}
	rec := login(h, "alice", alicePassword)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("6th: status = %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestSuccessResetsUsernameCounter(t *testing.T) {
	h := routesServer(t, aliceMeta(t))
	for range 4 {
		login(h, "alice", "wrong-password-123")
	}
	if rec := login(h, "alice", alicePassword); rec.Code != http.StatusOK {
		t.Fatalf("login = %d", rec.Code)
	}
	for i := range 5 {
		if rec := login(h, "alice", "wrong-password-123"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d after reset: status = %d", i+1, rec.Code)
		}
	}
}

// cookieReq is an authenticated cookie request carrying the CSRF header.
func cookieReq(method, target, body string) *http.Request {
	r := withCookie(httptest.NewRequest(method, target, strings.NewReader(body)), cookieToken)
	r.Header.Set("X-Requested-With", "urara")
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestLogoutDeletesSessionAndClearsCookie(t *testing.T) {
	meta := aliceMeta(t)
	rec := serve(routesServer(t, meta), cookieReq(http.MethodPost, "/api/v1/auth/logout", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(meta.sessions) != 0 {
		t.Error("session not deleted")
	}
	if c := sessionCookieFrom(t, rec); c.MaxAge >= 0 {
		t.Errorf("cookie not cleared: %+v", c)
	}
}

func TestLogoutAsServiceDoesNothing(t *testing.T) {
	meta := aliceMeta(t)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	rec := serve(routesServer(t, meta), r)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Set-Cookie") != "" || len(meta.sessions) != 1 {
		t.Errorf("status = %d, set-cookie %q, sessions %d", rec.Code, rec.Header().Get("Set-Cookie"), len(meta.sessions))
	}
}

type meBody struct {
	User        *model.User       `json:"user"`
	Kind        string            `json:"kind"`
	Permissions []auth.Permission `json:"permissions"`
}

func getMe(t *testing.T, h http.Handler, r *http.Request) meBody {
	t.Helper()
	rec := serve(h, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var b meBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMe(t *testing.T) {
	meta := aliceMeta(t)

	user := getMe(t, routesServer(t, meta), cookieReq(http.MethodGet, "/api/v1/auth/me", ""))
	if user.Kind != "user" || user.User == nil || user.User.ID != "u1" ||
		len(user.Permissions) != len(auth.Permissions(auth.RoleCreator)) {
		t.Errorf("user me = %+v", user)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	svc := getMe(t, routesServer(t, meta), r)
	if svc.Kind != "service" || svc.User != nil || len(svc.Permissions) != 4 {
		t.Errorf("service me = %+v", svc)
	}

	anonH := routesServer(t, meta, func(c *config.Config) { c.AuthDisabled = true })
	anon := getMe(t, anonH, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if anon.Kind != "anonymous" || anon.User != nil || len(anon.Permissions) != len(auth.Permissions(auth.RoleAdmin)) {
		t.Errorf("anonymous me = %+v", anon)
	}
}

func TestPasswordChange(t *testing.T) {
	t.Run("wrong current", func(t *testing.T) {
		rec := serve(routesServer(t, aliceMeta(t)), cookieReq(http.MethodPost, "/api/v1/auth/password",
			`{"current":"wrong-password-123","new":"another-password-456"}`))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("weak new", func(t *testing.T) {
		rec := serve(routesServer(t, aliceMeta(t)), cookieReq(http.MethodPost, "/api/v1/auth/password",
			`{"current":"`+alicePassword+`","new":"short"}`))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "12 to 72 bytes") {
			t.Errorf("status = %d, body %s", rec.Code, rec.Body)
		}
	})
	t.Run("success", func(t *testing.T) {
		meta := aliceMeta(t)
		meta.addSession("other-session", "u1")
		rec := serve(routesServer(t, meta), cookieReq(http.MethodPost, "/api/v1/auth/password",
			`{"current":"`+alicePassword+`","new":"another-password-456"}`))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		if !auth.VerifyPassword(meta.passwords["u1"], "another-password-456") {
			t.Error("password not changed")
		}
		if _, ok := meta.sessions[auth.HashToken(cookieToken)]; !ok {
			t.Error("current session was deleted")
		}
		if _, ok := meta.sessions["other-session"]; ok {
			t.Error("other session survived")
		}
	})
	t.Run("service is forbidden", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+testToken)
		if rec := serve(routesServer(t, aliceMeta(t)), r); rec.Code != http.StatusForbidden {
			t.Errorf("status = %d", rec.Code)
		}
	})
}

func TestAuthSession(t *testing.T) {
	meta := aliceMeta(t)
	rec := serve(routesServer(t, meta), cookieReq(http.MethodGet, "/api/v1/auth/session", ""))
	if rec.Code != http.StatusNoContent || rec.Header().Get("X-User-Id") != "u1" || rec.Header().Get("X-User-Role") != "creator" {
		t.Errorf("user: status = %d, headers %v", rec.Code, rec.Header())
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	if rec := serve(routesServer(t, meta), r); rec.Code != http.StatusUnauthorized {
		t.Errorf("service: status = %d", rec.Code)
	}
}
