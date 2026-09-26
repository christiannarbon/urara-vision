//go:build integration

package api_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/auth"
	"urara-vision/backend/tests/integration/harness"
)

func TestLoginMeLogout(t *testing.T) {
	base := stack(t)
	ctx := harness.Context(t)
	pg := harness.Postgres(t)

	username := "test-" + uuid.NewString()[:8]
	const password = "integration-password-1"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	u, err := pg.CreatePasswordUser(ctx, username, "", "viewer", hash)
	if err != nil {
		t.Fatalf("CreatePasswordUser: %v", err)
	}
	t.Cleanup(func() {
		conn, err := pgx.Connect(context.Background(), harness.PostgresDSN(t))
		if err != nil {
			t.Errorf("cleanup connect: %v", err)
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		_, _ = conn.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, u.ID)
	})

	var session *http.Cookie
	send := func(method, path, body string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "urara")
		if session != nil {
			req.AddCookie(session)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}

	res, body := send(http.MethodPost, "/api/v1/auth/login", `{"username":"`+username+`","password":"`+password+`"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login = %d: %s", res.StatusCode, body)
	}
	for _, c := range res.Cookies() {
		if c.Name == "urara_session" {
			session = &http.Cookie{Name: c.Name, Value: c.Value}
		}
	}
	if session == nil {
		t.Fatal("no session cookie")
	}

	if res, body := send(http.MethodGet, "/api/v1/auth/me", ""); res.StatusCode != http.StatusOK ||
		!strings.Contains(body, `"id":"`+u.ID+`"`) || !strings.Contains(body, `"kind":"user"`) {
		t.Fatalf("me = %d: %s", res.StatusCode, body)
	}
	if res, _ := send(http.MethodPost, "/api/v1/auth/logout", ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d", res.StatusCode)
	}
	// Same cookie again: the session row is gone.
	if res, _ := send(http.MethodGet, "/api/v1/auth/me", ""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("me after logout = %d", res.StatusCode)
	}
}
