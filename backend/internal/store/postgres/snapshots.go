// Snapshot-level reads: listing ingests, and finding the newest one.
package postgres

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
)

// snapshotColumns is every field a snapshot is rebuilt from, shared by the two
// reads below so a new one cannot be added to only half of them.
const snapshotColumns = `s.id, s.name, s.source_label, s.created_at, s.stats,
	s.project_name, s.project_version, s.project_description,
	s.i18n_primary, s.i18n_supported, s.i18n_type,
	COALESCE(p.id, ''), COALESCE(p.slug, '')`

// snapshotsFrom is the FROM clause snapshotColumns is read from.
const snapshotsFrom = `FROM snapshots s LEFT JOIN projects p ON p.id = s.project_id`

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
		&sn.Project.Internationalization.Type,
		&sn.ProjectID, &sn.ProjectSlug); err != nil {
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
		`SELECT `+snapshotColumns+` `+snapshotsFrom+` ORDER BY s.created_at DESC`)
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
		`SELECT `+snapshotColumns+` `+snapshotsFrom+` WHERE s.id = $1`, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sn, nil
}

// DeleteSnapshot removes a snapshot and everything hanging off it, and its
// project too when it was the last one.
func (s *Store) DeleteSnapshot(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var projectID string
	err = tx.QueryRow(ctx, `SELECT project_id FROM snapshots WHERE id = $1`, id).Scan(&projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	// Locked first, as DeleteProject does, so a concurrent save into this
	// project is not cascaded away with it.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM projects WHERE id = $1 FOR UPDATE`, projectID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM snapshots WHERE id = $1`, id); err != nil {
		return err
	}
	if _, err := deleteEmptyProject(ctx, tx, projectID); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
