//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/integration/harness"
)

// The last-admin guard counts every admin, so these run in an empty schema.
func adminStore(t *testing.T) (context.Context, *postgres.Store) {
	t.Helper()
	ctx := harness.Context(t)
	pg, _ := freshStore(t, ctx, "users_admin_")
	return ctx, pg
}

func newUser(t *testing.T, ctx context.Context, pg *postgres.Store, role string) *model.User {
	t.Helper()
	u, err := pg.CreatePasswordUser(ctx, uniqueUsername(), "", role, "h")
	if err != nil {
		t.Fatalf("CreatePasswordUser: %v", err)
	}
	return u
}

func ptr(s string) *string { return &s }

func adminCount(t *testing.T, ctx context.Context, pg *postgres.Store) int {
	t.Helper()
	users, err := pg.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, u := range users {
		if u.Role == "admin" {
			n++
		}
	}
	return n
}

func TestListUsersInUsernameOrder(t *testing.T) {
	ctx, pg := adminStore(t)
	for _, name := range []string{"cody", "alice", "bob"} {
		if _, err := pg.CreatePasswordUser(ctx, name, "", "viewer", "h"); err != nil {
			t.Fatal(err)
		}
	}
	users, err := pg.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 || users[0].Username != "alice" || users[1].Username != "bob" || users[2].Username != "cody" {
		t.Errorf("users = %+v", users)
	}
}

func TestUpdateUser(t *testing.T) {
	ctx, pg := adminStore(t)
	u := newUser(t, ctx, pg, "viewer")

	got, err := pg.UpdateUser(ctx, u.ID, nil, ptr("Vera"))
	if err != nil || got.DisplayName != "Vera" || got.Role != "viewer" {
		t.Fatalf("display name only: %+v, %v", got, err)
	}
	got, err = pg.UpdateUser(ctx, u.ID, ptr("creator"), nil)
	if err != nil || got.Role != "creator" || got.DisplayName != "Vera" {
		t.Fatalf("role only: %+v, %v", got, err)
	}
	if _, err := pg.UpdateUser(ctx, "nobody", ptr("viewer"), nil); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("unknown user err = %v", err)
	}
}

func TestDemotingTheOnlyAdminIsRefused(t *testing.T) {
	ctx, pg := adminStore(t)
	a := newUser(t, ctx, pg, "admin")

	if _, err := pg.UpdateUser(ctx, a.ID, ptr("viewer"), nil); !errors.Is(err, postgres.ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}
	if _, err := pg.UpdateUser(ctx, a.ID, ptr("admin"), ptr("Root")); err != nil {
		t.Errorf("keeping admin: %v", err)
	}

	b := newUser(t, ctx, pg, "admin")
	if _, err := pg.UpdateUser(ctx, b.ID, ptr("viewer"), nil); err != nil {
		t.Errorf("demote with two admins: %v", err)
	}
	if n := adminCount(t, ctx, pg); n != 1 {
		t.Errorf("%d admins, want 1", n)
	}
}

// The test holds the admin rows until both demotions are blocked on them, so
// the two transactions always overlap.
func TestConcurrentDemotionsKeepOneAdmin(t *testing.T) {
	ctx := harness.Context(t)
	pg, dsn := freshStore(t, ctx, "users_admin_")
	admins := []*model.User{newUser(t, ctx, pg, "admin"), newUser(t, ctx, pg, "admin")}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	hold, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hold.Exec(ctx, `SELECT id FROM users WHERE role = 'admin' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}

	errs := make([]error, len(admins))
	var wg sync.WaitGroup
	for i, a := range admins {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = pg.UpdateUser(ctx, a.ID, ptr("viewer"), nil)
		}()
	}
	waitForLockWaiters(t, ctx, len(admins))
	if err := hold.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	ok, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, postgres.ErrLastAdmin):
			refused++
		default:
			t.Errorf("UpdateUser: %v", err)
		}
	}
	if ok != 1 || refused != 1 {
		t.Errorf("succeeded %d, refused %d; want 1 and 1", ok, refused)
	}
	if n := adminCount(t, ctx, pg); n != 1 {
		t.Errorf("%d admins, want 1", n)
	}
}

func TestDeletingTheLastAdminIsRefused(t *testing.T) {
	ctx, pg := adminStore(t)
	a := newUser(t, ctx, pg, "admin")

	if err := pg.DeleteUser(ctx, a.ID); !errors.Is(err, postgres.ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}
	if err := pg.DeleteUser(ctx, "nobody"); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("unknown user err = %v", err)
	}
}

func TestDeleteUserCascades(t *testing.T) {
	ctx, pg := adminStore(t)
	newUser(t, ctx, pg, "admin")
	u := newUser(t, ctx, pg, "admin")
	if err := pg.CreateSession(ctx, "hash-"+u.ID, u.ID, future()); err != nil {
		t.Fatal(err)
	}

	if err := pg.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, _, err := pg.PasswordIdentity(ctx, u.Username); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("identity survived: %v", err)
	}
	if _, err := pg.SessionUser(ctx, "hash-"+u.ID); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("session survived: %v", err)
	}
}

func waitForLockWaiters(t *testing.T, ctx context.Context, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		n := countRows(t, `SELECT count(*) FROM pg_stat_activity
		                    WHERE datname = current_database() AND wait_event_type = 'Lock'`)
		if n >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d sessions waiting on a lock, want %d", n, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
