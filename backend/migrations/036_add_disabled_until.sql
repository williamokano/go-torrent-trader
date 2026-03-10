-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS disabled_until TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS disabled_until;
