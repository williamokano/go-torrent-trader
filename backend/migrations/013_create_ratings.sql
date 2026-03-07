-- +goose Up
CREATE TABLE torrent_ratings (
    id         BIGSERIAL PRIMARY KEY,
    torrent_id BIGINT NOT NULL REFERENCES torrents(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating     SMALLINT NOT NULL CHECK (rating >= 1 AND rating <= 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (torrent_id, user_id)
);

CREATE INDEX idx_torrent_ratings_torrent_id ON torrent_ratings(torrent_id);

-- +goose Down
DROP TABLE IF EXISTS torrent_ratings;
