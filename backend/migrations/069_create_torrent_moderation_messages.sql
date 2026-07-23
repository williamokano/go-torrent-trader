-- +goose Up
-- Per-torrent moderation discussion thread (BE-8.22b): staff and the uploader
-- exchange short messages about what needs fixing during review.
CREATE TABLE IF NOT EXISTS torrent_moderation_messages (
    id         BIGSERIAL PRIMARY KEY,
    torrent_id BIGINT NOT NULL REFERENCES torrents(id) ON DELETE CASCADE,
    author_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_torrent_moderation_messages_torrent
    ON torrent_moderation_messages (torrent_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS torrent_moderation_messages;
