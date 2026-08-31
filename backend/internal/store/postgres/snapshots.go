// Snapshot-level reads: listing ingests, and finding the newest one.
package postgres

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
)

// snapshotColumns is every field a snapshot is rebuilt from, shared by the two
// reads below so a new one cannot be added to only half of them.
const snapshotColumns = `id, name, source_label, created_at, stats,
	project_name, project_version, project_description,
	i18n_primary, i18n_supported, i18n_type`

// scanSnapshot reads one row of snapshotColumns.
func scanSnapshot(row pgx.Row) (model.Snapshot, error) {
	var sn model.Snapshot
	var stats, supported []byte
	if err := row.Scan(&sn.ID, &sn.Name, &sn.SourceLabel, &sn.CreatedAt, &stats,
		&sn.Project.Project.Name,
		&sn.Project.Project.Version,
		&sn.Project.Project.Description,
		&sn.Project.Internationalization.Primary,
		&supported,
		&sn.Project.Internationalization.Type); err != nil {
		return sn, err
	}
	if err := json.Unmarshal(stats, &sn.Stats); err != nil {
		return sn, err
	}
	if err := json.Unmarshal(supported, &sn.Project.Internationalization.Supported); err != nil {
		return sn, err
	}
	return sn, nil
}

// ListSnapshots returns every snapshot, newest first.
func (s *Store) ListSnapshots(ctx context.Context) ([]model.Snapshot, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+snapshotColumns+` FROM snapshots ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Snapshot{}
	for rows.Next() {
		sn, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

// GetSnapshot returns one snapshot by ID.
func (s *Store) GetSnapshot(ctx context.Context, id string) (*model.Snapshot, error) {
	sn, err := scanSnapshot(s.pool.QueryRow(ctx,
		`SELECT `+snapshotColumns+` FROM snapshots WHERE id = $1`, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
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
