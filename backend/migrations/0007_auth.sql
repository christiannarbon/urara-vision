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
