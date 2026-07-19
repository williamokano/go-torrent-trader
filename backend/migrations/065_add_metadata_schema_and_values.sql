-- +goose Up
-- Category-driven torrent metadata (BE-3.13).
-- categories.metadata_schema holds the category's own field definitions (a JSON array);
-- torrents.metadata holds the values submitted for those fields (a JSON object).
-- Non-null defaults keep scans simple (no NULL handling) and existing rows valid.
ALTER TABLE categories ADD COLUMN IF NOT EXISTS metadata_schema JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE torrents ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE torrents DROP COLUMN IF EXISTS metadata;
ALTER TABLE categories DROP COLUMN IF EXISTS metadata_schema;
