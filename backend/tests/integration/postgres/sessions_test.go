//go:build integration

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/integration/harness"
)

func sessionHash() string { return "hash-" + uuid.NewString() }

func TestSessionUserSeesCurrentRole(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	u := createdUser(t, ctx, pg, "h")
	hash := sessionHash()
	if err := pg.CreateSession(ctx, hash, u.ID, future()); err != nil {
		t.Fatal(err)
	}

	got, err := pg.SessionUser(ctx, hash)
	if err != nil || got.ID != u.ID || got.Role != "viewer" {
		t.Fatalf("SessionUser = %+v, %v", got, err)
	}

	execSQL(t, `UPDATE users SET role = 'admin' WHERE id = $1`, u.ID)
	if got, err := pg.SessionUser(ctx, hash); err != nil || got.Role != "admin" {
		t.Errorf("after role change = %+v, %v", got, err)
	}
}

func TestExpiredSessionIsNotFound(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	u := createdUser(t, ctx, pg, "h")
	hash := sessionHash()
	if err := pg.CreateSession(ctx, hash, u.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	if _, err := pg.SessionUser(ctx, hash); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("expired err = %v", err)
	}
	if _, err := pg.SessionUser(ctx, sessionHash()); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("missing err = %v", err)
	}
	if n, err := pg.DeleteExpiredSessions(ctx); err != nil || n < 1 {
		t.Errorf("DeleteExpiredSessions = %d, %v", n, err)
	}
	if n := countRows(t, `SELECT count(*) FROM sessions WHERE token_hash = $1`, hash); n != 0 {
		t.Errorf("expired session not deleted")
	}
}

func TestDeleteSession(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	u := createdUser(t, ctx, pg, "h")
	hash := sessionHash()
	if err := pg.CreateSession(ctx, hash, u.ID, future()); err != nil {
		t.Fatal(err)
	}
	if err := pg.DeleteSession(ctx, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.SessionUser(ctx, hash); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestDeleteUserSessionsKeepsOne(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	u := createdUser(t, ctx, pg, "h")
	keep := sessionHash()
	for _, h := range []string{keep, sessionHash(), sessionHash()} {
		if err := pg.CreateSession(ctx, h, u.ID, future()); err != nil {
			t.Fatal(err)
		}
	}

	if err := pg.DeleteUserSessions(ctx, u.ID, keep); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, `SELECT count(*) FROM sessions WHERE user_id = $1`, u.ID); n != 1 {
		t.Errorf("%d sessions left, want 1", n)
	}
	if _, err := pg.SessionUser(ctx, keep); err != nil {
		t.Errorf("kept session: %v", err)
	}

	if err := pg.DeleteUserSessions(ctx, u.ID, ""); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, `SELECT count(*) FROM sessions WHERE user_id = $1`, u.ID); n != 0 {
		t.Errorf("%d sessions left, want 0", n)
	}
}
