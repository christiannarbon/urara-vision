-- Older duplicates are renamed, not deleted, so the unique index can exist.
-- Unlabelled snapshots rank as 'legacy', and are renamed only after the
-- duplicates, so no row collides while the index exists.
WITH ranked AS (
    SELECT id, v, row_number() OVER (
        PARTITION BY project_id, v ORDER BY created_at DESC, id) AS rn
    FROM (SELECT id, project_id, created_at,
                 CASE WHEN project_version = '' THEN 'legacy' ELSE project_version END AS v
          FROM snapshots) s)
UPDATE snapshots s
   SET project_version = r.v || '+legacy.' || (r.rn - 1)
  FROM ranked r
 WHERE s.id = r.id AND r.rn > 1;

-- Unlabelled snapshots predate projectmeta.toml.
UPDATE snapshots SET project_version = 'legacy' WHERE project_version = '';

CREATE UNIQUE INDEX IF NOT EXISTS snapshots_project_version_key
    ON snapshots (project_id, project_version);
