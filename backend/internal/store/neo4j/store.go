// Package neo4j projects a parsed snapshot into a property graph and answers
// the traversal queries the visualiser needs: neighbourhoods, join paths
// between two tables, and upstream lineage.
package neo4j

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
// projection relies on. It is idempotent, and safe to run from several
// processes at once: the deployment starts more than one replica, and the
// integration suites run their packages in parallel, so concurrent callers are
// the normal case rather than the exception.
//
// Concurrency is why each statement runs in its own managed transaction. Two
// clients creating the same schema rule at the same moment deadlock in the
// server's schema locks -- a transient error the driver's retry policy handles
// only for a managed transaction -- and the loser of the race can still be told
// the rule already exists, despite the IF NOT EXISTS. Both mean the schema is
// there, which is all a caller asked for.
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
		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(ctx, stmt, nil)
			if err != nil {
				return nil, err
			}
			// Consume inside the transaction so a failure is this statement's
			// rather than the next one's.
			return res.Consume(ctx)
		})
		if err != nil && !alreadyExists(err) {
			return fmt.Errorf("ensure constraint: %w", err)
		}
	}
	return nil
}

// alreadyExists reports whether an error says the schema rule a statement asked
// for is already present, which a concurrent creation can produce even under
// IF NOT EXISTS.
func alreadyExists(err error) bool {
	var nerr *neo4j.Neo4jError
	if !errors.As(err, &nerr) {
		return false
	}
	return strings.HasPrefix(nerr.Code, "Neo.ClientError.Schema.") &&
		strings.HasSuffix(nerr.Code, "AlreadyExists")
}
