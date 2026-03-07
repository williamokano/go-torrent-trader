-- +goose Up
DROP INDEX IF EXISTS idx_password_resets_token;
CREATE UNIQUE INDEX idx_password_resets_token ON password_resets (token_hash);

-- +goose Down
DROP INDEX IF EXISTS idx_password_resets_token;
CREATE INDEX idx_password_resets_token ON password_resets (token_hash);
