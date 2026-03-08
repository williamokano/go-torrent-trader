-- +goose Up
CREATE TABLE reseed_requests (
    id            BIGSERIAL PRIMARY KEY,
    torrent_id    BIGINT NOT NULL REFERENCES torrents(id) ON DELETE CASCADE,
    requester_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(torrent_id, requester_id)
);

CREATE INDEX idx_reseed_requests_torrent_id ON reseed_requests (torrent_id);

-- +goose Down
DROP TABLE IF EXISTS reseed_requests;
