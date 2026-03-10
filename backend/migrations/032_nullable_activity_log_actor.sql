-- +goose Up
ALTER TABLE activity_logs ALTER COLUMN actor_id DROP NOT NULL;
ALTER TABLE chat_mutes ALTER COLUMN muted_by DROP NOT NULL;

-- +goose Down
-- Cannot safely restore NOT NULL if null rows exist
