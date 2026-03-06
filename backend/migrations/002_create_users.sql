-- +goose Up
CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    username        VARCHAR(20) NOT NULL UNIQUE,
    email           VARCHAR(255) NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    password_scheme VARCHAR(50) NOT NULL DEFAULT 'argon2id',
    passkey         VARCHAR(32) UNIQUE,
    group_id        BIGINT NOT NULL REFERENCES groups(id),
    uploaded        BIGINT NOT NULL DEFAULT 0,
    downloaded      BIGINT NOT NULL DEFAULT 0,
    avatar          VARCHAR(255),
    title           VARCHAR(255),
    info            TEXT,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    parked          BOOLEAN NOT NULL DEFAULT false,
    ip              INET,
    last_login      TIMESTAMPTZ,
    last_access     TIMESTAMPTZ,
    invites         INT NOT NULL DEFAULT 0,
    warned          BOOLEAN NOT NULL DEFAULT false,
    warn_until      TIMESTAMPTZ,
    donor           BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_passkey ON users (passkey);
CREATE INDEX idx_users_group_id ON users (group_id);

-- +goose Down
DROP TABLE IF EXISTS users;
