-- Overflow storage for cluster broadcasts (#295).
--
-- Replicas tell each other about node-local changes (a chat line, a closed
-- stream, an evicted setting) with NOTIFY, whose payload is capped at 8000
-- bytes. The rare message that does not fit is written here and the
-- notification carries only its id. Rows are needed for the few milliseconds
-- it takes every listener to read them, and the publisher deletes anything older
-- than a few minutes whenever it writes a new one.

-- +goose Up
CREATE TABLE IF NOT EXISTS cluster_messages (
    id         BIGSERIAL PRIMARY KEY,
    body       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cluster_messages_created_at ON cluster_messages (created_at);

-- +goose Down
DROP TABLE IF EXISTS cluster_messages;
