// Versions: the snapshots of one project, addressed by project_version.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
)

// latestVersion is the alias GetVersion resolves to the newest import.
const latestVersion = "latest"

// ListVersions returns a project's snapshots, newest first.
func (s *Store) ListVersions(ctx context.Context, slug string) ([]model.Snapshot, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+snapshotColumns+` `+snapshotsFrom+` WHERE p.slug = $1 ORDER BY s.created_at DESC, s.id`, slug)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Snapshot, error) {
		return scanSnapshot(row)
	})
	if err != nil {
		return nil, err
	}
	// Every project has a snapshot, so no rows means no project.
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out, nil
}

// GetVersion returns one version; "latest" is the most recently imported.
func (s *Store) GetVersion(ctx context.Context, slug, version string) (*model.Snapshot, error) {
	q := `SELECT ` + snapshotColumns + ` ` + snapshotsFrom + ` WHERE p.slug = $1`
	args := []any{slug}
	if version == latestVersion {
		q += ` ORDER BY s.created_at DESC, s.id LIMIT 1`
	} else {
		q += ` AND s.project_version = $2`
		args = append(args, version)
	}
	sn, err := scanSnapshot(s.pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sn, nil
}

// DeleteVersion removes one version, and the project too when it was the last.
// It returns the deleted snapshot ID and whether the project went with it.
func (s *Store) DeleteVersion(ctx context.Context, slug, version string) (sid string, projectDeleted bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var projectID string
	err = tx.QueryRow(ctx,
		`DELETE FROM snapshots s USING projects p
		 WHERE p.id = s.project_id AND p.slug = $1 AND s.project_version = $2
		 RETURNING s.id, s.project_id`, slug, version).Scan(&sid, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, ErrNotFound
	}
	if err != nil {
		return "", false, fmt.Errorf("delete snapshot: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`DELETE FROM projects WHERE id = $1 AND NOT EXISTS (SELECT 1 FROM snapshots WHERE project_id = $1)`,
		projectID)
	if err != nil {
		return "", false, fmt.Errorf("delete empty project: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return sid, tag.RowsAffected() > 0, nil
}
