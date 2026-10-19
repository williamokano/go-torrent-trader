-- +goose Up
-- Spending an invite checked the balance and decremented separately, so two
-- concurrent invites could both spend the last one and leave -1. The
-- decrement is now conditional; the schema backs it.
UPDATE users SET invites = 0 WHERE invites < 0;

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_invites_nonnegative') THEN
    ALTER TABLE users ADD CONSTRAINT users_invites_nonnegative CHECK (invites >= 0);
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_invites_nonnegative;
