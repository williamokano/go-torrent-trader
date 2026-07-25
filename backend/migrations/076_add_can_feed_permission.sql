-- +goose Up
-- Live feeds were open to every member. `can_feed` makes that a decision: an
-- operator grants it by class, and a single user can have it taken away without
-- touching their class — the same shape as can_chat and can_invite.
--
-- Both default to true, so an upgrade changes nothing until someone decides it
-- should. Defaulting to false would silently break the live page for every
-- member on deploy, which is not a decision a migration gets to make.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS can_feed BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS can_feed BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS can_feed;
ALTER TABLE groups DROP COLUMN IF EXISTS can_feed;
