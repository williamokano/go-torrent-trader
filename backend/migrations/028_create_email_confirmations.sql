-- +goose Up
CREATE TABLE email_confirmations (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   BYTEA NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    confirmed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_confirmations_user_id ON email_confirmations(user_id);
-- token_hash already has a UNIQUE constraint which creates an implicit index

-- +goose Down
DROP TABLE IF EXISTS email_confirmations;
