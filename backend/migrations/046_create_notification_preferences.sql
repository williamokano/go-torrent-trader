-- +goose Up
CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id            BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type  TEXT NOT NULL,
    enabled            BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (user_id, notification_type)
);

-- +goose Down
DROP TABLE IF EXISTS notification_preferences;
