-- +goose Up
CREATE TABLE banned_emails (
    id BIGSERIAL PRIMARY KEY,
    pattern VARCHAR(255) NOT NULL UNIQUE,
    reason TEXT,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE banned_ips (
    id BIGSERIAL PRIMARY KEY,
    ip_range CIDR NOT NULL UNIQUE,
    reason TEXT,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS banned_ips;
DROP TABLE IF EXISTS banned_emails;
