-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS can_download BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS can_upload BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS can_chat BOOLEAN NOT NULL DEFAULT true;

CREATE TABLE IF NOT EXISTS user_restrictions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    restriction_type TEXT NOT NULL,
    reason TEXT NOT NULL,
    issued_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ,
    lifted_at TIMESTAMPTZ,
    lifted_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_restrictions_active
    ON user_restrictions (user_id, restriction_type, lifted_at);

-- +goose Down
DROP TABLE IF EXISTS user_restrictions;
ALTER TABLE users DROP COLUMN IF EXISTS can_download;
ALTER TABLE users DROP COLUMN IF EXISTS can_upload;
ALTER TABLE users DROP COLUMN IF EXISTS can_chat;
