-- +goose Up
CREATE TABLE password_resets (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    token_hash VARCHAR(64) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_password_resets_token ON password_resets (token_hash);
CREATE INDEX idx_password_resets_user ON password_resets (user_id);

-- +goose Down
DROP TABLE IF EXISTS password_resets;
