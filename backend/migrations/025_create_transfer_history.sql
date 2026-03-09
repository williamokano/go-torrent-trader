-- +goose Up
CREATE TABLE transfer_history (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id),
    torrent_id  BIGINT NOT NULL REFERENCES torrents(id) ON DELETE CASCADE,
    uploaded    BIGINT NOT NULL DEFAULT 0,
    downloaded  BIGINT NOT NULL DEFAULT 0,
    seeder      BOOLEAN NOT NULL DEFAULT false,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_announce TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, torrent_id)
);

CREATE INDEX idx_transfer_history_user ON transfer_history (user_id);
CREATE INDEX idx_transfer_history_torrent ON transfer_history (torrent_id);

-- +goose Down
DROP TABLE IF EXISTS transfer_history;
