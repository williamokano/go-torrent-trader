-- +goose Up
ALTER TABLE categories ADD COLUMN image_url TEXT;

-- +goose Down
ALTER TABLE categories DROP COLUMN IF EXISTS image_url;
