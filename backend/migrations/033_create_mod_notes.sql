-- +goose Up
CREATE TABLE IF NOT EXISTS mod_notes (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    author_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mod_notes_user_id ON mod_notes(user_id);

-- +goose Down
DROP TABLE IF EXISTS mod_notes;
