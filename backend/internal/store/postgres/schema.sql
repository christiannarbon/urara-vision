-- Schema for the data model documentation index.
--
-- Postgres is the system of record: it holds every parsed document verbatim
-- enough to rebuild the UI's detail panes, plus a full-text index. Neo4j holds
-- the projected graph and answers traversal queries.

CREATE TABLE IF NOT EXISTS snapshots (
    id           TEXT PRIMARY KEY,
    name         TEXT        NOT NULL,
    source_label TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    stats        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    -- What the directory declared about itself in projectmeta.toml. Columns
    -- rather than one document, because these are the fields worth asking a
    -- question of: which projects are on which version, and which of them are
    -- documented in a given language.
    project_name        TEXT  NOT NULL DEFAULT '',
    project_version     TEXT  NOT NULL DEFAULT '',
    project_description TEXT  NOT NULL DEFAULT '',
    i18n_primary        TEXT  NOT NULL DEFAULT '',
    i18n_supported      JSONB NOT NULL DEFAULT '[]'::jsonb,
    i18n_type           TEXT  NOT NULL DEFAULT ''
);

-- The manifest arrived after the first release, so a database created before it
-- has the table above without these columns. Adding them here as well keeps
-- Migrate a single idempotent script: a fresh database gets them from the
-- CREATE, an existing one from the ALTER, and neither needs to know which it is.
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS project_name        TEXT  NOT NULL DEFAULT '';
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS project_version     TEXT  NOT NULL DEFAULT '';
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS project_description TEXT  NOT NULL DEFAULT '';
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS i18n_primary        TEXT  NOT NULL DEFAULT '';
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS i18n_supported      JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS i18n_type           TEXT  NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS snapshots_project_idx ON snapshots (project_name);

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

-- Every save writes project_id, and the backfill above leaves none empty.
ALTER TABLE snapshots ALTER COLUMN project_id SET NOT NULL;

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

CREATE TABLE IF NOT EXISTS domains (
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    id          TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    title       TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    mermaid     TEXT NOT NULL DEFAULT '',
    lineage     JSONB NOT NULL DEFAULT '[]'::jsonb,
    doc_path    TEXT NOT NULL DEFAULT '',
    table_count INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (snapshot_id, id)
);

CREATE TABLE IF NOT EXISTS tables (
    snapshot_id       TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    id                TEXT NOT NULL,
    name              TEXT NOT NULL,
    domain_id         TEXT NOT NULL,
    kind              TEXT NOT NULL DEFAULT 'unknown',
    kind_raw          TEXT NOT NULL DEFAULT '',
    grain             TEXT NOT NULL DEFAULT '',
    update_frequency  TEXT NOT NULL DEFAULT '',
    layer             TEXT NOT NULL DEFAULT '',
    domain_label      TEXT NOT NULL DEFAULT '',
    description       TEXT NOT NULL DEFAULT '',
    notes             JSONB NOT NULL DEFAULT '[]'::jsonb,
    relationship_note TEXT NOT NULL DEFAULT '',
    doc_path          TEXT NOT NULL DEFAULT '',
    conformed         BOOLEAN NOT NULL DEFAULT FALSE,
    conformed_in      JSONB NOT NULL DEFAULT '[]'::jsonb,
    search            tsvector,
    PRIMARY KEY (snapshot_id, id)
);

CREATE INDEX IF NOT EXISTS tables_domain_idx ON tables (snapshot_id, domain_id);
CREATE INDEX IF NOT EXISTS tables_name_idx   ON tables (snapshot_id, name);
CREATE INDEX IF NOT EXISTS tables_search_idx ON tables USING GIN (search);

