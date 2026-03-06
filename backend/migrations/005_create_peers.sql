-- +goose Up
CREATE TABLE peers (
    id             BIGSERIAL PRIMARY KEY,
    torrent_id     BIGINT NOT NULL REFERENCES torrents(id),
    user_id        BIGINT NOT NULL REFERENCES users(id),
    peer_id        BYTEA NOT NULL,
    ip             INET NOT NULL,
    port           INT NOT NULL,
    uploaded       BIGINT NOT NULL DEFAULT 0,
    downloaded     BIGINT NOT NULL DEFAULT 0,
    left_bytes     BIGINT NOT NULL DEFAULT 0,
    seeder         BOOLEAN NOT NULL DEFAULT false,
    agent          VARCHAR(255),
    started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_announce  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (torrent_id, user_id, peer_id)
);

CREATE INDEX idx_peers_torrent ON peers (torrent_id);
CREATE INDEX idx_peers_last_announce ON peers (last_announce);

-- +goose Down
DROP TABLE IF EXISTS peers;
