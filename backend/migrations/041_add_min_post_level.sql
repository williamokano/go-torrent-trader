-- +goose Up
ALTER TABLE forums ADD COLUMN IF NOT EXISTS min_post_level INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE forums DROP COLUMN IF EXISTS min_post_level;
