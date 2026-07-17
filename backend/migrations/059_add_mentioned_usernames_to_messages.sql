-- +goose Up
-- Extends resolved @mention linking (see 058_add_mentioned_usernames.sql) to
-- private message bodies. Links only — messages intentionally do not publish
-- a mention notification, since the recipient already gets a "new message"
-- notification for the PM itself.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS mentioned_usernames JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE messages DROP COLUMN IF EXISTS mentioned_usernames;
