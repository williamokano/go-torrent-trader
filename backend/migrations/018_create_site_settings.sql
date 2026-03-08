-- +goose Up
CREATE TABLE site_settings (
    key        VARCHAR(64) PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed default registration mode
INSERT INTO site_settings (key, value) VALUES ('registration_mode', 'invite_only');

-- +goose Down
DROP TABLE IF EXISTS site_settings;
