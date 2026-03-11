-- +goose Up
-- Backfill topic subscriptions for existing forum participants.
-- Users who created topics or posted in them before the notification system
-- should be subscribed so they receive topic_reply notifications.
INSERT INTO topic_subscriptions (user_id, topic_id, created_at)
SELECT DISTINCT p.user_id, p.topic_id, NOW()
FROM forum_posts p
WHERE NOT EXISTS (
    SELECT 1 FROM topic_subscriptions ts
    WHERE ts.user_id = p.user_id AND ts.topic_id = p.topic_id
)
ON CONFLICT DO NOTHING;

-- +goose Down
-- No-op: we can't distinguish backfilled from organic subscriptions.
