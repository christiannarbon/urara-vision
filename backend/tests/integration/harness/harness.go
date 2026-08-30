//go:build integration

// Package harness connects the integration suites to real databases.
//
// These tests are behind the "integration" build tag and are skipped unless
// they are told where to connect, so `go test ./...` stays fast and needs
// nothing running. Point them at the docker-compose stack with:
//
//	make test-integration
//
// Isolation is by snapshot ID rather than by database: both stores are
// snapshot-scoped and replace whatever was previously written under the same
// ID, so each test gets its own ID and deletes it afterwards. That means the
// suites can run against a shared instance without colliding, and cannot
// disturb snapshots they did not create.
package harness

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
	neostore "urara-vision/backend/internal/store/neo4j"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/fixtures"
)

// Env var names, kept here so the skip messages and the Makefile agree.
const (
	EnvPostgresDSN   = "TEST_POSTGRES_DSN"
	EnvNeo4jURI      = "TEST_NEO4J_URI"
	EnvNeo4jUser     = "TEST_NEO4J_USER"
	EnvNeo4jPassword = "TEST_NEO4J_PASSWORD"
)

// Context returns a context with a deadline generous enough for a cold
// container but short enough that a wedged connection fails the test rather
// than hanging the run.
func Context(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// SnapshotID returns a fresh snapshot ID for one test to own.
func SnapshotID() string { return "test-" + uuid.NewString() }

// PostgresDSN returns where to connect, or skips the test if nothing said.
func PostgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv(EnvPostgresDSN)
	if dsn == "" {
		t.Skipf("set %s to run this test (see: make test-integration)", EnvPostgresDSN)
	}
	return dsn
}

// Postgres opens a migrated store, or skips the test if no DSN was given.
func Postgres(t *testing.T) *postgres.Store {
	t.Helper()
	dsn := PostgresDSN(t)

	ctx := Context(t)
	store, err := postgres.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(store.Close)

	if err := store.Ping(ctx); err != nil {
		t.Fatalf("ping postgres at %s: %v", redact(dsn), err)
	}
	// The schema is idempotent, so every run can assert it is present rather
	// than depending on a migration having been applied out of band.
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
	return store
}

// Neo4j opens a store with its constraints in place, or skips the test.
func Neo4j(t *testing.T) *neostore.Store {
	t.Helper()
	uri := os.Getenv(EnvNeo4jURI)
	if uri == "" {
		t.Skipf("set %s to run this test (see: make test-integration)", EnvNeo4jURI)
	}
	user := os.Getenv(EnvNeo4jUser)
	if user == "" {
		user = "neo4j"
	}

	ctx := Context(t)
	store, err := neostore.New(uri, user, os.Getenv(EnvNeo4jPassword))
	if err != nil {
		t.Fatalf("connect neo4j: %v", err)
	}
	t.Cleanup(func() { _ = store.Close(context.Background()) })

	if err := store.Ping(ctx); err != nil {
		t.Fatalf("ping neo4j at %s: %v", uri, err)
	}
	if err := store.EnsureConstraints(ctx); err != nil {
		t.Fatalf("ensure neo4j constraints: %v", err)
	}
	return store
}

// SavedModel ingests the star-schema fixture into Postgres under a fresh
// snapshot ID and removes it when the test ends.
func SavedModel(t *testing.T, ctx context.Context, pg *postgres.Store) *model.Model {
	t.Helper()
	m := fixtures.BuildAs(SnapshotID(), fixtures.StarSchema())
	if err := pg.SaveSnapshot(ctx, m); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	t.Cleanup(func() {
		// ErrNotFound is fine: a test may have deleted the snapshot itself.
		err := pg.DeleteSnapshot(context.Background(), m.Snapshot.ID)
		if err != nil && !errors.Is(err, postgres.ErrNotFound) {
			t.Errorf("cleanup: delete snapshot %s: %v", m.Snapshot.ID, err)
		}
	})
	return m
}

// ProjectedModel projects the star-schema fixture into Neo4j under a fresh
// snapshot ID and removes it when the test ends.
func ProjectedModel(t *testing.T, ctx context.Context, gs *neostore.Store) (*model.Model, []graph.Edge) {
	t.Helper()
	m := fixtures.BuildAs(SnapshotID(), fixtures.StarSchema())
	edges := graph.Edges(m)
	if err := gs.Project(ctx, m, edges); err != nil {
		t.Fatalf("project graph: %v", err)
	}
	t.Cleanup(func() {
		if err := gs.DeleteSnapshot(context.Background(), m.Snapshot.ID); err != nil {
			t.Errorf("cleanup: delete projection %s: %v", m.Snapshot.ID, err)
		}
	})
	return m, edges
}

// redact hides the password in a DSN so a failure message can name the target
// without printing a credential into CI logs.
func redact(dsn string) string {
	at := -1
	for i, c := range dsn {
		if c == '@' {
			at = i
		}
	}
	if at < 0 {
		return dsn
	}
	scheme := 0
	for i := 0; i+2 < len(dsn); i++ {
		if dsn[i] == ':' && dsn[i+1] == '/' && dsn[i+2] == '/' {
			scheme = i + 3
			break
		}
	}
	return dsn[:scheme] + "***" + dsn[at:]
}
