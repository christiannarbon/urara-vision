//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/integration/harness"
)

func future() time.Time { return time.Now().Add(time.Hour) }

func uniqueUsername() string { return "test-" + uuid.NewString()[:8] }

// createdUser creates a password user and deletes it when the test ends.
func createdUser(t *testing.T, ctx context.Context, pg *postgres.Store, hash string) *model.User {
	t.Helper()
	u, err := pg.CreatePasswordUser(ctx, uniqueUsername(), "Test User", "viewer", hash)
	if err != nil {
		t.Fatalf("CreatePasswordUser: %v", err)
	}
	t.Cleanup(func() { execSQL(t, `DELETE FROM users WHERE id = $1`, u.ID) })
	return u
}

func countRows(t *testing.T, sql string, args ...any) int {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, harness.PostgresDSN(t))
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	var n int
	if err := conn.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestCreateThenPasswordIdentity(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	u := createdUser(t, ctx, pg, "$2a$12$exacthash")

	got, hash, err := pg.PasswordIdentity(ctx, u.Username)
	if err != nil {
		t.Fatalf("PasswordIdentity: %v", err)
	}
	if hash != "$2a$12$exacthash" {
		t.Errorf("hash = %q", hash)
	}
	if *got != *u {
		t.Errorf("user = %+v, want %+v", got, u)
	}
	if g, err := pg.GetUser(ctx, u.ID); err != nil || g.Username != u.Username {
		t.Errorf("GetUser = %+v, %v", g, err)
	}
}

func TestUnknownUserIsNotFound(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	if _, _, err := pg.PasswordIdentity(ctx, uniqueUsername()); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("PasswordIdentity err = %v", err)
	}
	if _, err := pg.GetUser(ctx, uuid.NewString()); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("GetUser err = %v", err)
	}
}

func TestDuplicateUsernameConflictsAndRollsBack(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	u := createdUser(t, ctx, pg, "h")

	if _, err := pg.CreatePasswordUser(ctx, u.Username, "", "viewer", "h"); !errors.Is(err, postgres.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	if n := countRows(t, `SELECT count(*) FROM users WHERE username = $1`, u.Username); n != 1 {
		t.Errorf("%d users rows, want 1", n)
	}
}

// An identity whose subject is taken fails after the users insert; that row must roll back.
func TestIdentityConflictLeavesNoOrphanUser(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	taken := uniqueUsername()
	fresh := uniqueUsername()
	execSQL(t, `INSERT INTO users (id, username, role) VALUES ($1, $2, 'viewer')`, taken, taken)
	execSQL(t, `INSERT INTO user_identities (id, user_id, provider, subject) VALUES ($1, $1, 'password', $2)`, taken, fresh)
	t.Cleanup(func() { execSQL(t, `DELETE FROM users WHERE id = $1`, taken) })

	if _, err := pg.CreatePasswordUser(ctx, fresh, "", "viewer", "h"); !errors.Is(err, postgres.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	if n := countRows(t, `SELECT count(*) FROM users WHERE username = $1`, fresh); n != 0 {
		t.Errorf("%d orphan users rows", n)
	}
}

func TestSetPasswordHash(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	u := createdUser(t, ctx, pg, "old")

	if err := pg.SetPasswordHash(ctx, u.ID, "new"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	if _, hash, _ := pg.PasswordIdentity(ctx, u.Username); hash != "new" {
		t.Errorf("hash = %q", hash)
	}
	if err := pg.SetPasswordHash(ctx, uuid.NewString(), "x"); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("unknown user err = %v", err)
	}
}

func TestDeletingUserCascades(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	u := createdUser(t, ctx, pg, "h")
	if err := pg.CreateSession(ctx, "hash-"+u.ID, u.ID, future()); err != nil {
		t.Fatal(err)
	}

	execSQL(t, `DELETE FROM users WHERE id = $1`, u.ID)

	if n := countRows(t, `SELECT count(*) FROM user_identities WHERE user_id = $1`, u.ID); n != 0 {
		t.Errorf("%d identities left", n)
	}
	if n := countRows(t, `SELECT count(*) FROM sessions WHERE user_id = $1`, u.ID); n != 0 {
		t.Errorf("%d sessions left", n)
	}
}

// Needs an empty users table, so it runs in a schema of its own.
func TestBootstrapAdminCreatesExactlyOne(t *testing.T) {
	ctx := harness.Context(t)
	dsn := harness.PostgresDSN(t)

	schema := "bootstrap_race_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})

	pg, err := postgres.New(ctx, inSchema(t, dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const starters = 5
	var wg sync.WaitGroup
	created := make(chan bool, starters)
	errs := make(chan error, starters)
	for range starters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := pg.BootstrapAdmin(ctx, "admin", "h")
			created <- ok
			errs <- err
		}()
	}
	wg.Wait()
	close(created)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("BootstrapAdmin: %v", err)
		}
	}
	n := 0
	for ok := range created {
		if ok {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d calls reported creating the admin, want 1", n)
	}
	if c, _ := pg.CountUsers(ctx); c != 1 {
		t.Errorf("%d users, want 1", c)
	}
	u, _, err := pg.PasswordIdentity(ctx, "admin")
	if err != nil || u.Role != "admin" {
		t.Errorf("admin = %+v, %v", u, err)
	}
}
