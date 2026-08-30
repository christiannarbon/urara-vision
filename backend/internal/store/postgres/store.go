// Package postgres is the record of truth: it holds every parsed document in
// enough detail to rebuild the UI's detail panes, plus a full-text index.
//
// The graph projection lives in the neo4j package; nothing here answers a
// traversal question.
package postgres

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// Store wraps a pgx pool.
type Store struct {
	pool *pgxpool.Pool
}

// New opens a pool and waits for the database to accept connections.
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	cfg.MaxConns = 8
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping reports whether the database is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// migrateLock identifies the advisory lock Migrate holds. The value is
// arbitrary and matters only in that nothing else in this database uses it.
const migrateLock int64 = 0x75726172 // "urar"

// Migrate applies the embedded schema. It is idempotent, and safe to run from
// several processes at once.
//
// The concurrency is not hypothetical: the backend runs more than one replica
// and every one of them migrates as it starts, so a rollout has them arriving
// together. `CREATE TABLE IF NOT EXISTS` does not survive that on its own --
// two sessions both find the table absent, both create it, and the loser fails
// on a duplicate key in pg_type. The advisory lock is held to the end of the
// transaction, so the second migrator applies the schema only once the first
// has committed, by which point every statement is the no-op it claims to be.
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, migrateLock); err != nil {
		return fmt.Errorf("take migration lock: %w", err)
	}
	if _, err := tx.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return tx.Commit(ctx)
}
