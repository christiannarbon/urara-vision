//go:build integration

// Integration tests for the Postgres store: schema, round-tripping and the
// CASCADE deletes -- the parts no fake can stand in for.
//
// Run with `make test-integration`; without a DSN they skip.
package postgres_test

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/fixtures"
	"urara-vision/backend/tests/integration/harness"
)

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t) // already migrated once
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

// TestMigrateIsSafeConcurrently: `CREATE TABLE IF NOT EXISTS` is not safe
// against a concurrent identical statement -- both sessions find nothing, both
// create, and the loser gets a duplicate key on pg_type. Two backend replicas
// start together and both migrate, and these suites migrate from three packages
// at once, so this is a race that happens rather than one that could.
//
// It only races against an empty schema, which the shared test database is not,
// so the test gives the migrators one of their own to fight over.
func TestMigrateIsSafeConcurrently(t *testing.T) {
	ctx := harness.Context(t)
	dsn := harness.PostgresDSN(t)

	schema := "migrate_race_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	// Registered before the drop below, so it runs after it: cleanups are
	// last-in-first-out, and a closed connection cannot drop anything.
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})

	const migrators = 8
	errs := make(chan error, migrators)
	var start sync.WaitGroup
	start.Add(1)
	for i := 0; i < migrators; i++ {
		go func() {
			store, err := postgres.New(ctx, inSchema(t, dsn, schema))
			if err != nil {
				errs <- err
				return
			}
			defer store.Close()
			// All eight wait here, so they reach the schema together rather
			// than one after another as they connect.
			start.Wait()
			errs <- store.Migrate(ctx)
		}()
	}
	start.Done()

	for i := 0; i < migrators; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Migrate: %v", err)
		}
	}
}

// inSchema points a DSN at one schema, so a connection made with it creates
// and reads tables there rather than in public.
func inSchema(t *testing.T, dsn, schema string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func TestSaveAndReadBackSnapshot(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	snap, err := pg.GetSnapshot(ctx, m.Snapshot.ID)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap.ID != m.Snapshot.ID {
		t.Errorf("id = %q, want %q", snap.ID, m.Snapshot.ID)
	}
	if snap.Stats.Tables != m.Snapshot.Stats.Tables {
		t.Errorf("stats.tables = %d, want %d", snap.Stats.Tables, m.Snapshot.Stats.Tables)
	}
	if snap.CreatedAt.IsZero() {
		t.Error("CreatedAt was not defaulted on write")
	}

	// The manifest is the one thing the documentation states rather than the
	// parser infers, so it has to survive the round trip intact -- including
	// the language list, which is the only field stored as a document.
	if got, want := snap.Project, fixtures.ProjectMeta(); !reflect.DeepEqual(got, want) {
		t.Errorf("project = %+v, want %+v", got, want)
	}

	// The snapshot must be findable in the list and as the newest entry.
	all, err := pg.ListSnapshots(ctx)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	var found bool
	for _, s := range all {
		if s.ID == m.Snapshot.ID {
			found = true
			if got, want := s.Project, fixtures.ProjectMeta(); !reflect.DeepEqual(got, want) {
				t.Errorf("listed project = %+v, want %+v", got, want)
			}
		}
	}
	if !found {
		t.Error("saved snapshot did not appear in ListSnapshots")
	}

	// Nothing else writes during this test, so the snapshot just saved is the
	// most recent one.
	latest, err := pg.LatestSnapshotID(ctx)
	if err != nil {
		t.Fatalf("LatestSnapshotID: %v", err)
	}
	if latest != m.Snapshot.ID {
		t.Errorf("latest = %q, want the snapshot just saved (%q)", latest, m.Snapshot.ID)
	}
}

func TestGetSnapshotNotFound(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)

	if _, err := pg.GetSnapshot(ctx, "no-such-snapshot"); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// TestSaveSnapshotReplacesTheSameID covers re-ingesting the same directory: the
// second write must replace the first rather than duplicating or failing on the
// primary key.
func TestSaveSnapshotReplacesTheSameID(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	before, err := pg.ListTables(ctx, m.Snapshot.ID, "")
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}

	if err := pg.SaveSnapshot(ctx, m); err != nil {
		t.Fatalf("second SaveSnapshot: %v", err)
	}
	after, err := pg.ListTables(ctx, m.Snapshot.ID, "")
	if err != nil {
		t.Fatalf("ListTables after rewrite: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("tables = %d after rewrite, want %d; rows were duplicated or lost",
			len(after), len(before))
	}
}

// TestDeleteSnapshotCascades: every child table hangs off the snapshot, and a
// missing ON DELETE CASCADE would leave orphaned rows that the next ingest of
// the same ID would then collide with.
func TestDeleteSnapshotCascades(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)

	m := harness.SavedModel(t, ctx, pg)
	sid := m.Snapshot.ID

	if err := pg.DeleteSnapshot(ctx, sid); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	if _, err := pg.GetSnapshot(ctx, sid); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("GetSnapshot after delete = %v, want ErrNotFound", err)
	}

	tables, err := pg.ListTables(ctx, sid, "")
	if err != nil {
		t.Fatalf("ListTables after delete: %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("%d tables outlived their snapshot", len(tables))
	}
	domains, err := pg.ListDomains(ctx, sid)
	if err != nil {
		t.Fatalf("ListDomains after delete: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("%d domains outlived their snapshot", len(domains))
	}
	diags, err := pg.ListDiagnostics(ctx, sid, "")
	if err != nil {
		t.Fatalf("ListDiagnostics after delete: %v", err)
	}
	if len(diags) != 0 {
		t.Errorf("%d diagnostics outlived their snapshot", len(diags))
	}
	sources, err := pg.ListSourceTables(ctx, sid)
	if err != nil {
		t.Fatalf("ListSourceTables after delete: %v", err)
	}
	if len(sources) != 0 {
		t.Errorf("%d source tables outlived their snapshot", len(sources))
	}

	// Deleting an absent snapshot reports ErrNotFound rather than succeeding
	// silently, which is what lets DELETE /snapshots/{sid} answer 404 for an ID
	// that was never there.
	if err := pg.DeleteSnapshot(ctx, sid); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("deleting an already-deleted snapshot = %v, want ErrNotFound", err)
	}
}
