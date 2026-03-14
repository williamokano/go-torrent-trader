-- +goose Up
-- Composite index for post_number calculation in search (correlated subquery)
-- and GetFirstPostIDByTopic (ORDER BY id ASC LIMIT 1).
CREATE INDEX IF NOT EXISTS idx_forum_posts_topic_id_id ON forum_posts(topic_id, id);

-- Partial index for user_post_count subquery that filters deleted_at IS NULL.
CREATE INDEX IF NOT EXISTS idx_forum_posts_user_id_active ON forum_posts(user_id) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_forum_posts_user_id_active;
DROP INDEX IF EXISTS idx_forum_posts_topic_id_id;
