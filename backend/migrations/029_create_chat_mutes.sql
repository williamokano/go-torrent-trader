-- +goose Up
CREATE TABLE chat_mutes (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id),
    muted_by   BIGINT NOT NULL REFERENCES users(id),
    reason     TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_mutes_user_expires ON chat_mutes (user_id, expires_at);

-- +goose Down
DROP TABLE IF EXISTS chat_mutes;
