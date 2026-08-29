// Snapshot-level reads: listing ingests, and finding the newest one.
package postgres

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
)

// ListSnapshots returns every snapshot, newest first.
func (s *Store) ListSnapshots(ctx context.Context) ([]model.Snapshot, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, source_label, created_at, stats FROM snapshots ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Snapshot{}
	for rows.Next() {
		var sn model.Snapshot
		var stats []byte
		if err := rows.Scan(&sn.ID, &sn.Name, &sn.SourceLabel, &sn.CreatedAt, &stats); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(stats, &sn.Stats); err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

// GetSnapshot returns one snapshot by ID.
func (s *Store) GetSnapshot(ctx context.Context, id string) (*model.Snapshot, error) {
	var sn model.Snapshot
	var stats []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, source_label, created_at, stats FROM snapshots WHERE id = $1`, id).
		Scan(&sn.ID, &sn.Name, &sn.SourceLabel, &sn.CreatedAt, &stats)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(stats, &sn.Stats); err != nil {
		return nil, err
	}
	return &sn, nil
}

// DeleteSnapshot removes a snapshot and everything hanging off it.
func (s *Store) DeleteSnapshot(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM snapshots WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LatestSnapshotID returns the most recent snapshot, or ErrNotFound.
func (s *Store) LatestSnapshotID(ctx context.Context) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id FROM snapshots ORDER BY created_at DESC LIMIT 1`).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	return id, nil
}
