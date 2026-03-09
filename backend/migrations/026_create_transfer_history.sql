-- +goose Up
CREATE TABLE transfer_history (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    torrent_id  BIGINT REFERENCES torrents(id) ON DELETE SET NULL,
    uploaded    BIGINT NOT NULL DEFAULT 0,
    downloaded  BIGINT NOT NULL DEFAULT 0,
    seeder      BOOLEAN NOT NULL DEFAULT false,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_announce TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, torrent_id)
);

-- idx_transfer_history_user is intentionally omitted — the UNIQUE(user_id, torrent_id)
-- constraint already creates an index with user_id as the leading column.
CREATE INDEX idx_transfer_history_torrent ON transfer_history (torrent_id);

-- Composite index for user activity queries (seeding/leeching tabs)
CREATE INDEX IF NOT EXISTS idx_peers_user_seeder ON peers (user_id, seeder);

-- +goose Down
DROP INDEX IF EXISTS idx_peers_user_seeder;
DROP TABLE IF EXISTS transfer_history;
