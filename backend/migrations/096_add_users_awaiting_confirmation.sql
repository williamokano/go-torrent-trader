-- +goose Up
-- "Waiting for email confirmation" was inferred from enabled = false, which a
-- ban also sets, so confirming an email could re-enable a banned account. The
-- pending state gets its own column: set at a registration that needs
-- confirmation, cleared by confirming and by any disable.
ALTER TABLE users ADD COLUMN IF NOT EXISTS awaiting_confirmation BOOLEAN NOT NULL DEFAULT false;

-- Accounts that are pending today: never activated, disabled, and holding an
-- unclaimed confirmation. An expired token still counts — the member may ask
-- for a new one.
UPDATE users SET awaiting_confirmation = true
WHERE enabled = false
  AND activated_at IS NULL
  AND EXISTS (
    SELECT 1 FROM email_confirmations ec
    WHERE ec.user_id = users.id AND ec.confirmed_at IS NULL
  );

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS awaiting_confirmation;
