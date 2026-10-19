-- +goose Up
-- "Waiting for email confirmation" was inferred from enabled = false, which a
-- ban also sets, so confirming an email could re-enable a banned account. The
-- pending state gets its own column: set at a registration that needs
-- confirmation, cleared by confirming and by any disable.
ALTER TABLE users ADD COLUMN IF NOT EXISTS awaiting_confirmation BOOLEAN NOT NULL DEFAULT false;

-- Backfill accounts that are pending today: off and never activated. The old
-- schema cannot tell those apart from an account banned before it confirmed,
-- so any account carrying a sign of staff action — a timed ban, a warning, or
-- an admin edit of enabled — is left unconfirmable; staff can enable it by
-- hand. No token is required, so a registration whose confirmation email
-- never went out can still ask for a new one.
UPDATE users SET awaiting_confirmation = true
WHERE enabled = false
  AND activated_at IS NULL
  AND disabled_until IS NULL
  AND NOT EXISTS (SELECT 1 FROM warnings w WHERE w.user_id = users.id)
  AND NOT EXISTS (
    SELECT 1 FROM user_edit_history h
    WHERE h.user_id = users.id AND h.field = 'enabled'
  );

-- An account cannot be both on and still waiting to confirm. Guarded so a
-- partial run can be repeated.
-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'users_awaiting_confirmation_disabled'
  ) THEN
    ALTER TABLE users ADD CONSTRAINT users_awaiting_confirmation_disabled
      CHECK (NOT (enabled AND awaiting_confirmation));
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_awaiting_confirmation_disabled;
ALTER TABLE users DROP COLUMN IF EXISTS awaiting_confirmation;
