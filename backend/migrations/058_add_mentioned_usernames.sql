-- +goose Up
-- Resolved @mentions, persisted once at write time so rendering never has to
-- re-look-up whether a token is a real user — it's a "is this in the known
-- set" check against the array already sitting on the row. Only tokens that
-- resolved to a real, existing user are stored; typos and non-mention `@`
-- uses (e.g. an email address) never appear here.
ALTER TABLE torrent_comments ADD COLUMN IF NOT EXISTS mentioned_usernames JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE forum_posts ADD COLUMN IF NOT EXISTS mentioned_usernames JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE torrent_comments DROP COLUMN IF EXISTS mentioned_usernames;
ALTER TABLE forum_posts DROP COLUMN IF EXISTS mentioned_usernames;
