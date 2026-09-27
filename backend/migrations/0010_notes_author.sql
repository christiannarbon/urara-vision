-- The user-delete SET NULL looks notes up by author.
CREATE INDEX IF NOT EXISTS notes_author_idx ON notes (author_id);
