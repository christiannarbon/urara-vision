// Package neo4j projects a parsed snapshot into a property graph and answers
// the traversal queries the visualiser needs: neighbourhoods, join paths
// between two tables, and upstream lineage.
package neo4j

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Store wraps a Neo4j driver.
type Store struct {
	driver neo4j.DriverWithContext
}

// New opens a driver. It does not connect until first use; call Ping to verify.
func New(uri, user, password string) (*Store, error) {
	d, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, password, ""),
		func(c *neo4j.Config) {
			c.MaxConnectionPoolSize = 16
			c.MaxConnectionLifetime = time.Hour
		})
	if err != nil {
		return nil, fmt.Errorf("open neo4j driver: %w", err)
	}
	return &Store{driver: d}, nil
}

// Close shuts the driver down.
func (s *Store) Close(ctx context.Context) error { return s.driver.Close(ctx) }

// Ping verifies connectivity.
func (s *Store) Ping(ctx context.Context) error { return s.driver.VerifyConnectivity(ctx) }

// EnsureConstraints creates the uniqueness constraints and indexes the
// projection relies on. It is idempotent.
func (s *Store) EnsureConstraints(ctx context.Context) error {
	stmts := []string{
		`CREATE CONSTRAINT table_key IF NOT EXISTS
		 FOR (t:Table) REQUIRE (t.snapshotId, t.id) IS UNIQUE`,
		`CREATE CONSTRAINT domain_key IF NOT EXISTS
		 FOR (d:Domain) REQUIRE (d.snapshotId, d.id) IS UNIQUE`,
		`CREATE CONSTRAINT source_key IF NOT EXISTS
		 FOR (s:Source) REQUIRE (s.snapshotId, s.id) IS UNIQUE`,
		`CREATE INDEX table_snapshot IF NOT EXISTS FOR (t:Table) ON (t.snapshotId)`,
		`CREATE INDEX table_name IF NOT EXISTS FOR (t:Table) ON (t.name)`,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() { _ = session.Close(ctx) }()

	for _, stmt := range stmts {
		if _, err := session.Run(ctx, stmt, nil); err != nil {
			return fmt.Errorf("ensure constraint: %w", err)
		}
	}
	return nil
}
