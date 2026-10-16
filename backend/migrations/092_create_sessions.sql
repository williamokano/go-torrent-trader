-- Sessions move from Redis to PostgreSQL (#288).
--
-- Tokens are stored only as SHA-256 hashes, as password resets and email
-- confirmations already are: a row read from here — or a backup restored
-- somewhere else — cannot be replayed as a login. Nothing in the application
-- reads a token back out of a stored session; every lookup starts from the
-- token the caller presented.
--
-- One row per session, indexed by both halves. That is what keeps a session
-- reachable for as long as either token can be used: the access token lives an
-- hour and the refresh token a month, and revoking by user must reach rows whose
-- access token expired weeks ago (#231).
--
-- Existing Redis sessions are not carried over, so every member signs in once
-- after the upgrade.

-- +goose Up
CREATE TABLE IF NOT EXISTS sessions (
    id                 BIGSERIAL PRIMARY KEY,
    user_id            BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    access_token_hash  BYTEA       NOT NULL UNIQUE,
    refresh_token_hash BYTEA       NOT NULL UNIQUE,
    group_id           BIGINT      NOT NULL,
    -- The permission snapshot taken at login, as the Redis session held it.
    permissions        JSONB       NOT NULL,
    device_name        TEXT        NOT NULL DEFAULT '',
    ip                 TEXT        NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL,
    last_active_at     TIMESTAMPTZ NOT NULL,
    access_expires_at  TIMESTAMPTZ NOT NULL,
    refresh_expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions (user_id);
-- For the maintenance sweep, which deletes rows whose refresh token has expired.
CREATE INDEX IF NOT EXISTS idx_sessions_refresh_expires_at ON sessions (refresh_expires_at);

-- +goose Down
DROP TABLE IF EXISTS sessions;
