-- Discussion pinned to one part of one snapshot. Replies are one level deep.
CREATE TABLE IF NOT EXISTS notes (
    id               TEXT PRIMARY KEY,
    snapshot_id      TEXT        NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    parent_id        TEXT        REFERENCES notes(id) ON DELETE CASCADE,
    anchor_kind      TEXT        NOT NULL,
    anchor_id        TEXT        NOT NULL,
    body             TEXT        NOT NULL,
    author_id        TEXT        REFERENCES users(id) ON DELETE SET NULL,
    author_name      TEXT        NOT NULL,
    resolved_at      TIMESTAMPTZ,
    resolved_by_name TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS notes_anchor_idx ON notes (snapshot_id, anchor_kind, anchor_id, created_at);
CREATE INDEX IF NOT EXISTS notes_parent_idx ON notes (parent_id);
