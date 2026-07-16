-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS can_invite BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS can_invite;
