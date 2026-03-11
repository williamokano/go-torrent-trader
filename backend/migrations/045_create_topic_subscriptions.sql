-- +goose Up
CREATE TABLE IF NOT EXISTS topic_subscriptions (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    topic_id    BIGINT NOT NULL REFERENCES forum_topics(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, topic_id)
);

CREATE INDEX IF NOT EXISTS idx_topic_subscriptions_topic ON topic_subscriptions(topic_id);

-- +goose Down
DROP TABLE IF EXISTS topic_subscriptions;
