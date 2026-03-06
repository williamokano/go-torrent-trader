-- +goose Up
CREATE TABLE invites (
    id          BIGSERIAL PRIMARY KEY,
    inviter_id  BIGINT NOT NULL REFERENCES users(id),
    email       VARCHAR(255) NOT NULL,
    token       VARCHAR(64) NOT NULL UNIQUE,
    used_by_id  BIGINT REFERENCES users(id),
    used_at     TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_invites_token ON invites (token);
CREATE INDEX idx_invites_inviter ON invites (inviter_id);

-- +goose Down
DROP TABLE IF EXISTS invites;
