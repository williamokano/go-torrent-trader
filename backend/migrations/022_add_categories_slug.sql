-- +goose Up
ALTER TABLE categories ADD COLUMN IF NOT EXISTS slug VARCHAR(255);

-- Populate slug from name (lowercase, replace spaces with dashes).
UPDATE categories SET slug = LOWER(REPLACE(name, ' ', '-'));

ALTER TABLE categories ALTER COLUMN slug SET NOT NULL;

-- +goose Down
ALTER TABLE categories DROP COLUMN slug;
