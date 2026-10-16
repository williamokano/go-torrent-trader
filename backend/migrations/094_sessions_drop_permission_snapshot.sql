-- Sessions stop carrying a copy of the member's authority (#291).
--
-- A session row used to hold the member's group and a permission snapshot taken
-- at login. Refresh copied them forward, so a banned or demoted member kept the
-- old access for as long as their client kept refreshing — through the admin
-- edit, the promotion engine and the warning ladder alike. Lookups now join the
-- member's account and current group instead (repository/postgres.SessionStore),
-- so neither column is read any more.

-- +goose Up
ALTER TABLE sessions DROP COLUMN IF EXISTS permissions;
ALTER TABLE sessions DROP COLUMN IF EXISTS group_id;

-- +goose Down
-- The snapshot is not recoverable, and the previous release would read a
-- zero-permission placeholder for every existing row — admins silently without
-- their rights, and no 401 to tell anyone to sign in again. So end every
-- session: rolling back costs one re-login, honestly.
DELETE FROM sessions;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS group_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS permissions JSONB NOT NULL DEFAULT '{}';
