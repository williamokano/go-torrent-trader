-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS activated_at TIMESTAMPTZ;

-- Backfill: a user who ever logged in or was ever seen active clearly was
-- activated at some point, even though the application only starts stamping
-- activated_at going forward (registration without email confirmation,
-- email confirmation, or first login). Without this, every pre-existing
-- disabled account — including already-banned-but-previously-active ones —
-- would read as "never activated" until they happened to log in again, which
-- a banned account never will. This is exactly the BE-8.19 bug the new
-- column exists to close, so it must not reopen it for existing rows.
UPDATE users
SET activated_at = COALESCE(last_login, last_access)
WHERE activated_at IS NULL
  AND (last_login IS NOT NULL OR last_access IS NOT NULL);

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS activated_at;
