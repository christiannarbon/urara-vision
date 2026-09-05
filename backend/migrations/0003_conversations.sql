-- Chat conversations about a snapshot, and the turns they are made of.
--
-- The agent answers questions against one ingest, so a transcript belongs to
-- that snapshot and goes when it does.

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
