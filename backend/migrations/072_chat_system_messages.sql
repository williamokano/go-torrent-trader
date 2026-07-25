-- +goose Up
-- System chat messages (BE-10.1). The chat connector posts announcements to the
-- shoutbox with no author. A dedicated "system user" row was rejected because it
-- would pollute user lists, member counts and admin search; instead user_id goes
-- NULL and `system` marks the row.
ALTER TABLE chat_messages ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS system BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
-- System rows have a NULL user_id, so they must go before NOT NULL can be
-- restored — otherwise the ALTER fails and `goose down` wedges.
DELETE FROM chat_messages WHERE system;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS system;
ALTER TABLE chat_messages ALTER COLUMN user_id SET NOT NULL;
