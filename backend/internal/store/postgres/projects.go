// Projects: the groups snapshots are saved under.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/projectmeta"
)

// projectFor is the slug and name a snapshot is saved under. A name with no
// usable slug gets a project of its own.
func projectFor(sn model.Snapshot) (slug, name string) {
	if slug = projectmeta.Slug(sn.Project.Project.Name); slug != "" {
		return slug, sn.Project.Project.Name
	}
	return projectmeta.LegacySlug(sn.ID), sn.Name
}

// ensureProject returns the ID of the project with slug, creating it if needed.
// DO NOTHING rather than DO UPDATE: an update would lock the row for the whole
// ingest and queue every other save to the same project behind it.
func ensureProject(ctx context.Context, tx pgx.Tx, slug, name, description string) (string, error) {
	if _, err := tx.Exec(ctx,
		`INSERT INTO projects (id, slug, name, description) VALUES (gen_random_uuid()::text, $1, $2, $3)
		 ON CONFLICT (slug) DO NOTHING`, slug, name, description); err != nil {
		return "", fmt.Errorf("create project %q: %w", slug, err)
	}
	var id string
	if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE slug = $1`, slug).Scan(&id); err != nil {
		return "", fmt.Errorf("find project %q: %w", slug, err)
	}
	return id, nil
}

// touchProject takes the newest save's name and description. Run last, so the
// row lock it takes is held only until commit.
func touchProject(ctx context.Context, tx pgx.Tx, id, name, description string) error {
	if _, err := tx.Exec(ctx,
		`UPDATE projects SET name = $2, description = $3, updated_at = now() WHERE id = $1`,
		id, name, description); err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	return nil
}

// projectSummaryQuery reads projects with their version count and newest
// snapshot in one pass. Callers append a WHERE and ORDER BY.
const projectSummaryQuery = `
	SELECT p.id, p.slug, p.name, p.description, p.created_at, p.updated_at,
	       COALESCE(c.n, 0), l.id, l.project_version, l.created_at
	FROM projects p
	LEFT JOIN (SELECT project_id, count(*) AS n FROM snapshots GROUP BY project_id) c
	       ON c.project_id = p.id
	LEFT JOIN (SELECT DISTINCT ON (project_id) project_id, id, project_version, created_at
	           FROM snapshots ORDER BY project_id, created_at DESC) l
	       ON l.project_id = p.id`

func scanProjectSummary(row pgx.Row) (model.ProjectSummary, error) {
	var p model.ProjectSummary
	var latestID, latestVersion *string
	var latestAt *time.Time
	if err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt,
		&p.VersionCount, &latestID, &latestVersion, &latestAt); err != nil {
		return p, err
	}
	if latestID != nil {
		p.Latest = &model.ProjectVersionRef{SnapshotID: *latestID, Version: *latestVersion, CreatedAt: *latestAt}
	}
	return p, nil
}

// ListProjects returns every project, most recently updated first.
func (s *Store) ListProjects(ctx context.Context) ([]model.ProjectSummary, error) {
	rows, err := s.pool.Query(ctx, projectSummaryQuery+` ORDER BY p.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.ProjectSummary{}
	for rows.Next() {
		p, err := scanProjectSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProject returns one project by slug, or ErrNotFound.
func (s *Store) GetProject(ctx context.Context, slug string) (*model.ProjectSummary, error) {
	p, err := scanProjectSummary(s.pool.QueryRow(ctx, projectSummaryQuery+` WHERE p.slug = $1`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DeleteProject deletes the project and returns the snapshot IDs it held, so
// the caller can clear Neo4j.
func (s *Store) DeleteProject(ctx context.Context, slug string) ([]string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Locking the project row makes a concurrent save's upsert wait, so no
	// snapshot can join after the IDs are read and be deleted unreported.
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM projects WHERE slug = $1 FOR UPDATE`, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `SELECT id FROM snapshots WHERE project_id = $1`, id)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return ids, nil
}
