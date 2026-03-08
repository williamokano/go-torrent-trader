-- +goose Up
ALTER TABLE messages ADD COLUMN parent_id BIGINT REFERENCES messages(id);

-- +goose Down
ALTER TABLE messages DROP COLUMN IF EXISTS parent_id;
