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
