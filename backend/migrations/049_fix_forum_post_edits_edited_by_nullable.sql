-- +goose Up
-- Fix NOT NULL + ON DELETE SET NULL conflict: edited_by must allow NULL
-- so that ON DELETE SET NULL can work when the editor user is deleted.
ALTER TABLE forum_post_edits ALTER COLUMN edited_by DROP NOT NULL;

-- +goose Down
-- Restore NOT NULL (only safe if no NULL values exist)
ALTER TABLE forum_post_edits ALTER COLUMN edited_by SET NOT NULL;
