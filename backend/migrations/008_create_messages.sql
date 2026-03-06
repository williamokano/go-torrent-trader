-- +goose Up
CREATE TABLE messages (
    id               BIGSERIAL PRIMARY KEY,
    sender_id        BIGINT NOT NULL REFERENCES users(id),
    receiver_id      BIGINT NOT NULL REFERENCES users(id),
    subject          VARCHAR(255) NOT NULL,
    body             TEXT NOT NULL,
    is_read          BOOLEAN NOT NULL DEFAULT false,
    sender_deleted   BOOLEAN NOT NULL DEFAULT false,
    receiver_deleted BOOLEAN NOT NULL DEFAULT false,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_messages_receiver ON messages (receiver_id, is_read);
CREATE INDEX idx_messages_sender ON messages (sender_id);

-- +goose Down
DROP TABLE IF EXISTS messages;
