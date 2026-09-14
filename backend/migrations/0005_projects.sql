-- A project groups the snapshots of one documentation set.
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    slug        TEXT        NOT NULL UNIQUE,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS project_id TEXT
    REFERENCES projects(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS snapshots_project_id_idx
    ON snapshots (project_id, created_at DESC);

-- Backfill snapshots saved without a project. Each step touches only rows with
-- project_id IS NULL, so it is a no-op once nothing is left.
-- Must match projectmeta.Slug; projects_backfill_test.go checks it.
-- slug(x) = btrim(left(btrim(regexp_replace(lower(x), '[^a-z0-9]+', '-', 'g'), '-'), 64), '-')
INSERT INTO projects (id, slug, name, description, created_at, updated_at)
SELECT gen_random_uuid()::text, slug, project_name, project_description, first_at, last_at
FROM (
    SELECT DISTINCT ON (slug)
        slug, project_name, project_description,
        min(created_at) OVER (PARTITION BY slug) AS first_at,
        max(created_at) OVER (PARTITION BY slug) AS last_at
    FROM (
        SELECT btrim(left(btrim(regexp_replace(lower(project_name), '[^a-z0-9]+', '-', 'g'), '-'), 64), '-') AS slug,
               project_name, project_description, created_at
        FROM snapshots
        WHERE project_id IS NULL
    ) named
    WHERE slug <> ''
    ORDER BY slug, created_at DESC
) newest
ON CONFLICT (slug) DO NOTHING;

UPDATE snapshots SET project_id = p.id
FROM projects p
WHERE snapshots.project_id IS NULL
  AND p.slug = btrim(left(btrim(regexp_replace(lower(snapshots.project_name), '[^a-z0-9]+', '-', 'g'), '-'), 64), '-');

-- Whatever is left has no usable name: one project each, as projectmeta.LegacySlug.
INSERT INTO projects (id, slug, name, created_at, updated_at)
SELECT gen_random_uuid()::text, 'legacy-' || left(md5(id), 12), name, created_at, created_at
FROM snapshots
WHERE project_id IS NULL
ON CONFLICT (slug) DO NOTHING;

UPDATE snapshots SET project_id = p.id
FROM projects p
WHERE snapshots.project_id IS NULL
  AND p.slug = 'legacy-' || left(md5(snapshots.id), 12);
