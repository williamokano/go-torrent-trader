-- +goose Up
ALTER TABLE users ADD COLUMN invited_by BIGINT REFERENCES users(id);

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS invited_by;
