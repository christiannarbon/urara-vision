-- The projectmeta.toml a documentation directory must now carry.
--
-- Snapshots ingested before it exists keep their rows and simply declare no
-- project; every ingest from here on writes one, because an upload without a
-- manifest is refused.

ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS project_name        TEXT  NOT NULL DEFAULT '';
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS project_version     TEXT  NOT NULL DEFAULT '';
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS project_description TEXT  NOT NULL DEFAULT '';
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS i18n_primary        TEXT  NOT NULL DEFAULT '';
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS i18n_supported      JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS i18n_type           TEXT  NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS snapshots_project_idx ON snapshots (project_name);
