-- A conversation's owner. NULL for threads from before owners; users never see those.
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS user_id TEXT
    REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS conversations_user_snapshot_idx
    ON conversations (user_id, snapshot_id, created_at DESC);
