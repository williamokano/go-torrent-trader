-- +goose Up
ALTER TABLE torrents ADD COLUMN files JSONB;

-- +goose Down
ALTER TABLE torrents DROP COLUMN IF EXISTS files;
