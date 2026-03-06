-- +goose Up
CREATE TABLE chat_messages (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id),
    message    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_created ON chat_messages (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS chat_messages;
