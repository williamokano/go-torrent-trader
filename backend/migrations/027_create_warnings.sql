-- +goose Up
CREATE TABLE warnings (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    reason      TEXT NOT NULL,
    issued_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    lifted_at   TIMESTAMPTZ,
    lifted_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    lifted_reason TEXT,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_warnings_user_status ON warnings (user_id, status);

-- Seed ratio warning site settings with defaults
INSERT INTO site_settings (key, value, updated_at) VALUES
    ('ratio_warning_threshold', '0.3', NOW()),
    ('ratio_minimum_downloaded', '5368709120', NOW()),
    ('ratio_warn_days', '7', NOW()),
    ('ratio_ban_days', '14', NOW()),
    ('ratio_warning_message', 'Dear {{username}}, your ratio ({{ratio}}) has been below the minimum threshold of {{threshold}} for {{days_elapsed}} days. You have {{days_remaining}} days to improve before your account is disabled.', NOW()),
    ('ratio_ban_message', 'Dear {{username}}, your account has been disabled because your ratio ({{ratio}}) remained below the minimum threshold of {{threshold}} for more than {{days_elapsed}} days.', NOW())
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS warnings;

DELETE FROM site_settings WHERE key IN (
    'ratio_warning_threshold',
    'ratio_minimum_downloaded',
    'ratio_warn_days',
    'ratio_ban_days',
    'ratio_warning_message',
    'ratio_ban_message'
);
