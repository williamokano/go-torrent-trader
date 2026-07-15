-- +goose Up
-- BE-5.10 renamed the "forum_mention" notification type to the unified "mention".
-- Carry over each user's saved preference so an existing opt-out is preserved
-- (IsEnabled defaults to enabled when no row exists, so without this a user who
-- disabled forum mentions would silently start receiving mention notifications).
-- Skip rows where a "mention" preference already exists to avoid a PK collision,
-- then drop any leftover "forum_mention" rows.
UPDATE notification_preferences
SET notification_type = 'mention'
WHERE notification_type = 'forum_mention'
  AND user_id NOT IN (
      SELECT user_id FROM notification_preferences WHERE notification_type = 'mention'
  );

DELETE FROM notification_preferences WHERE notification_type = 'forum_mention';

-- +goose Down
UPDATE notification_preferences
SET notification_type = 'forum_mention'
WHERE notification_type = 'mention'
  AND user_id NOT IN (
      SELECT user_id FROM notification_preferences WHERE notification_type = 'forum_mention'
  );