CREATE TABLE IF NOT EXISTS columns (
    snapshot_id TEXT NOT NULL,
    table_id    TEXT NOT NULL,
    ordinal     INT  NOT NULL,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    is_pk       BOOLEAN NOT NULL DEFAULT FALSE,
    is_fk       BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (snapshot_id, table_id, ordinal),
    FOREIGN KEY (snapshot_id, table_id) REFERENCES tables(snapshot_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS columns_name_idx ON columns (snapshot_id, name);

CREATE TABLE IF NOT EXISTS relationships (
    snapshot_id   TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    id            TEXT NOT NULL,
    from_table_id TEXT NOT NULL,
    to_table_id   TEXT NOT NULL DEFAULT '',
    target_ref    TEXT NOT NULL DEFAULT '',
    from_column   TEXT NOT NULL DEFAULT '',
    to_column     TEXT NOT NULL DEFAULT '',
    join_key_raw  TEXT NOT NULL DEFAULT '',
    cardinality   TEXT NOT NULL DEFAULT '',
    resolution    TEXT NOT NULL DEFAULT '',
    candidates    JSONB NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (snapshot_id, id)
);

CREATE INDEX IF NOT EXISTS relationships_from_idx ON relationships (snapshot_id, from_table_id);
CREATE INDEX IF NOT EXISTS relationships_to_idx   ON relationships (snapshot_id, to_table_id);

CREATE TABLE IF NOT EXISTS column_lineage (
    snapshot_id   TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    table_id      TEXT NOT NULL,
    ordinal       INT  NOT NULL,
    column_name   TEXT NOT NULL,
    source_table  TEXT NOT NULL DEFAULT '',
    source_column TEXT NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT '',
    derived       BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (snapshot_id, table_id, ordinal)
);

CREATE INDEX IF NOT EXISTS column_lineage_source_idx ON column_lineage (snapshot_id, source_table);

CREATE TABLE IF NOT EXISTS source_tables (
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    id          TEXT NOT NULL,
    dataset     TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL DEFAULT '',
    refs        INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (snapshot_id, id)
);

CREATE TABLE IF NOT EXISTS diagnostics (
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    ordinal     INT  NOT NULL,
    severity    TEXT NOT NULL,
    code        TEXT NOT NULL,
    message     TEXT NOT NULL,
    domain_id   TEXT NOT NULL DEFAULT '',
    table_id    TEXT NOT NULL DEFAULT '',
    doc_path    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (snapshot_id, ordinal)
);

CREATE INDEX IF NOT EXISTS diagnostics_severity_idx ON diagnostics (snapshot_id, severity);

-- Chat conversations about a snapshot.
--
-- They cascade from the snapshot rather than outliving it: every citation a
-- conversation carries is a table ID, and once the snapshot is gone those point
-- at nothing. A transcript full of dead links is worse than no transcript.
CREATE TABLE IF NOT EXISTS conversations (
    id          TEXT PRIMARY KEY,
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    title       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS conversations_snapshot_idx
    ON conversations (snapshot_id, created_at DESC);

-- One turn. `citations` is the table IDs the answer drew on; `meta` is what the
-- turn cost -- model, tokens, tool calls, latency -- kept for evaluation rather
-- than for display.
--
-- `role` is checked in Go rather than by a constraint here, so an invalid role
-- is a 400 with a message rather than a driver error the handler has to
-- interpret.
CREATE TABLE IF NOT EXISTS conversation_messages (
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    ordinal         INT  NOT NULL,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL DEFAULT '',
    citations       JSONB NOT NULL DEFAULT '[]'::jsonb,
    meta            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, ordinal)
);

-- Runtime switches an admin can change without a redeploy.
CREATE TABLE IF NOT EXISTS app_settings (
    key        TEXT PRIMARY KEY,
    value      JSONB       NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A person. Role is checked in Go (internal/auth).
CREATE TABLE IF NOT EXISTS users (
    id           TEXT PRIMARY KEY,
    username     TEXT        NOT NULL UNIQUE,  -- stored lower-case
    display_name TEXT        NOT NULL DEFAULT '',
    role         TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- How a user logs in. provider 'password' uses subject = username; 'google' comes later.
CREATE TABLE IF NOT EXISTS user_identities (
    id            TEXT PRIMARY KEY,
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider      TEXT        NOT NULL,
    subject       TEXT        NOT NULL,
    password_hash TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, subject)
);
CREATE INDEX IF NOT EXISTS user_identities_user_idx ON user_identities (user_id);

-- Only the SHA-256 of the session token is stored.
CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user_idx    ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions (expires_at);
