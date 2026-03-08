-- +goose Up
CREATE TABLE activity_logs (
    id         BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(64) NOT NULL,
    actor_id   BIGINT      NOT NULL REFERENCES users(id),
    message    TEXT        NOT NULL,
    metadata   JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_activity_logs_event_type ON activity_logs (event_type);
CREATE INDEX idx_activity_logs_actor_id ON activity_logs (actor_id);
CREATE INDEX idx_activity_logs_created_at ON activity_logs (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS activity_logs;
